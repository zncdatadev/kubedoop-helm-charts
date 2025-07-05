package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	repository    string
	chartDir      string
	baseBranch    string
	pagesBranch   string
	versionPattern string
	force         bool
	withTags      bool
	cleanIndex    bool
	logLevel      string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "chart-release-manager",
		Short: "Helm Chart Release Manager",
		Long: `A tool for managing GitHub releases for Helm charts.
Provides functionality to:
1. Delete all releases from a repository
2. Delete specific chart releases based on version patterns and git changes
3. Clean up chart index entries from the gh-pages branch`,
	}

	var deleteAllCmd = &cobra.Command{
		Use:   "delete-all",
		Short: "Delete all releases from a repository",
		RunE:  deleteAllReleases,
	}

	var deleteReleaseCmd = &cobra.Command{
		Use:   "delete-release",
		Short: "Delete specific chart releases",
		RunE:  deleteSpecificReleases,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "INFO", "Set the logging level (DEBUG, INFO, WARNING, ERROR, CRITICAL)")

	// Delete all command flags
	deleteAllCmd.Flags().StringVarP(&repository, "repository", "r", "", "GitHub repository (format: owner/repo)")
	deleteAllCmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	deleteAllCmd.Flags().BoolVarP(&withTags, "with-tags", "t", false, "Also delete associated tags for each release")
	deleteAllCmd.Flags().BoolVarP(&cleanIndex, "clean-index", "i", false, "Also clean up chart index entries from the pages branch")
	deleteAllCmd.Flags().StringVarP(&pagesBranch, "pages-branch", "p", "gh-pages", "The branch containing the chart index")
	deleteAllCmd.MarkFlagRequired("repository")

	// Delete release command flags
	deleteReleaseCmd.Flags().StringVarP(&repository, "repository", "r", "", "GitHub repository (format: owner/repo)")
	deleteReleaseCmd.Flags().StringVarP(&chartDir, "chart-dir", "d", "charts", "Directory containing Helm charts")
	deleteReleaseCmd.Flags().StringVarP(&baseBranch, "base-branch", "b", "main", "Base branch to compare changes against")
	deleteReleaseCmd.Flags().StringVarP(&pagesBranch, "pages-branch", "p", "gh-pages", "Branch containing the chart index")
	deleteReleaseCmd.Flags().StringVarP(&versionPattern, "version-pattern", "v", "^0\\.0\\.0-dev$", "Regex pattern to match chart versions for deletion")
	deleteReleaseCmd.MarkFlagRequired("repository")

	rootCmd.AddCommand(deleteAllCmd, deleteReleaseCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func deleteAllReleases(cmd *cobra.Command, args []string) error {
	setupLogging(logLevel)

	ctx := context.Background()
	manager, err := NewChartReleaseManager(repository)
	if err != nil {
		return fmt.Errorf("failed to create release manager: %w", err)
	}

	if !manager.CheckAuthentication(ctx) {
		return fmt.Errorf("not authenticated with GitHub. Please login using 'gh auth login' or set GH_TOKEN environment variable")
	}

	releases, err := manager.GetAllReleases(ctx)
	if err != nil {
		return fmt.Errorf("failed to get releases: %w", err)
	}

	if len(releases) == 0 {
		log.Printf("No releases found in repository %s", repository)
		return nil
	}

	if withTags {
		log.Println("Associated tags will also be deleted.")
	}

	if cleanIndex {
		log.Printf("Chart index entries will also be cleaned up from %s branch.", pagesBranch)
	}

	if !force {
		if !confirmAction("Are you sure you want to proceed? [y/N] ") {
			log.Println("Operation cancelled.")
			return nil
		}
	}

	// Delete releases
	for _, release := range releases {
		if err := manager.DeleteRelease(ctx, release.GetID(), release.GetName()); err != nil {
			log.Printf("Failed to delete release %s: %v", release.GetName(), err)
		}

		// Delete associated tag if requested
		if withTags && release.GetTagName() != "" {
			if err := manager.DeleteTag(ctx, release.GetTagName()); err != nil {
				log.Printf("Failed to delete tag %s: %v", release.GetTagName(), err)
			}
		}
	}

	// Clean up chart index if requested
	if cleanIndex {
		git := NewGit(".")
		indexManager := NewChartIndexManager(git, pagesBranch)
		if err := indexManager.CleanAllChartIndex(); err != nil {
			log.Printf("Failed to clean chart index: %v", err)
		}
	}

	return nil
}

func deleteSpecificReleases(cmd *cobra.Command, args []string) error {
	setupLogging(logLevel)

	ctx := context.Background()
	manager, err := NewChartReleaseManager(repository)
	if err != nil {
		return fmt.Errorf("failed to create release manager: %w", err)
	}

	if !manager.CheckAuthentication(ctx) {
		return fmt.Errorf("not authenticated with GitHub. Please login using 'gh auth login' or set GH_TOKEN environment variable")
	}

	git := NewGit(".")
	chartManager := NewChartManager(chartDir)
	indexManager := NewChartIndexManager(git, pagesBranch)

	// Get latest tag for comparison
	latestTag, err := git.GetLatestTag(baseBranch)
	if err != nil {
		return fmt.Errorf("failed to get latest tag: %w", err)
	}
	log.Printf("Discovering changes since %s...", latestTag)

	// Get changed files
	changedFiles, err := git.GetChangedFiles(latestTag, chartDir)
	if err != nil {
		return fmt.Errorf("failed to get changed files: %w", err)
	}

	// Get changed charts
	changedCharts, err := chartManager.GetChangedCharts(changedFiles, versionPattern)
	if err != nil {
		return fmt.Errorf("failed to get changed charts: %w", err)
	}

	if len(changedCharts) == 0 {
		log.Println("No changes detected.")
		return nil
	}

	chartNames := make([]string, len(changedCharts))
	for i, chart := range changedCharts {
		chartNames[i] = chart.Name
	}
	log.Printf("The following charts have changed, and their releases will be deleted: %v", chartNames)

	// Delete releases for each changed chart
	for _, chart := range changedCharts {
		releaseName := fmt.Sprintf("%s-%s", chart.Name, chart.Version)
		log.Printf("Deleting release and tags for %s...", releaseName)

		// Delete release by tag
		if err := manager.DeleteReleaseByTag(ctx, releaseName); err != nil {
			log.Printf("Failed to delete release by tag %s: %v", releaseName, err)
		}

		// Delete tag
		if err := manager.DeleteTag(ctx, releaseName); err != nil {
			log.Printf("Failed to delete tag %s: %v", releaseName, err)
		}

		log.Printf("Deleted release for %s", chart.Name)
	}

	// Clean up chart indexes in batch
	if len(changedCharts) > 0 {
		if err := indexManager.CleanMultipleChartIndexes(changedCharts); err != nil {
			log.Printf("Failed to clean chart indexes: %v", err)
		}
	}

	return nil
}

func setupLogging(level string) {
	// Configure logging based on level
	switch level {
	case "DEBUG":
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	default:
		log.SetFlags(log.LstdFlags)
	}
}

func confirmAction(prompt string) bool {
	fmt.Print(prompt)
	var response string
	fmt.Scanln(&response)
	return response == "y" || response == "Y"
}

func init() {
	viper.AutomaticEnv()
}
