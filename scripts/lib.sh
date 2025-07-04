#!/bin/bash
# lib.sh - Common library functions for Helm chart release management

# prepare_pages_branch prepares the pages branch for index operations.
# Arguments:
#   $1: The pages branch name.
# Returns:
#   0 on success, 1 on failure.
#   Sets global variable ORIGINAL_BRANCH with the current branch name.
function prepare_pages_branch() {
  local pages_branch="$1"

  # Save current branch
  ORIGINAL_BRANCH=$(git rev-parse --abbrev-ref HEAD)

  # Check if pages branch exists
  if ! git ls-remote --heads origin "$pages_branch" | grep -q "$pages_branch"; then
    echo "$pages_branch branch does not exist. Skipping index cleanup." 1>&2
    return 1
  fi

  # Fetch latest changes from remote
  git fetch origin "$pages_branch" >/dev/null 2>&1 || {
    echo "Warning: Failed to fetch $pages_branch branch from remote" >&2
    return 1
  }

  # Switch to pages branch
  if ! git checkout "$pages_branch" >/dev/null 2>&1; then
    echo "Error: Failed to checkout $pages_branch branch" >&2
    return 1
  fi

  # Pull latest changes
  git pull origin "$pages_branch" >/dev/null 2>&1 || {
    echo "Warning: Failed to pull latest changes from $pages_branch" >&2
  }

  # Check if index.yaml exists
  if [[ ! -f "index.yaml" ]]; then
    echo "index.yaml not found in $pages_branch branch. Skipping index cleanup." 1>&2
    restore_original_branch
    return 1
  fi

  return 0
}

# commit_and_push_index commits and pushes changes to index.yaml.
# Arguments:
#   $1: The pages branch name.
#   $2: The commit message.
# Returns:
#   0 on success, 1 on failure.
function commit_and_push_index() {
  local pages_branch="$1"
  local commit_message="$2"

  # Check if there are any changes to commit
  if git diff --quiet index.yaml; then
    echo "No changes to commit in index.yaml" 1>&2
    return 0
  fi

  # Commit the changes
  git add index.yaml
  git commit -m "$commit_message"

  # Push the changes
  if git push origin "$pages_branch" >/dev/null 2>&1; then
    echo "Successfully pushed index.yaml changes to $pages_branch branch"
    return 0
  else
    echo "Warning: Failed to push changes to $pages_branch branch" >&2
    return 1
  fi
}

# restore_original_branch switches back to the original branch.
# Uses global variable ORIGINAL_BRANCH set by prepare_pages_branch.
# Returns:
#   0 on success, 1 on failure.
function restore_original_branch() {
  if [[ -n "${ORIGINAL_BRANCH:-}" ]]; then
    git checkout "$ORIGINAL_BRANCH" >/dev/null 2>&1 || {
      echo "Error: Failed to checkout back to $ORIGINAL_BRANCH" >&2
      return 1
    }
  fi
  return 0
}

# clean_specific_chart_index cleans up a specific chart index entry from the pages branch.
# Arguments:
#   $1: The chart name.
#   $2: The chart version.
#   $3: The pages branch name.
# Returns:
#   0 on success, 1 on failure.
function clean_specific_chart_index() {
  local chart_name="$1"
  local chart_version="$2"
  local pages_branch="$3"

  echo "Cleaning up chart index for $chart_name version $chart_version..."

  # Prepare pages branch
  if ! prepare_pages_branch "$pages_branch"; then
    return 1
  fi

  # Remove the chart entry from index.yaml
  local temp_file="index.yaml.tmp"
  if yq eval "del(.entries.\"$chart_name\"[] | select(.version == \"$chart_version\"))" index.yaml > "$temp_file"; then
    mv "$temp_file" index.yaml
    echo "Removed $chart_name version $chart_version from index.yaml"

    # Commit and push changes
    commit_and_push_index "$pages_branch" "Remove $chart_name version $chart_version from index"
  else
    echo "Error: Failed to remove chart entry from index.yaml" >&2
    rm -f "$temp_file"
    restore_original_branch
    return 1
  fi

  # Switch back to original branch
  restore_original_branch
  return 0
}

# clean_all_chart_index cleans up all chart index entries from the pages branch.
# Arguments:
#   $1: The pages branch name.
# Returns:
#   0 on success, 1 on failure.
function clean_all_chart_index() {
  local pages_branch="$1"

  echo "Cleaning up all chart index entries from $pages_branch branch..."

  # Prepare pages branch
  if ! prepare_pages_branch "$pages_branch"; then
    return 1
  fi

  # Clear all entries from index.yaml
  local temp_file="index.yaml.tmp"
  if yq eval '.entries = {}' index.yaml > "$temp_file"; then
    mv "$temp_file" index.yaml
    echo "Cleared all entries from index.yaml"

    # Commit and push changes
    commit_and_push_index "$pages_branch" "Clear all chart entries from index"
  else
    echo "Error: Failed to clear chart entries from index.yaml" >&2
    rm -f "$temp_file"
    restore_original_branch
    return 1
  fi

  # Switch back to original branch
  restore_original_branch
  return 0
}

# check_gh_login checks if the user is logged in to GitHub.
# Check gh login status or GH_TOKEN in env
# Returns:
#   0 on success, 1 on failure.
function check_gh_login() {
  if gh auth status >/dev/null 2>&1; then
    echo "GitHub CLI is authenticated." 1>&2
  elif [[ -n "${GH_TOKEN:-}" ]]; then
    echo "Using GH_TOKEN from environment." 1>&2
  else
    echo "Error: Not authenticated with GitHub. Please login using 'gh auth login' or set GH_TOKEN in the environment." >&2
    return 1
  fi
  return 0
}
