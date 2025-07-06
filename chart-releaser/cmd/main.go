package main

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/zncdatadev/kubedoop-helm-charts/chart-releaser/internal"
)

var (
	owner          string
	repository     string
	chartDir       string
	baseBranch     string
	pagesBranch    string
	versionPattern string
	cleanAllIndex  bool
	logLevel       string
	logger         logr.Logger
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Initialize logger
			if err := internal.InitLogger(internal.LogLevel(logLevel)); err != nil {
				return fmt.Errorf("failed to initialize logger: %w", err)
			}
			logger = internal.WithName("main")
			logger.Info("Starting chart release manager", "version", "1.0.0")
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			// Flush log buffer
			internal.FlushLogs()
		},
	}

	var cleanup = &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up chart index entries from the gh-pages branch",
		RunE:  cleanupChartIndex,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "INFO", "Set the logging level (DEBUG, INFO, WARN, ERROR)")

	// Delete release command flags
	cleanup.Flags().StringVarP(&owner, "owner", "o", "", "GitHub repository owner")
	cleanup.Flags().StringVarP(&repository, "repo", "r", "", "GitHub repository")
	cleanup.Flags().StringVarP(&chartDir, "chart-dir", "d", "charts", "Directory containing Helm charts")
	cleanup.Flags().StringVarP(&pagesBranch, "pages-branch", "p", "gh-pages", "Branch containing the chart index")
	cleanup.Flags().StringVarP(&baseBranch, "base-branch", "b", "main", "Base branch to compare changes against")
	cleanup.Flags().StringVarP(&versionPattern, "version-pattern", "v", "^0\\.0\\.0-dev$", "Regex pattern to match chart versions for deletion")
	cleanup.Flags().BoolVar(&cleanAllIndex, "all", false, "Clean all chart index entries (ignore version pattern)")
	cleanup.MarkFlagRequired("owner")
	cleanup.MarkFlagRequired("repo")

	rootCmd.AddCommand(cleanup)

	if err := rootCmd.Execute(); err != nil {
		if logger.Enabled() {
			logger.Error(err, "Command execution failed")
		}
		os.Exit(1)
	}
}

func cleanupChartIndex(cmd *cobra.Command, args []string) error {
	cmdLogger := logger.WithName("cleanup")

	if owner == "" || repository == "" {
		return fmt.Errorf("repository owner and name must be specified")
	}

	cmdLogger.Info("Starting cleanup process",
		"owner", owner,
		"repository", repository,
		"chartDir", chartDir,
		"pagesBranch", pagesBranch,
		"baseBranch", baseBranch,
		"versionPattern", versionPattern,
		"cleanAllIndex", cleanAllIndex)

	// get ghtoken from environment variable
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is not set")
	}

	git := internal.NewGit(chartDir)
	ghc, err := internal.NewGHClient(owner, repository, ghToken)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}
	index := internal.NewIndexManager(git, pagesBranch)
	releaseManager := internal.NewReleaseManager(baseBranch, chartDir, git, index, ghc, versionPattern)

	if cleanAllIndex {
		cmdLogger.Info("Cleaning all chart index entries...")
		if err := index.CleanAllEntries(); err != nil {
			return fmt.Errorf("failed to clean all chart index entries: %w", err)
		}
		cmdLogger.Info("Successfully cleaned all chart index entries")
		return nil
	}

	cmdLogger.Info("Deleting changed chart releases...")
	if err := releaseManager.DeleteChangedCharts(); err != nil {
		return fmt.Errorf("failed to delete changed chart releases: %w", err)
	}
	cmdLogger.Info("Successfully deleted changed chart releases")

	return nil
}

func init() {
	viper.AutomaticEnv()
}
