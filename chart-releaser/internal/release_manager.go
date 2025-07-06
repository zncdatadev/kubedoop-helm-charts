package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
)

// ChartMetadata represents the Chart.yaml structure
type ChartMetadata struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// ChartInfo represents information about a Helm chart
type ChartInfo struct {
	Path    string
	Name    string
	Version string
}

// ReleaseManager manages Helm chart operations
type ReleaseManager struct {
	git            *Git
	index          *IndexManager
	ghc            *GHClient
	baseBranch     string
	chartDir       string
	versionPattern string

	logger logr.Logger
}

// NewReleaseManager creates a new ReleaseManager
func NewReleaseManager(
	baseBranch string,
	chartDir string,
	git *Git,
	index *IndexManager,
	ghc *GHClient,
	versionPattern string,
) *ReleaseManager {
	return &ReleaseManager{
		git:            git,
		baseBranch:     baseBranch,
		chartDir:       chartDir,
		versionPattern: versionPattern,
		index:          index,
		ghc:            ghc,

		logger: Logger.WithName("release-manager"),
	}
}

// IsHelmChart checks if a directory contains a Helm chart
func (m *ReleaseManager) isHelmChart(chartPath string) bool {
	chartFile := filepath.Join(chartPath, "Chart.yaml")
	_, err := os.Stat(chartFile)
	return err == nil
}

// GetChartInfo gets chart information from Chart.yaml
func (m *ReleaseManager) getChartInfo(chartPath string) (*ChartInfo, error) {
	chartFile := filepath.Join(chartPath, "Chart.yaml")

	// Read Chart.yaml file
	data, err := os.ReadFile(chartFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read chart file %s: %w", chartFile, err)
	}

	// Parse YAML
	var metadata ChartMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse chart file %s: %w", chartFile, err)
	}

	return &ChartInfo{
		Path:    chartPath,
		Name:    metadata.Name,
		Version: metadata.Version,
	}, nil
}

// VersionMatchesPattern checks if a version matches a regex pattern
func (m *ReleaseManager) versionMatchesPattern(version, pattern string) (bool, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regex pattern '%s': %w", pattern, err)
	}
	return regex.MatchString(version), nil
}

// GetChangedCharts gets list of changed charts that match the version pattern
func (m *ReleaseManager) getChangedCharts(changedFiles []string, versionPattern string) ([]*ChartInfo, error) {
	var changedCharts []*ChartInfo

	// Extract unique chart directories from changed files
	chartDirs := make(map[string]bool)
	for _, filePath := range changedFiles {
		pathParts := strings.Split(filepath.Clean(filePath), string(filepath.Separator))
		if len(pathParts) >= 2 && pathParts[0] == m.chartDir {
			chartDir := filepath.Join(m.chartDir, pathParts[1])
			chartDirs[chartDir] = true
		}
	}

	// Filter and validate charts
	for chartDir := range chartDirs {
		if !m.isHelmChart(chartDir) {
			m.logger.Info("%s is not a Helm chart. Skipping.", chartDir)
			continue
		}

		chartInfo, err := m.getChartInfo(chartDir)
		if err != nil {
			m.logger.Info("Error getting chart info for %s: %v", chartDir, err)
			continue
		}

		matches, err := m.versionMatchesPattern(chartInfo.Version, versionPattern)
		if err != nil {
			return nil, fmt.Errorf("error checking version pattern: %w", err)
		}

		if !matches {
			return nil, fmt.Errorf("chart version %s does not match supported pattern for deletion: %s", chartInfo.Version, versionPattern)
		}

		m.logger.Info("Found Helm chart: %s with version %s", chartDir, chartInfo.Version)
		changedCharts = append(changedCharts, chartInfo)
	}

	return changedCharts, nil
}

func (m *ReleaseManager) DeleteAllReleases() error {
	m.logger.Info("Deleting all releases...")

	// Delete all releases from the repository
	if err := m.ghc.DeleteAllReleases(); err != nil {
		return fmt.Errorf("failed to delete all releases: %w", err)
	}

	m.logger.Info("Successfully deleted all releases")

	// Clean up the chart index entries
	if err := m.index.CleanAllEntries(); err != nil {
		return fmt.Errorf("failed to clean chart index entries: %w", err)
	}
	m.logger.Info("Successfully cleaned chart index entries")

	return nil
}

func (m *ReleaseManager) DeleteChangedCharts() error {
	changedFiles, err := m.git.GetChangedFiles(m.baseBranch, m.chartDir)
	if err != nil {
		return fmt.Errorf("failed to get changed files: %w", err)
	}
	if len(changedFiles) == 0 {
		m.logger.Info("No changed files found since the base branch")
		return nil
	}

	m.logger.Info("Found %d changed files", len(changedFiles))

	// Get the charts that match the version pattern
	changedCharts, err := m.getChangedCharts(changedFiles, m.versionPattern)
	if err != nil {
		return fmt.Errorf("failed to get changed charts: %w", err)
	}

	if len(changedCharts) == 0 {
		m.logger.Info("No charts found matching the version pattern")
		return nil
	}

	m.logger.Info("Found %d charts to delete", len(changedCharts))

	// Delete the releases for the changed charts
	m.logger.Info("Deleting releases for changed charts...")
	for _, chart := range changedCharts {
		releaseName := fmt.Sprintf("%s-%s", chart.Name, chart.Version)
		m.logger.Info("Deleting release: %s", releaseName)
		if err := m.ghc.DeleteReleaseAndTag(releaseName); err != nil {
			m.logger.Info("Failed to delete release %s: %v", releaseName, err)
			continue
		}
	}

	m.logger.Info("Successfully deleted all specified chart releases")
	// Clean up the chart index entries
	if err := m.index.CleanEntriesVersions(changedCharts); err != nil {
		return fmt.Errorf("failed to clean chart index entries: %w", err)
	}
	m.logger.Info("Successfully cleaned chart index entries")
	return nil
}
