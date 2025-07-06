package internal

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-logr/logr"
)

// Git manages Git operations for detecting changes and managing branches
type Git struct {
	logger logr.Logger
}

// NewGit creates a new Git instance
func NewGit(repoPath string) *Git {
	return &Git{
		logger: Logger.WithName("git"),
	}
}

// runGitCommand runs a git command and returns its output
func (g *Git) runGitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)

	g.logger.V(1).Info("Running git command", "args", args)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git command failed. args: %v, error: %w", args, err)
	}

	result := strings.TrimSpace(string(output))
	g.logger.V(2).Info("Git command output", "args", args, "output", result)
	return result, nil
}

// GetCurrentBranch gets the current branch name
func (g *Git) GetCurrentBranch() (string, error) {
	return g.runGitCommand("rev-parse", "--abbrev-ref", "HEAD")
}

// FetchTags fetches tags from remote repository
func (g *Git) fetchTags() error {
	g.logger.Info("Fetching tags from remote")
	_, err := g.runGitCommand("fetch", "--tags")
	if err != nil {
		g.logger.Error(err, "Failed to fetch tags from remote")
	}
	return err
}

// GetLatestTag gets the latest tag or appropriate commit for comparison
func (g *Git) getLatestTag(baseBranch string) (string, error) {
	// Fetch tags first
	g.fetchTags()

	currentBranch, err := g.GetCurrentBranch()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	g.logger.Info("Getting latest tag", "currentBranch", currentBranch, "baseBranch", baseBranch)

	// Try to get the latest tag
	latestTag, err := g.runGitCommand("describe", "--tags", "--abbrev=0", "HEAD~")
	if err == nil {
		g.logger.Info("Found latest tag", "tag", latestTag)
		return latestTag, nil
	}

	g.logger.Info("No tags found, using commit-based comparison")

	// No tags found, decide based on branch
	if currentBranch == baseBranch {
		// On base branch, use first commit
		g.logger.Info("On base branch, using first commit")
		return g.runGitCommand("rev-list", "--max-parents=0", "--first-parent", "HEAD")
	} else {
		// On other branches, use merge base with base branch
		g.logger.Info("On feature branch, using merge base with base branch")
		return g.runGitCommand("merge-base", "HEAD", baseBranch)
	}
}

// GetChangedFiles gets list of changed files since a commit
func (g *Git) GetChangedFiles(baseBranch, pathFilter string) ([]string, error) {
	sinceCommit, err := g.getLatestTag(baseBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest tag: %w", err)
	}
	if sinceCommit == "" {
		return nil, fmt.Errorf("no commits found since base branch %s", baseBranch)
	}

	g.logger.Info("Getting changed files", "sinceCommit", sinceCommit, "pathFilter", pathFilter)

	args := []string{"diff", "--find-renames", "--name-only", sinceCommit}
	if pathFilter != "" {
		args = append(args, "--", pathFilter)
	}

	output, err := g.runGitCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	if output == "" {
		g.logger.Info("No changed files found")
		return []string{}, nil
	}

	files := strings.Split(output, "\n")
	g.logger.Info("Found changed files", "count", len(files), "files", files)
	return files, nil
}

// CheckoutBranch checks out a branch
func (g *Git) CheckoutBranch(branch string) error {
	_, err := g.runGitCommand("checkout", branch)
	return err
}

// BranchExists checks if a branch exists on remote
func (g *Git) BranchExists(branch string) bool {
	output, err := g.runGitCommand("ls-remote", "--heads", "origin", branch)
	if err != nil {
		return false
	}
	return strings.Contains(output, branch)
}

// FetchBranch fetches a branch from remote
func (g *Git) FetchBranch(branch string) error {
	_, err := g.runGitCommand("fetch", "origin", branch)
	return err
}

// PullBranch pulls latest changes from a branch
func (g *Git) PullBranch(branch string) error {
	_, err := g.runGitCommand("pull", "origin", branch)
	return err
}

// HasChanges checks if a file has uncommitted changes
func (g *Git) HasChanges(filePath string) bool {
	_, err := g.runGitCommand("diff", "--quiet", filePath)
	return err != nil // Has changes if command fails
}

// CommitAndPush commits and pushes changes to a file
func (g *Git) CommitAndPush(filePath, message, branch string) error {
	// Add file
	if _, err := g.runGitCommand("add", filePath); err != nil {
		return fmt.Errorf("failed to add file: %w", err)
	}

	// Commit
	if _, err := g.runGitCommand("commit", "-m", message); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	// Push
	// if _, err := g.runGitCommand("push", "origin", branch); err != nil {
	// 	return fmt.Errorf("failed to push: %w", err)
	// }

	return nil
}
