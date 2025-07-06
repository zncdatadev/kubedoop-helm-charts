package main

import (
	"fmt"
	"os"

	"github.com/zncdatadev/kubedoop-helm-charts/chart-releaser/internal"
)

func main() {
	// Get log level from environment variable, default to INFO
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}

	// Initialize logging system
	if err := internal.InitLogger(internal.LogLevel(logLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Get main logger
	logger := internal.WithName("example")
	logger.Info("Starting example application", "version", "1.0.0", "logLevel", logLevel)

	// Demonstrate logging for different components
	demoGitOperations()
	demoGitHubOperations()
	demoIndexOperations()

	// Flush log buffer
	internal.FlushLogs()
	logger.Info("Example application completed")
}

func demoGitOperations() {
	logger := internal.WithName("git").WithValues("component", "git-operations")

	logger.Info("Starting Git operations demo")

	// Simulate some Git operations
	logger.V(1).Info("Fetching tags from remote")
	logger.Info("Found changed files", "count", 3, "files", []string{"chart1/Chart.yaml", "chart2/values.yaml", "chart3/README.md"})

	// Simulate error scenario
	if false { // Set to true to test error logging
		logger.Error(fmt.Errorf("git command failed"), "Failed to checkout branch", "branch", "gh-pages")
	}

	logger.Info("Git operations completed successfully")
}

func demoGitHubOperations() {
	logger := internal.WithName("github").WithValues("component", "github-operations")

	logger.Info("Starting GitHub operations demo")

	// Simulate GitHub API calls
	logger.V(1).Info("Authenticating with GitHub API")
	logger.Info("Found releases", "count", 5)

	// Simulate delete operations
	for i := 1; i <= 3; i++ {
		releaseLogger := logger.WithValues("releaseID", i)
		releaseLogger.Info("Deleting release", "tag", fmt.Sprintf("v1.%d.0", i))
		releaseLogger.V(2).Info("Release deleted successfully")
	}

	logger.Info("GitHub operations completed successfully")
}

func demoIndexOperations() {
	logger := internal.WithName("index").WithValues("component", "index-operations")

	logger.Info("Starting index operations demo")

	// Simulate index operations
	logger.V(1).Info("Switching to gh-pages branch")
	logger.Info("Loading index.yaml", "path", "index.yaml")

	// Simulate cleanup operations
	charts := []string{"airflow-operator", "commons-operator", "hdfs-operator"}
	for _, chart := range charts {
		chartLogger := logger.WithValues("chart", chart)
		chartLogger.Info("Cleaning chart from index", "version", "0.0.0-dev")
		chartLogger.V(2).Info("Chart removed from index")
	}

	logger.Info("Committing changes to index", "changes", len(charts))
	logger.Info("Index operations completed successfully")
}
