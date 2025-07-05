package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

// ChartManager manages Helm chart operations
type ChartManager struct {
	ChartsDir string
}

// NewChartManager creates a new ChartManager
func NewChartManager(chartsDir string) *ChartManager {
	return &ChartManager{
		ChartsDir: chartsDir,
	}
}

// IsHelmChart checks if a directory contains a Helm chart
func (cm *ChartManager) IsHelmChart(chartPath string) bool {
	chartFile := filepath.Join(chartPath, "Chart.yaml")
	_, err := os.Stat(chartFile)
	return err == nil
}

// GetChartInfo gets chart information from Chart.yaml
func (cm *ChartManager) GetChartInfo(chartPath string) (*ChartInfo, error) {
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
func (cm *ChartManager) VersionMatchesPattern(version, pattern string) (bool, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regex pattern '%s': %w", pattern, err)
	}
	return regex.MatchString(version), nil
}

// GetChangedCharts gets list of changed charts that match the version pattern
func (cm *ChartManager) GetChangedCharts(changedFiles []string, versionPattern string) ([]*ChartInfo, error) {
	var changedCharts []*ChartInfo

	// Extract unique chart directories from changed files
	chartDirs := make(map[string]bool)
	for _, filePath := range changedFiles {
		pathParts := strings.Split(filepath.Clean(filePath), string(filepath.Separator))
		if len(pathParts) >= 2 && pathParts[0] == cm.ChartsDir {
			chartDir := filepath.Join(cm.ChartsDir, pathParts[1])
			chartDirs[chartDir] = true
		}
	}

	// Filter and validate charts
	for chartDir := range chartDirs {
		if !cm.IsHelmChart(chartDir) {
			log.Printf("%s is not a Helm chart. Skipping.", chartDir)
			continue
		}

		chartInfo, err := cm.GetChartInfo(chartDir)
		if err != nil {
			log.Printf("Error getting chart info for %s: %v", chartDir, err)
			continue
		}

		matches, err := cm.VersionMatchesPattern(chartInfo.Version, versionPattern)
		if err != nil {
			return nil, fmt.Errorf("error checking version pattern: %w", err)
		}

		if !matches {
			return nil, fmt.Errorf("chart version %s does not match supported pattern for deletion: %s", chartInfo.Version, versionPattern)
		}

		log.Printf("Found Helm chart: %s with version %s", chartDir, chartInfo.Version)
		changedCharts = append(changedCharts, chartInfo)
	}

	return changedCharts, nil
}
