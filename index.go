package main

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

// ChartVersion represents a chart version in the index
type ChartVersion struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	URLs    []string `yaml:"urls"`
}

// IndexFile represents the structure of index.yaml
type IndexFile struct {
	APIVersion string                       `yaml:"apiVersion"`
	Entries    map[string][]*ChartVersion `yaml:"entries"`
	Generated  string                     `yaml:"generated"`
}

// ChartIndexManager manages Helm chart index operations
type ChartIndexManager struct {
	Git *Git
	PagesBranch string
	OriginalBranch string
}

// NewChartIndexManager creates a new ChartIndexManager
func NewChartIndexManager(git *Git, pagesBranch string) *ChartIndexManager {
	return &ChartIndexManager{
		GitOps:      gitOps,
		PagesBranch: pagesBranch,
	}
}

// PreparePagesBranch prepares the pages branch for index operations
func (cim *ChartIndexManager) PreparePagesBranch() error {
	// Save current branch
	currentBranch, err := cim.GitOps.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	cim.OriginalBranch = currentBranch

	// Check if pages branch exists
	if !cim.GitOps.BranchExists(cim.PagesBranch) {
		log.Printf("%s branch does not exist. Skipping index cleanup.", cim.PagesBranch)
		return fmt.Errorf("pages branch does not exist")
	}

	// Fetch and checkout pages branch
	if err := cim.GitOps.FetchBranch(cim.PagesBranch); err != nil {
		return fmt.Errorf("failed to fetch %s branch: %w", cim.PagesBranch, err)
	}

	if err := cim.GitOps.CheckoutBranch(cim.PagesBranch); err != nil {
		return fmt.Errorf("failed to checkout %s branch: %w", cim.PagesBranch, err)
	}

	// Pull latest changes
	if err := cim.GitOps.PullBranch(cim.PagesBranch); err != nil {
		log.Printf("Failed to pull latest changes from %s: %v", cim.PagesBranch, err)
	}

	// Check if index.yaml exists
	if _, err := os.Stat("index.yaml"); os.IsNotExist(err) {
		log.Printf("index.yaml not found in %s branch. Skipping index cleanup.", cim.PagesBranch)
		cim.RestoreOriginalBranch()
		return fmt.Errorf("index.yaml not found")
	}

	return nil
}

// RestoreOriginalBranch restores the original branch
func (cim *ChartIndexManager) RestoreOriginalBranch() error {
	if cim.OriginalBranch != "" {
		return cim.GitOps.CheckoutBranch(cim.OriginalBranch)
	}
	return nil
}

// loadIndexFile loads the index.yaml file
func (cim *ChartIndexManager) loadIndexFile() (*IndexFile, error) {
	data, err := os.ReadFile("index.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read index.yaml: %w", err)
	}

	var indexFile IndexFile
	if err := yaml.Unmarshal(data, &indexFile); err != nil {
		return nil, fmt.Errorf("failed to parse index.yaml: %w", err)
	}

	return &indexFile, nil
}

// writeIndexFile writes the index.yaml file
func (cim *ChartIndexManager) writeIndexFile(indexFile *IndexFile) error {
	data, err := yaml.Marshal(indexFile)
	if err != nil {
		return fmt.Errorf("failed to marshal index.yaml: %w", err)
	}

	if err := os.WriteFile("index.yaml", data, 0644); err != nil {
		return fmt.Errorf("failed to write index.yaml: %w", err)
	}

	return nil
}

// CleanMultipleChartIndexes cleans up multiple chart index entries in a single operation
func (cim *ChartIndexManager) CleanMultipleChartIndexes(charts []*ChartInfo) error {
	if len(charts) == 0 {
		log.Printf("No charts provided for index cleanup")
		return nil
	}

	chartNames := make([]string, len(charts))
	for i, chart := range charts {
		chartNames[i] = fmt.Sprintf("%s v%s", chart.Name, chart.Version)
	}
	log.Printf("Cleaning up chart indexes for: %v", chartNames)

	if err := cim.PreparePagesBranch(); err != nil {
		return err
	}
	defer cim.RestoreOriginalBranch()

	// Load index.yaml
	indexFile, err := cim.loadIndexFile()
	if err != nil {
		return fmt.Errorf("failed to load index.yaml: %w", err)
	}

	var removedCharts []string

	// Remove each chart version
	for _, chart := range charts {
		chartName := chart.Name
		chartVersion := chart.Version

		if chartVersions, exists := indexFile.Entries[chartName]; exists {
			// Filter out the specific version
			var filteredVersions []*ChartVersion
			for _, version := range chartVersions {
				if version.Version != chartVersion {
					filteredVersions = append(filteredVersions, version)
				}
			}

			if len(filteredVersions) != len(chartVersions) {
				// Version was found and removed
				indexFile.Entries[chartName] = filteredVersions
				removedCharts = append(removedCharts, fmt.Sprintf("%s v%s", chartName, chartVersion))

				// Remove chart entry if no versions left
				if len(indexFile.Entries[chartName]) == 0 {
					delete(indexFile.Entries, chartName)
					log.Printf("Removed all versions of %s, deleted chart entry", chartName)
				}
			} else {
				log.Printf("Chart %s version %s not found in index.yaml", chartName, chartVersion)
			}
		} else {
			log.Printf("Chart %s not found in index.yaml", chartName)
		}
	}

	if len(removedCharts) > 0 {
		// Write back to file
		if err := cim.writeIndexFile(indexFile); err != nil {
			return fmt.Errorf("failed to write index.yaml: %w", err)
		}

		log.Printf("Removed from index.yaml: %v", removedCharts)

		// Commit and push if there are changes
		if cim.GitOps.HasChanges("index.yaml") {
			var commitMsg string
			if len(removedCharts) == 1 {
				commitMsg = fmt.Sprintf("Remove %s from index", removedCharts[0])
			} else {
				commitMsg = fmt.Sprintf("Remove %d chart versions from index", len(removedCharts))
			}

			if err := cim.GitOps.CommitAndPush("index.yaml", commitMsg, cim.PagesBranch); err != nil {
				return fmt.Errorf("failed to push index.yaml changes: %w", err)
			}
			log.Printf("Successfully pushed index.yaml changes to %s branch", cim.PagesBranch)
		} else {
			log.Printf("No changes to commit in index.yaml")
		}
	} else {
		log.Printf("No chart versions were removed from index.yaml")
	}

	return nil
}

// CleanAllChartIndex cleans up all chart index entries
func (cim *ChartIndexManager) CleanAllChartIndex() error {
	log.Printf("Cleaning up all chart index entries from %s branch...", cim.PagesBranch)

	if err := cim.PreparePagesBranch(); err != nil {
		return err
	}
	defer cim.RestoreOriginalBranch()

	// Load index.yaml
	indexFile, err := cim.loadIndexFile()
	if err != nil {
		return fmt.Errorf("failed to load index.yaml: %w", err)
	}

	// Clear all entries
	indexFile.Entries = make(map[string][]*ChartVersion)

	// Write back to file
	if err := cim.writeIndexFile(indexFile); err != nil {
		return fmt.Errorf("failed to write index.yaml: %w", err)
	}

	log.Printf("Cleared all entries from index.yaml")

	// Commit and push changes
	if cim.GitOps.HasChanges("index.yaml") {
		commitMsg := "Clear all chart entries from index"
		if err := cim.GitOps.CommitAndPush("index.yaml", commitMsg, cim.PagesBranch); err != nil {
			return fmt.Errorf("failed to push index.yaml changes: %w", err)
		}
		log.Printf("Successfully pushed index.yaml changes to %s branch", cim.PagesBranch)
	} else {
		log.Printf("No changes to commit in index.yaml")
	}

	return nil
}
