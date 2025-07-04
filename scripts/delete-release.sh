#!/bin/bash
set -euo pipefail

set -x

# Load common library functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib.sh"

# Regular expression pattern to match chart versions that are supported for deletion
# Examples:
# - "^0\.0\.0-dev$" matches exactly "0.0.0-dev"
# - "^0\.0\.0-.*$" matches any version starting with "0.0.0-"
# - "^.*-dev$" matches any version ending with "-dev"
# - "^(0\.0\.0-dev|1\.0\.0-beta)$" matches either "0.0.0-dev" or "1.0.0-beta"
SUPPORTED_DELETE_RELEASE_VERSION_PATTERN="^0\.0\.0-dev$"

function main () {
  local usage="
  Usage: delete-release.sh [options]

  Delete a release for a Helm chart and clean up the chart index.

  This script will:
  1. Delete the GitHub release
  2. Delete associated tags from the remote repository
  3. Clean up the chart index from the gh-pages branch

  Arguments:
    -r, --repository <repo>       The GitHub repository to look for Helm charts. Required.
    -d, --chart-dir <dir>         The directory to look for Helm charts. Default is 'charts'.
    -b, --base-branch <branch>    The base branch to compare changes against. Default is 'main'.
    -p, --pages-branch <branch>   The branch containing the chart index. Default is 'gh-pages'.
    -v, --version-pattern <regex> Regular expression pattern to match chart versions for deletion.
                                  Default is '^0\.0\.0-dev$'.
    -h, --help                    Display this help message.
"

  local charts_dir="charts"
  local repository
  local base_branch="main"
  local pages_branch="gh-pages"
  local version_pattern="$SUPPORTED_DELETE_RELEASE_VERSION_PATTERN"

  while [[ $# -gt 0 ]]; do
    case $1 in
      -r|--repository)
        repository=$2
        shift 2
        ;;
      -d|--chart-dir)
        charts_dir=$2
        shift 2
        ;;
      -b|--base-branch)
        base_branch=$2
        shift 2
        ;;
      -p|--pages-branch)
        pages_branch=$2
        shift 2
        ;;
      -v|--version-pattern)
        version_pattern=$2
        shift 2
        ;;
      -h|--help)
        echo "$usage"
        exit 0
        ;;
      *)
        echo "Unknown argument: $1"
        echo "$usage"
        exit 1
        ;;
    esac
  done

  if [[ -z "$repository" ]]; then
    echo "Repository is required."
    echo "$usage"
    exit 1
  fi

  check_gh_login || exit 1

  local latest_tag=$(lookup_latest_tag "$base_branch")
  echo "Discovering changes since $latest_tag..."

  local changed_charts=()
  readarray -t changed_charts <<<"$(lookup_changed_charts "$latest_tag" "$charts_dir" "$version_pattern")"

  if [[ ${#changed_charts[@]} -eq 0 ]]; then
    echo "No changes detected." 1>&2
    exit 0
  fi

  echo "The following charts have changed, and their releases will be deleted: ${changed_charts[*]}" 1>&2
  for chart in "${changed_charts[@]}"; do
    delete_chart_release "$repository" "$chart" "$pages_branch" "$version_pattern"
    echo "Deleted release for $chart" 1>&2
  done

}

# lookup_latest_tag looks up the latest tag in the repository.
# If the current branch is the base branch, it looks up the latest tag or first commit.
# If the current branch is not the base branch, it looks up the latest tag or merge base commit.
# Arguments:
#   $1: The base branch name to compare against.
# Returns:
#   The latest tag in the repository or the commit hash if no tags are found.
function lookup_latest_tag() {
  local base_branch="$1"

  # Ensure local tags are up-to-date
  git fetch --tags >/dev/null 2>&1 || {
    echo "Warning: Failed to fetch tags from remote" >&2
  }

  local current_branch
  current_branch=$(git rev-parse --abbrev-ref HEAD)

  # First, try to get the latest tag
  local tag_or_commit
  tag_or_commit=$(git describe --tags --abbrev=0 HEAD~ 2>/dev/null)

  # If no tag is found, decide which commit to use based on the branch
  if [[ -z "$tag_or_commit" ]]; then
    if [[ "$current_branch" == "$base_branch" ]]; then
      # On the base branch, use the first commit
      tag_or_commit=$(git rev-list --max-parents=0 --first-parent HEAD)
    else
      # On other branches, use the merge base with the base branch
      tag_or_commit=$(git merge-base HEAD "$base_branch") || {
        echo "Error: Could not find merge base with $base_branch" >&2
        exit 1
      }
    fi
  fi

  echo "$tag_or_commit"
}

# filter_charts filters out non-Helm charts from a list of directories.
# Arguments:
#   $1: A list of directories.
#   $2: Version pattern to match for deletion support.
# Returns:
#   A list of directories that contain Helm charts.
function filter_charts() {
  local version_pattern="$1"
  while read -r chart; do
    [[ ! -d "$chart" ]] && continue
    local file="$chart/Chart.yaml"
    if [[ -f "$file" ]]; then
      # Check chart version support delete using regex pattern
      local chart_version=$(yq eval '.version' "$file")
      if [[ "$chart_version" =~ $version_pattern ]]; then
        echo "$chart"
      else
        echo "Chart version $chart_version does not match supported pattern for deletion: $version_pattern" >&2
        exit 1
      fi
    else
      echo "WARNING: $file is missing, assuming that '$chart' is not a Helm chart. Skipping." 1>&2
    fi
  done
}

# lookupd_change_charts looks up the Helm charts that have changed in a commit.
# Arguments:
#   $1: The commit hash.
#   $2: The directory to look for Helm charts.
#   $3: Version pattern to match for deletion support.
# Returns:
#   A list of helm charts that have changed in the commit.
function lookup_changed_charts() {
  local commit="$1"
  local charts_dir="$2"
  local version_pattern="$3"
  local changed_files

  # Get the list of changed files in this commit.
  changed_files=$(git diff --find-renames --name-only "$commit" -- "$charts_dir")
  local depth=$(($(tr "/" "\n" <<<"$charts_dir" | sed '/^\(\.\)*$/d' | wc -l) + 1))
  # Get the list of changed charts.
  if [[ -n "$changed_files" ]]; then
    cut -d "/" -f "1-$depth" <<<"$changed_files" | uniq | sort -u | filter_charts "$version_pattern"
  else
    echo "No changed files found in commit $commit within directory $charts_dir." 1>&2
  fi
}

# delete_chart_release deletes a release for a Helm chart.
# Arguments:
#   $1: The repository name.
#   $2: The changed chart path.
#   $3: The pages branch name.
#   $4: Version pattern to match for deletion support.
# Returns:
#   None.
function delete_chart_release() {
  local repository="$1"
  local changed_chart="$2"
  local pages_branch="$3"
  local version_pattern="$4"

  local chart_name=$(basename "$changed_chart")
  local chart_version=$(yq eval '.version' "$changed_chart/Chart.yaml")
  local release_name="$chart_name-$chart_version"

  echo "Deleting release and tags for $release_name..."
  # check chart_version support delete using regex pattern
  if [[ ! "$chart_version" =~ $version_pattern ]]; then
    echo "WARNING: Release $release_name version does not match supported pattern for deletion: $version_pattern. Skipping." 1>&2
    return
  fi

  # Delete release
  local release_id=$(gh api -X GET "repos/$repository/releases" | jq -r ".[] | select(.name | startswith(\"$release_name\")) | .id")
  if [[ -n "$release_id" ]]; then
    if ! gh api -X DELETE "repos/$repository/releases/$release_id"; then
      echo "Failed to delete release $release_name" 1>&2
      return 1
    fi
    echo "Successfully deleted release $release_name"
  else
    echo "No release found for $release_name" 1>&2
  fi

  # Delete tags from remote
  for tag in "$release_name" "$chart_version"; do
    if gh api -X DELETE "repos/$repository/git/refs/tags/$tag" 2>/dev/null; then
      echo "Successfully deleted remote tag $tag"
    else
      echo "No remote tag found for $tag or failed to delete" 1>&2
    fi
  done

  # Clean up chart index from pages branch
  clean_specific_chart_index "$chart_name" "$chart_version" "$pages_branch"
  echo "Cleaned up chart index for $chart" 1>&2
}

main "$@"
