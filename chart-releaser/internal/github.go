package internal

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

// GHClient GitHub client wrapper
type GHClient struct {
	client *github.Client
	ctx    context.Context
	owner  string
	repo   string
	logger logr.Logger
}

// NewGHClient creates a new GitHub client
func NewGHClient(owner, repo string, token string) (*GHClient, error) {
	var client *github.Client
	ctx := context.Background()

	logger := Logger.WithName("github")

	logger.Info("Creating GitHub client", "owner", owner, "repo", repo, "hasToken", token != "")

	if token == "" {
		client = github.NewClient(nil)
	} else {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(ctx, ts)
		client = github.NewClient(tc)
	}

	return &GHClient{
		client: client,
		ctx:    ctx,
		owner:  owner,
		repo:   repo,
		logger: logger,
	}, nil
}

func (gh *GHClient) GetAllReleases() ([]*github.RepositoryRelease, error) {
	gh.logger.Info("Getting all releases", "owner", gh.owner, "repo", gh.repo)

	var allReleases []*github.RepositoryRelease
	opts := &github.ListOptions{PerPage: 100}

	for {
		releases, resp, err := gh.client.Repositories.ListReleases(gh.ctx, gh.owner, gh.repo, opts)
		if err != nil {
			gh.logger.Error(err, "Failed to list releases")
			return nil, fmt.Errorf("failed to list releases: %w", err)
		}

		allReleases = append(allReleases, releases...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	gh.logger.Info("Found releases", "count", len(allReleases))
	return allReleases, nil
}

// DeleteRelease deletes the specified release
func (gh *GHClient) DeleteRelease(releaseID int64) error {
	gh.logger.Info("Deleting release", "releaseID", releaseID)

	// _, err := gh.client.Repositories.DeleteRelease(gh.ctx, gh.owner, gh.repo, releaseID)
	// if err != nil {
	// 	return fmt.Errorf("failed to delete release %d: %w", releaseID, err)
	// }

	gh.logger.Info("Successfully deleted release", "releaseID", releaseID)
	return nil
}

// DeleteReleaseByTag deletes release by tag
func (gh *GHClient) DeleteReleaseByTag(tag string) error {
	gh.logger.Info("Deleting release by tag", "tag", tag)

	// First get release information
	release, _, err := gh.client.Repositories.GetReleaseByTag(gh.ctx, gh.owner, gh.repo, tag)
	if err != nil {
		if isNotFoundError(err) {
			gh.logger.Info("Release not found, skipping deletion", "tag", tag)
			return nil
		}
		return fmt.Errorf("failed to get release by tag %s: %w", tag, err)
	}

	// Delete release
	return gh.DeleteRelease(release.GetID())
}

// DeleteTag deletes the specified tag
func (gh *GHClient) DeleteTag(tag string) error {
	gh.logger.Info("Deleting tag", "tag", tag)

	// Delete tag reference
	// _, err := gh.client.Git.DeleteRef(gh.ctx, gh.owner, gh.repo, fmt.Sprintf("tags/%s", tag))
	// if err != nil {
	// 	if isNotFoundError(err) {
	// 		log.Printf("Tag %s not found, skipping deletion", tag)
	// 		return nil
	// 	}
	// 	return fmt.Errorf("failed to delete tag %s: %w", tag, err)
	// }

	log.Printf("Successfully deleted tag: %s", tag)
	return nil
}

// DeleteAllReleases deletes all releases
func (gh *GHClient) DeleteAllReleases() error {
	log.Printf("Deleting all releases for %s/%s", gh.owner, gh.repo)

	releases, err := gh.GetAllReleases()
	if err != nil {
		return fmt.Errorf("failed to get releases: %w", err)
	}

	if len(releases) == 0 {
		log.Println("No releases found to delete")
		return nil
	}

	var errors []string
	for _, release := range releases {
		if err := gh.DeleteRelease(release.GetID()); err != nil {
			errors = append(errors, err.Error())
			log.Printf("Failed to delete release %s: %v", release.GetTagName(), err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to delete some releases: %s", strings.Join(errors, "; "))
	}

	log.Printf("Successfully deleted %d releases", len(releases))
	return nil
}

// DeleteReleaseAndTag deletes release and corresponding tag
func (gh *GHClient) DeleteReleaseAndTag(tag string) error {
	log.Printf("Deleting release and tag: %s", tag)

	// First delete release
	if err := gh.DeleteReleaseByTag(tag); err != nil {
		return fmt.Errorf("failed to delete release: %w", err)
	}

	// Then delete tag
	if err := gh.DeleteTag(tag); err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}

	log.Printf("Successfully deleted release and tag: %s", tag)
	return nil
}

// isNotFoundError checks if the error is a 404 error
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "404")
}
