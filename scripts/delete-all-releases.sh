#!/bin/bash
set -euo pipefail

# Delete the specified release and its associated tags
function delete_release() {
  local repository="$1"
  local release="$2"
  local delete_tags="$3"
  local release_id=$(echo "$release" | jq -r '.id')
  local release_name=$(echo "$release" | jq -r '.name')
  local release_tag=$(echo "$release" | jq -r '.tag_name')

  # Delete release
  if gh api -X DELETE "repos/$repository/releases/$release_id" 2>/dev/null; then
    echo "Successfully deleted release: $release_name"
  else
    echo "Failed to delete release: $release_name" >&2
    return 1
  fi

  # If delete tags is specified and the tag exists
  if [[ "$delete_tags" == "true" && -n "$release_tag" ]]; then
    if gh api -X DELETE "repos/$repository/git/refs/tags/$release_tag" 2>/dev/null; then
      echo "Successfully deleted associated tag: $release_tag"
    else
      echo "Failed to delete associated tag: $release_tag" >&2
    fi
  fi
}

function main() {
  local usage="
  Usage: delete-all-releases.sh [options]

  Delete all releases from a GitHub repository, optionally with their associated tags.

  Arguments:
    -r, --repository <repo>    The GitHub repository (format: owner/repo). Required.
    -f, --force               Skip confirmation prompt.
    -t, --with-tags          Also delete associated tags for each release.
    -h, --help                Display this help message.
"

  local repository=""
  local force=false
  local delete_tags=false

  while [[ $# -gt 0 ]]; do
    case $1 in
      -r|--repository)
        repository=$2
        shift 2
        ;;
      -f|--force)
        force=true
        shift
        ;;
      -t|--with-tags)
        delete_tags=true
        shift
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
    echo "Error: Repository is required."
    echo "$usage"
    exit 1
  fi

  # Confirm GitHub authentication
  if ! gh auth status >/dev/null 2>&1 && [[ -z "${GH_TOKEN:-}" ]]; then
    echo "Error: Not authenticated with GitHub. Please run 'gh auth login' or set GH_TOKEN." >&2
    exit 1
  fi

  # Get all releases
  local releases
  releases=$(gh api "repos/$repository/releases")

  if [[ $(echo "$releases" | jq '. | length') -eq 0 ]]; then
    echo "No releases found in repository $repository"
    exit 0
  fi

  # Display the content to be deleted
  echo "The following releases will be deleted from $repository:"
  echo "$releases" | jq -r '.[] | "- \(.name) (tag: \(.tag_name))"'
  echo "Total releases to delete: $(echo "$releases" | jq '. | length')"

  if [[ "$delete_tags" == "true" ]]; then
    echo "Associated tags will also be deleted."
  fi

  # If not in force mode, request confirmation
  if [[ "$force" != "true" ]]; then
    read -p "Are you sure you want to proceed? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
      echo "Operation cancelled."
      exit 0
    fi
  fi

  # Batch delete releases
  echo "$releases" | jq -c '.[]' | while read -r release; do
    delete_release "$repository" "$release" "$delete_tags"
  done
}

main "$@"
