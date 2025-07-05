#!/usr/bin/env python3
"""
Helm Chart Release Manager

This script manages GitHub releases for Helm charts, providing functionality to:
1. Delete all releases from a repository
2. Delete specific chart releases based on version patterns and git changes
3. Clean up chart index entries from the gh-pages branch

Usage:
    python chart_release_manager.py delete-all --repository owner/repo [options]
    python chart_release_manager.py delete-release --repository owner/repo [options]
"""

import argparse
import logging
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

import requests
from ruamel.yaml import YAML


yaml = YAML()
yaml.preserve_quotes = True
yaml.default_flow_style = False


class GitHubAPIError(Exception):
    """GitHub API related errors"""
    pass


class ChartReleaseManager:
    """Manages Helm chart releases and GitHub operations"""

    def __init__(self, repository: str, token: str | None = None):
        """
        Initialize the release manager.

        Args:
            repository: GitHub repository in format "owner/repo"
            token: GitHub token, if None will try to get from environment
        """
        self.repository = repository
        self.token = token or os.getenv('GH_TOKEN')
        self.base_url = 'https://api.github.com'
        self.session = requests.Session()

        if self.token:
            self.session.headers.update({
                'Authorization': f'token {self.token}',
                'Accept': 'application/vnd.github.v3+json'
            })

    def _make_request(self, method: str, endpoint: str, **kwargs) -> requests.Response:
        """
        Make a request to GitHub API.

        Args:
            method: HTTP method (GET, POST, DELETE, etc.)
            endpoint: API endpoint (without base URL)
            **kwargs: Additional arguments for requests

        Returns:
            Response object

        Raises:
            GitHubAPIError: If request fails
        """
        url = f"{self.base_url}/{endpoint}"

        try:
            response = self.session.request(method, url, **kwargs)
            if response.status_code == 404:
                return response  # Let caller handle 404 specifically
            response.raise_for_status()
            return response
        except requests.exceptions.RequestException as e:
            raise GitHubAPIError(f"GitHub API request failed: {e}")

    def check_authentication(self) -> bool:
        """
        Check if GitHub authentication is working.

        Returns:
            True if authenticated, False otherwise
        """
        try:
            response = self._make_request('GET', 'user')
            return response.status_code == 200
        except GitHubAPIError:
            # Try with gh CLI if direct API fails
            try:
                result = subprocess.run(['gh', 'auth', 'status'],
                                      capture_output=True, text=True)
                return result.returncode == 0
            except (subprocess.SubprocessError, FileNotFoundError):
                return False

    def get_all_releases(self) -> list[dict[str, Any]]:
        """
        Get all releases from the repository.

        Returns:
            List of release dictionaries
        """
        try:
            response = self._make_request('GET', f'repos/{self.repository}/releases')
            return response.json()
        except GitHubAPIError as e:
            logging.error(f"Error fetching releases: {e}")
            return []

    def delete_release(self, release_id: int, release_name: str) -> bool:
        """
        Delete a GitHub release.

        Args:
            release_id: ID of the release to delete
            release_name: Name of the release (for logging)

        Returns:
            True if successful, False otherwise
        """
        try:
            response = self._make_request('DELETE', f'repos/{self.repository}/releases/{release_id}')
            logging.info(f"Successfully deleted release: {release_name}")
            return True
        except GitHubAPIError as e:
            logging.error(f"Failed to delete release {release_name}: {e}")
            return False

    def delete_tag(self, tag_name: str) -> bool:
        """
        Delete a git tag from the repository.

        Args:
            tag_name: Name of the tag to delete

        Returns:
            True if successful, False otherwise
        """
        try:
            response = self._make_request('DELETE', f'repos/{self.repository}/git/refs/tags/{tag_name}')
            logging.info(f"Successfully deleted tag: {tag_name}")
            return True
        except GitHubAPIError as e:
            if "404" in str(e):
                logging.warning(f"Tag {tag_name} not found")
                return True  # Consider missing tag as success
            logging.error(f"Failed to delete tag {tag_name}: {e}")
            return False

    def get_release_by_tag(self, tag_name: str) -> dict[str, Any] | None:
        """
        Get a release by its tag name.

        Args:
            tag_name: Tag name to search for

        Returns:
            Release dictionary if found, None otherwise
        """
        try:
            response = self._make_request('GET', f'repos/{self.repository}/releases/tags/{tag_name}')
            return response.json()
        except GitHubAPIError:
            return None

    def delete_release_by_tag(self, tag_name: str) -> bool:
        """
        Delete a release by its tag name.

        Args:
            tag_name: Tag name of the release to delete

        Returns:
            True if successful, False otherwise
        """
        try:
            response = self._make_request('DELETE', f'repos/{self.repository}/releases/tags/{tag_name}')
            logging.info(f"Successfully deleted release with tag: {tag_name}")
            return True
        except GitHubAPIError as e:
            if "404" in str(e):
                logging.warning(f"No release found for tag {tag_name}")
                return True  # Consider missing release as success
            logging.error(f"Failed to delete release with tag {tag_name}: {e}")
            return False


class GitOperations:
    """Git operations for detecting changes and managing branches"""

    def __init__(self, repo_path: str = "."):
        """
        Initialize git operations.

        Args:
            repo_path: Path to the git repository
        """
        self.repo_path = Path(repo_path)

    def _run_git_command(self, args: list[str]) -> str:
        """
        Run a git command and return its output.

        Args:
            args: Git command arguments

        Returns:
            Command output as string

        Raises:
            subprocess.CalledProcessError: If git command fails
        """
        cmd = ['git'] + args
        result = subprocess.run(cmd, cwd=self.repo_path, capture_output=True, text=True)
        if result.returncode != 0:
            raise subprocess.CalledProcessError(result.returncode, cmd, result.stderr)
        return result.stdout.strip()

    def get_current_branch(self) -> str:
        """Get the current branch name"""
        return self._run_git_command(['rev-parse', '--abbrev-ref', 'HEAD'])

    def fetch_tags(self) -> None:
        """Fetch tags from remote repository"""
        try:
            self._run_git_command(['fetch', '--tags'])
        except subprocess.CalledProcessError:
            logging.warning("Failed to fetch tags from remote")

    def get_latest_tag(self, base_branch: str = "main") -> str:
        """
        Get the latest tag or appropriate commit for comparison.

        Args:
            base_branch: Base branch name

        Returns:
            Tag name or commit hash
        """
        self.fetch_tags()
        current_branch = self.get_current_branch()

        # Try to get the latest tag
        try:
            return self._run_git_command(['describe', '--tags', '--abbrev=0', 'HEAD~'])
        except subprocess.CalledProcessError:
            # No tags found, decide based on branch
            if current_branch == base_branch:
                # On base branch, use first commit
                return self._run_git_command(['rev-list', '--max-parents=0', '--first-parent', 'HEAD'])
            else:
                # On other branches, use merge base with base branch
                return self._run_git_command(['merge-base', 'HEAD', base_branch])

    def get_changed_files(self, since_commit: str, path_filter: str | None = None) -> list[str]:
        """
        Get list of changed files since a commit.

        Args:
            since_commit: Commit hash or tag to compare against
            path_filter: Optional path filter (e.g., "charts")

        Returns:
            List of changed file paths
        """
        cmd = ['diff', '--find-renames', '--name-only', since_commit]
        if path_filter:
            cmd.extend(['--', path_filter])

        try:
            output = self._run_git_command(cmd)
            return [line for line in output.split('\n') if line]
        except subprocess.CalledProcessError:
            return []

    def checkout_branch(self, branch: str) -> bool:
        """
        Checkout a branch.

        Args:
            branch: Branch name to checkout

        Returns:
            True if successful, False otherwise
        """
        try:
            self._run_git_command(['checkout', branch])
            return True
        except subprocess.CalledProcessError:
            return False

    def branch_exists(self, branch: str) -> bool:
        """
        Check if a branch exists on remote.

        Args:
            branch: Branch name to check

        Returns:
            True if branch exists, False otherwise
        """
        try:
            output = self._run_git_command(['ls-remote', '--heads', 'origin', branch])
            return branch in output
        except subprocess.CalledProcessError:
            return False

    def fetch_branch(self, branch: str) -> bool:
        """
        Fetch a branch from remote.

        Args:
            branch: Branch name to fetch

        Returns:
            True if successful, False otherwise
        """
        try:
            self._run_git_command(['fetch', 'origin', branch])
            return True
        except subprocess.CalledProcessError:
            return False

    def pull_branch(self, branch: str) -> bool:
        """
        Pull latest changes from a branch.

        Args:
            branch: Branch name to pull

        Returns:
            True if successful, False otherwise
        """
        try:
            self._run_git_command(['pull', 'origin', branch])
            return True
        except subprocess.CalledProcessError:
            return False

    def has_changes(self, file_path: str) -> bool:
        """
        Check if a file has uncommitted changes.

        Args:
            file_path: Path to the file to check

        Returns:
            True if file has changes, False otherwise
        """
        try:
            self._run_git_command(['diff', '--quiet', file_path])
            return False  # No changes
        except subprocess.CalledProcessError:
            return True  # Has changes

    def commit_and_push(self, file_path: str, message: str, branch: str) -> bool:
        """
        Commit and push changes to a file.

        Args:
            file_path: Path to file to commit
            message: Commit message
            branch: Branch to push to

        Returns:
            True if successful, False otherwise
        """
        try:
            self._run_git_command(['add', file_path])
            self._run_git_command(['commit', '-m', message])
            self._run_git_command(['push', 'origin', branch])
            return True
        except subprocess.CalledProcessError as e:
            logging.error(f"Failed to commit and push: {e}")
            return False


class ChartIndexManager:
    """Manages Helm chart index operations"""

    def __init__(self, git_ops: GitOperations):
        """
        Initialize chart index manager.

        Args:
            git_ops: GitOperations instance
        """
        self.git_ops = git_ops
        self.original_branch = None

    def prepare_pages_branch(self, pages_branch: str) -> bool:
        """
        Prepare the pages branch for index operations.

        Args:
            pages_branch: Name of the pages branch

        Returns:
            True if successful, False otherwise
        """
        # Save current branch
        self.original_branch = self.git_ops.get_current_branch()

        # Check if pages branch exists
        if not self.git_ops.branch_exists(pages_branch):
            logging.warning(f"{pages_branch} branch does not exist. Skipping index cleanup.")
            return False

        # Fetch and checkout pages branch
        if not self.git_ops.fetch_branch(pages_branch):
            logging.error(f"Failed to fetch {pages_branch} branch")
            return False

        if not self.git_ops.checkout_branch(pages_branch):
            logging.error(f"Failed to checkout {pages_branch} branch")
            return False

        # Pull latest changes
        if not self.git_ops.pull_branch(pages_branch):
            logging.warning(f"Failed to pull latest changes from {pages_branch}")

        # Check if index.yaml exists
        if not Path('index.yaml').exists():
            logging.warning(f"index.yaml not found in {pages_branch} branch. Skipping index cleanup.")
            self.restore_original_branch()
            return False

        return True

    def restore_original_branch(self) -> bool:
        """
        Restore the original branch.

        Returns:
            True if successful, False otherwise
        """
        if self.original_branch:
            return self.git_ops.checkout_branch(self.original_branch)
        return True

    def clean_specific_chart_index(self, chart_name: str, chart_version: str, pages_branch: str) -> bool:
        """
        Clean up a specific chart index entry.

        Args:
            chart_name: Name of the chart
            chart_version: Version of the chart
            pages_branch: Name of the pages branch

        Returns:
            True if successful, False otherwise
        """
        logging.info(f"Cleaning up chart index for {chart_name} version {chart_version}...")

        if not self.prepare_pages_branch(pages_branch):
            return False

        try:
            # Load index.yaml
            with open('index.yaml', 'r') as f:
                index_data = yaml.load(f)

            # Remove specific chart version
            if 'entries' in index_data and chart_name in index_data['entries']:
                entries = index_data['entries'][chart_name]
                # Filter out the specific version
                index_data['entries'][chart_name] = [
                    entry for entry in entries if entry.get('version') != chart_version
                ]

                # Remove chart entry if no versions left
                if not index_data['entries'][chart_name]:
                    del index_data['entries'][chart_name]

                # Write back to file
                with open('index.yaml', 'w') as f:
                    yaml.dump(index_data, f)

                logging.info(f"Removed {chart_name} version {chart_version} from index.yaml")

                # Commit and push if there are changes
                if self.git_ops.has_changes('index.yaml'):
                    commit_msg = f"Remove {chart_name} version {chart_version} from index"
                    success = self.git_ops.commit_and_push('index.yaml', commit_msg, pages_branch)
                    if success:
                        logging.info(f"Successfully pushed index.yaml changes to {pages_branch} branch")
                else:
                    logging.info("No changes to commit in index.yaml")
            else:
                logging.warning(f"Chart {chart_name} not found in index.yaml")

            return True

        except Exception as e:
            logging.error(f"Error cleaning specific chart index: {e}")
            return False

        finally:
            self.restore_original_branch()

    def clean_all_chart_index(self, pages_branch: str) -> bool:
        """
        Clean up all chart index entries.

        Args:
            pages_branch: Name of the pages branch

        Returns:
            True if successful, False otherwise
        """
        logging.info(f"Cleaning up all chart index entries from {pages_branch} branch...")

        if not self.prepare_pages_branch(pages_branch):
            return False

        try:
            # Load index.yaml
            with open('index.yaml', 'r') as f:
                index_data = yaml.load(f)

            # Clear all entries
            index_data['entries'] = {}

            # Write back to file
            with open('index.yaml', 'w') as f:
                yaml.dump(index_data, f)

            logging.info("Cleared all entries from index.yaml")

            # Commit and push changes
            if self.git_ops.has_changes('index.yaml'):
                commit_msg = "Clear all chart entries from index"
                success = self.git_ops.commit_and_push('index.yaml', commit_msg, pages_branch)
                if success:
                    logging.info(f"Successfully pushed index.yaml changes to {pages_branch} branch")
            else:
                logging.info("No changes to commit in index.yaml")

            return True

        except Exception as e:
            logging.error(f"Error cleaning all chart index: {e}")
            return False

        finally:
            self.restore_original_branch()


class ChartManager:
    """Manages Helm chart operations"""

    def __init__(self, charts_dir: str = "charts"):
        """
        Initialize chart manager.

        Args:
            charts_dir: Directory containing Helm charts
        """
        self.charts_dir = Path(charts_dir)
        yaml = YAML()

    def is_helm_chart(self, chart_path: Path) -> bool:
        """
        Check if a directory contains a Helm chart.

        Args:
            chart_path: Path to check

        Returns:
            True if it's a Helm chart, False otherwise
        """
        chart_file = chart_path / "Chart.yaml"
        return chart_file.exists()

    def get_chart_info(self, chart_path: Path) -> dict[str, str]:
        """
        Get chart information from Chart.yaml.

        Args:
            chart_path: Path to the chart directory

        Returns:
            Dictionary with chart name and version
        """
        chart_file = chart_path / "Chart.yaml"
        if not chart_file.exists():
            return {}

        try:
            with open(chart_file, 'r') as f:
                chart_data = yaml.load(f)
            return {
                'name': chart_data.get('name', chart_path.name),
                'version': chart_data.get('version', '')
            }
        except Exception as e:
            logging.error(f"Error reading {chart_file}: {e}")
            return {}

    def version_matches_pattern(self, version: str, pattern: str) -> bool:
        """
        Check if a version matches a regex pattern.

        Args:
            version: Version string to check
            pattern: Regex pattern

        Returns:
            True if version matches pattern, False otherwise
        """
        try:
            return bool(re.match(pattern, version))
        except re.error as e:
            logging.error(f"Invalid regex pattern '{pattern}': {e}")
            return False

    def get_changed_charts(self, changed_files: list[str], version_pattern: str) -> list[dict[str, str]]:
        """
        Get list of changed charts that match the version pattern.

        Args:
            changed_files: List of changed file paths
            version_pattern: Regex pattern for version matching

        Returns:
            List of dictionaries with chart information
        """
        changed_charts = []

        # Extract unique chart directories from changed files
        chart_dirs = set()
        for file_path in changed_files:
            path_parts = Path(file_path).parts
            if len(path_parts) >= 2 and path_parts[0] == self.charts_dir.name:
                chart_dir = self.charts_dir / path_parts[1]
                chart_dirs.add(chart_dir)

        # Filter and validate charts
        for chart_dir in chart_dirs:
            if not self.is_helm_chart(chart_dir):
                logging.warning(f"{chart_dir} is not a Helm chart. Skipping.")
                continue

            chart_info = self.get_chart_info(chart_dir)
            if not chart_info:
                continue

            version = chart_info['version']
            if not self.version_matches_pattern(version, version_pattern):
                logging.error(f"Chart version {version} does not match supported pattern for deletion: {version_pattern}")
                sys.exit(1)

            logging.info(f"Found Helm chart: {chart_dir} with version {version}")
            changed_charts.append({
                'path': str(chart_dir),
                'name': chart_info['name'],
                'version': version
            })

        return changed_charts


def delete_all_releases(args):
    """Delete all releases from a repository"""

    # Initialize managers
    release_manager = ChartReleaseManager(args.repository)

    # Check authentication
    if not release_manager.check_authentication():
        logging.error("Not authenticated with GitHub. Please login using 'gh auth login' or set GH_TOKEN environment variable.")
        sys.exit(1)

    # Get all releases
    releases = release_manager.get_all_releases()

    if not releases:
        logging.info(f"No releases found in repository {args.repository}")
        return

    # Display what will be deleted
    print(f"The following releases will be deleted from {args.repository}:")
    for release in releases:
        print(f"- {release['name']} (tag: {release['tag_name']})")
    print(f"Total releases to delete: {len(releases)}")

    if args.with_tags:
        print("Associated tags will also be deleted.")

    if args.clean_index:
        print(f"Chart index entries will also be cleaned up from {args.pages_branch} branch.")

    # Confirm deletion unless forced
    if not args.force:
        response = input("Are you sure you want to proceed? [y/N] ")
        if response.lower() != 'y':
            logging.info("Operation cancelled.")
            return

    # Delete releases
    for release in releases:
        release_manager.delete_release(release['id'], release['name'])

        # Delete associated tag if requested
        if args.with_tags and release['tag_name']:
            release_manager.delete_tag(release['tag_name'])

    # Clean up chart index if requested
    if args.clean_index:
        git_ops = GitOperations()
        index_manager = ChartIndexManager(git_ops)
        index_manager.clean_all_chart_index(args.pages_branch)


def delete_specific_releases(args):
    """Delete specific chart releases based on changes and version patterns"""

    # Initialize managers
    release_manager = ChartReleaseManager(args.repository)
    git_ops = GitOperations()
    chart_manager = ChartManager(args.chart_dir)
    index_manager = ChartIndexManager(git_ops)

    # Check authentication
    if not release_manager.check_authentication():
        logging.error("Not authenticated with GitHub. Please login using 'gh auth login' or set GH_TOKEN environment variable.")
        sys.exit(1)

    # Get latest tag for comparison
    latest_tag = git_ops.get_latest_tag(args.base_branch)
    logging.info(f"Discovering changes since {latest_tag}...")

    # Get changed files
    changed_files = git_ops.get_changed_files(latest_tag, args.chart_dir)

    # Get changed charts
    changed_charts = chart_manager.get_changed_charts(changed_files, args.version_pattern)

    if not changed_charts:
        logging.info("No changes detected.")
        return

    chart_names = [chart['name'] for chart in changed_charts]
    logging.info(f"The following charts have changed, and their releases will be deleted: {chart_names}")

    # Delete releases for each changed chart
    for chart in changed_charts:
        chart_name = chart['name']
        chart_version = chart['version']
        release_name = f"{chart_name}-{chart_version}"

        logging.info(f"Deleting release and tags for {release_name}...")

        # Delete release by tag
        release_manager.delete_release_by_tag(release_name)

        # Delete tag
        release_manager.delete_tag(release_name)

        # Clean up chart index
        index_manager.clean_specific_chart_index(chart_name, chart_version, args.pages_branch)

        logging.info(f"Deleted release for {chart_name}")


def main():
    """Main entry point"""
    # Configure logging
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )

    parser = argparse.ArgumentParser(
        description='Helm Chart Release Manager',
        formatter_class=argparse.RawDescriptionHelpFormatter
    )

    # Add global options
    parser.add_argument('--log-level',
                        choices=['DEBUG', 'INFO', 'WARNING', 'ERROR', 'CRITICAL'],
                        default='INFO',
                        help='Set the logging level (default: INFO)')

    subparsers = parser.add_subparsers(dest='command', help='Available commands')

    # Delete all releases command
    delete_all_parser = subparsers.add_parser('delete-all', help='Delete all releases from a repository')
    delete_all_parser.add_argument('-r', '--repository', required=True,
                                   help='GitHub repository (format: owner/repo)')
    delete_all_parser.add_argument('-f', '--force', action='store_true',
                                   help='Skip confirmation prompt')
    delete_all_parser.add_argument('-t', '--with-tags', action='store_true',
                                   help='Also delete associated tags for each release')
    delete_all_parser.add_argument('-i', '--clean-index', action='store_true',
                                   help='Also clean up chart index entries from the pages branch')
    delete_all_parser.add_argument('-p', '--pages-branch', default='gh-pages',
                                   help='The branch containing the chart index (default: gh-pages)')

    # Delete specific releases command
    delete_release_parser = subparsers.add_parser('delete-release', help='Delete specific chart releases')
    delete_release_parser.add_argument('-r', '--repository', required=True,
                                       help='GitHub repository (format: owner/repo)')
    delete_release_parser.add_argument('-d', '--chart-dir', default='charts',
                                       help='Directory containing Helm charts (default: charts)')
    delete_release_parser.add_argument('-b', '--base-branch', default='main',
                                       help='Base branch to compare changes against (default: main)')
    delete_release_parser.add_argument('-p', '--pages-branch', default='gh-pages',
                                       help='Branch containing the chart index (default: gh-pages)')
    delete_release_parser.add_argument('-v', '--version-pattern', default=r'^0\.0\.0-dev$',
                                       help=r'Regex pattern to match chart versions for deletion (default: ^0\.0\.0-dev$)')

    args = parser.parse_args()

    # Update logging level if specified
    if hasattr(args, 'log_level'):
        logging.getLogger().setLevel(getattr(logging, args.log_level))

    if not args.command:
        parser.print_help()
        sys.exit(1)

    # Execute the appropriate command
    if args.command == 'delete-all':
        delete_all_releases(args)
    elif args.command == 'delete-release':
        delete_specific_releases(args)


if __name__ == '__main__':
    main()
