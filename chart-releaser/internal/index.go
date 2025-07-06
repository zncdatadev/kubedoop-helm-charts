package internal

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/repo"
)

const indexYaml = "index.yaml"

// IndexManager manages Helm chart index operations
type IndexManager struct {
	Git            *Git
	PagesBranch    string
	OriginalBranch string

	logger logr.Logger
}

// NewIndexManager creates a new IndexManager
func NewIndexManager(git *Git, pagesBranch string) *IndexManager {
	return &IndexManager{
		Git:         git,
		PagesBranch: pagesBranch,
		logger:      Logger.WithName("index"),
	}
}

// preparePagesBranch prepares the pages branch for index operations
func (i *IndexManager) preparePagesBranch() error {
	// Save current branch
	currentBranch, err := i.Git.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	i.OriginalBranch = currentBranch

	// Check if pages branch exists
	if !i.Git.BranchExists(i.PagesBranch) {
		i.logger.Info("%s branch does not exist. Skipping index cleanup.", i.PagesBranch)
		return fmt.Errorf("pages branch does not exist")
	}

	// Fetch and checkout pages branch
	if err := i.Git.FetchBranch(i.PagesBranch); err != nil {
		return fmt.Errorf("failed to fetch %s branch: %w", i.PagesBranch, err)
	}

	if err := i.Git.CheckoutBranch(i.PagesBranch); err != nil {
		return fmt.Errorf("failed to checkout %s branch: %w", i.PagesBranch, err)
	}

	// Pull latest changes
	if err := i.Git.PullBranch(i.PagesBranch); err != nil {
		i.logger.Info("Failed to pull latest changes from %s: %v", i.PagesBranch, err)
	}

	// Check if index.yaml exists
	if _, err := os.Stat(indexYaml); os.IsNotExist(err) {
		i.logger.Info("index.yaml not found in %s branch. Skipping index cleanup.", i.PagesBranch)
		i.restoreOriginalBranch()
		return fmt.Errorf("index.yaml not found")
	}

	return nil
}

func (i *IndexManager) commitIndexChanges(message string) error {
	if changed := i.Git.HasChanges(indexYaml); !changed {
		i.logger.Info("No changes detected in %s, skipping commit", indexYaml)
		return nil
	}

	if err := i.Git.CommitAndPush(indexYaml, message, i.PagesBranch); err != nil {
		return fmt.Errorf("failed to commit and push changes: %w", err)
	}
	i.logger.Info("Successfully committed and pushed changes to %s branch", i.PagesBranch)
	return nil
}

// restoreOriginalBranch restores the original branch
func (i *IndexManager) restoreOriginalBranch() error {
	if i.OriginalBranch != "" {
		return i.Git.CheckoutBranch(i.OriginalBranch)
	}
	return nil
}

// loadIndexFile loads the index.yaml file
func (i *IndexManager) loadIndexFile() (*repo.IndexFile, error) {
	var indexFile *repo.IndexFile
	_, err := os.Stat(indexYaml)
	if os.IsNotExist(err) {
		return repo.NewIndexFile(), nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat index.yaml: %w", err)
	}

	indexFile, err = repo.LoadIndexFile(indexYaml)
	if err != nil {
		return nil, fmt.Errorf("failed to load index.yaml: %w", err)
	}

	return indexFile, nil
}

// writeIndexFile writes the index.yaml file
func (i *IndexManager) writeIndexFile(indexFile *repo.IndexFile) error {
	if err := indexFile.WriteFile(indexYaml, 0644); err != nil {
		return fmt.Errorf("failed to write index.yaml: %w", err)
	}
	i.logger.Info("Successfully wrote index.yaml")
	return nil
}

// CleanChartEntry removes a specific chart version from the index.yaml
func (i *IndexManager) CleanAllEntries() error {
	if err := i.preparePagesBranch(); err != nil {
		return fmt.Errorf("failed to prepare pages branch: %w", err)
	}
	defer i.restoreOriginalBranch()

	indexFile, err := i.loadIndexFile()
	if err != nil {
		return fmt.Errorf("failed to load index file: %w", err)
	}

	indexFile.Entries = make(map[string]repo.ChartVersions, 0) // Clear all entries

	if err := i.writeIndexFile(indexFile); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	i.logger.Info("All chart entries removed from index.yaml")

	if err := i.commitIndexChanges("Cleaned all chart entries from index.yaml"); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}
	return nil
}

// CleanEntriesVersions removes charts with specific versions
func (i *IndexManager) CleanEntriesVersions(charts []*ChartInfo) error {
	if err := i.preparePagesBranch(); err != nil {
		return fmt.Errorf("failed to prepare pages branch: %w", err)
	}
	defer i.restoreOriginalBranch()

	indexFile, err := i.loadIndexFile()
	if err != nil {
		return fmt.Errorf("failed to load index file: %w", err)
	}

	for _, chart := range charts {
		if _, exists := indexFile.Entries[chart.Name]; !exists {
			i.logger.Info("No entries found for chart %s", chart.Name)
			continue
		}

		// Remove the specific chart versions
		existVersions := indexFile.Entries[chart.Name]
		filteredVersions := make(repo.ChartVersions, 0, len(existVersions))
		for _, version := range existVersions {
			found := false
			if version.Version == chart.Version {
				found = true
			}
			if !found {
				filteredVersions = append(filteredVersions, version)
			}
		}
		if len(filteredVersions) == 0 {
			delete(indexFile.Entries, chart.Name)
		} else {
			indexFile.Entries[chart.Name] = filteredVersions
		}
	}

	if err := i.writeIndexFile(indexFile); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	i.logger.Info("Removed specified chart versions from index.yaml")

	if err := i.commitIndexChanges("Removed specified chart versions from index.yaml"); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}
	return nil
}
