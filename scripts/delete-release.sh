#!/bin/bash
set -euo pipefail

set -x

SUPPORT_DELETE_RELEASE_VERSION="0.0.0-dev"

function main () {
  local usage="
  Usage: delete-release.sh [options]

  Delete a release for a Helm chart.

  Arguments:
    -r, --repository <repo>  The GitHub repository to look for Helm charts. Required.
    -d, --chart-dir <dir>      The directory to look for Helm charts. Default is 'charts'.
    -h, --help               Display
"

  local charts_dir="charts"
  local repository

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

  check_gh_login

  local latest_tag=$(lookup_latest_tag)
  echo "Discovering changes since $latest_tag"

  local changed_charts=()
  readarray -t changed_charts <<<"$(lookup_changed_charts "$latest_tag" "$charts_dir")"

  if [[ ${#changed_charts[@]} -eq 0 ]]; then
    echo "No changes detected." 1>&2
    exit 0
  fi

  echo "The following charts have changed, and their releases will be deleted: ${changed_charts[*]}" 1>&2
  for chart in "${changed_charts[@]}"; do
    delete_chart_release "$repository" "$chart"
    echo "Deleted release for $chart" 1>&2
  done


}

# check_gh_login checks if the user is logged in to GitHub.
# Check gh login status or GH_TOKEN in env
function check_gh_login() {
  if gh auth status >/dev/null 2>&1; then
    echo "GitHub CLI is authenticated." 1>&2
  elif [[ -n "$GH_TOKEN" ]]; then
    echo "Using GH_TOKEN from environment." 1>&2
  else
    echo "Error: Not authenticated with GitHub. Please login using 'gh auth login' or set GH_TOKEN in the environment." >&2
    exit 1
  fi
}

# lookup_latest_tag looks up the latest tag in the repository.
# Arguments:
#   None.
# Returns:
#   The latest tag in the repository.
function lookup_latest_tag() {
  git fetch --tags >/dev/null 2>&1

  if ! git describe --tags --abbrev=0 HEAD~ 2>/dev/null; then
    git rev-list --max-parents=0 --first-parent HEAD
  fi

}

# filter_charts filters out non-Helm charts from a list of directories.
# Arguments:
#   $1: A list of directories.
# Returns:
#   A list of directories that contain Helm charts.
function filter_charts() {
  while read -r chart; do
    [[ ! -d "$chart" ]] && continue
    local file="$chart/Chart.yaml"
    if [[ -f "$file" ]]; then
      # Check chart version support delete
      local chart_version=$(yq eval '.version' "$file")
      if [[ "$chart_version" == "$SUPPORT_DELETE_RELEASE_VERSION" ]]; then
        echo "$chart"
      else
        echo "Chart version $chart_version is not supported for deletion." >&2
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
# Returns:
#   A list of helm charts that have changed in the commit.
function lookup_changed_charts() {
  local commit="$1"
  local charts_dir="$2"
  local changed_files

  # Get the list of changed files in this commit.
  changed_files=$(git diff --find-renames --name-only "$commit" -- "$charts_dir")
  local depth=$(($(tr "/" "\n" <<<"$charts_dir" | sed '/^\(\.\)*$/d' | wc -l) + 1))
  # Get the list of changed charts.
  if [[ -n "$changed_files" ]]; then
    cut -d "/" -f "1-$depth" <<<"$changed_files" | uniq | sort -u | filter_charts
  else
    echo "No changed files found in commit $commit within directory $charts_dir." 1>&2
  fi
}

# delete_chart_release deletes a release for a Helm chart.
# Arguments:
#   $1: The repository name.
#   $2: The changed chart path.
# Returns:
#   None.
function delete_chart_release() {
  local repository="$1"
  local changed_chart="$2"

  local chart_name=$(basename "$changed_chart")
  local chart_version=$(yq eval '.version' "$changed_chart/Chart.yaml")
  local release_name="$chart_name-$chart_version"
  
  echo "Deleting release $release_name for $changed_chart..."
  # check chart_version support delete, if not support, raise error
  if [[ "$chart_version" != "$SUPPORT_DELETE_RELEASE_VERSION" ]]; then
    echo "WARNING: Release $release_name is not supported for deletion. Skipping." 1>&2
    return
  fi

  local release_id=$(gh api -X GET "repos/$repository/releases" | jq -r ".[] | select(.name | startswith(\"$release_name\")) | .id")
  if [[ -n "$release_id" ]]; then
    gh api -X DELETE "repos/$repository/releases/$release_id"
  else
    echo "No release found for $release_name." 1>&2
  fi
}

main "$@"
