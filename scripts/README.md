# Helm Chart Release Manager

A Python tool for managing Helm Chart releases on GitHub, designed to replace the original shell scripts.

## Features

- **Delete all releases**: Delete all GitHub releases from a specified repository
- **Delete specific releases**: Delete specific chart releases based on git change detection and version pattern matching
- **Clean chart index**: Automatically clean up index.yaml files in the gh-pages branch
- **GitHub authentication**: Support for GH_TOKEN environment variable or gh CLI authentication
- **Version pattern matching**: Support for regex pattern matching to determine which versions can be deleted

## Installation

```bash
pip install -r requirements.txt
```

## Usage

### Delete all releases

```bash
# Delete all releases
python chart_release_manager.py delete-all --repository owner/repo

# Delete all releases and corresponding tags
python chart_release_manager.py delete-all --repository owner/repo --with-tags

# Delete all releases, tags, and clean chart index
python chart_release_manager.py delete-all --repository owner/repo --with-tags --clean-index

# Force delete, skip confirmation prompt
python chart_release_manager.py delete-all --repository owner/repo --force
```

### Delete specific releases

```bash
# Delete releases of changed charts
python chart_release_manager.py delete-release --repository owner/repo

# Specify chart directory
python chart_release_manager.py delete-release --repository owner/repo --chart-dir charts

# Specify version matching pattern
python chart_release_manager.py delete-release --repository owner/repo --version-pattern "^0\.0\.0-.*$"

# Specify base branch
python chart_release_manager.py delete-release --repository owner/repo --base-branch main
```

## Parameters

### Common Parameters

- `-r, --repository`: GitHub repository in format `owner/repo`
- `-p, --pages-branch`: Branch containing chart index, default is `gh-pages`

### delete-all Command Parameters

- `-f, --force`: Skip confirmation prompt
- `-t, --with-tags`: Also delete associated tags
- `-i, --clean-index`: Clean up chart index entries

### delete-release Command Parameters

- `-d, --chart-dir`: Helm charts directory, default is `charts`
- `-b, --base-branch`: Base branch for comparing changes, default is `main`
- `-v, --version-pattern`: Regex pattern for matching chart versions, default is `^0\.0\.0-dev$`

## Authentication

The script supports the following authentication methods:

1. **Environment Variable**: Set the `GH_TOKEN` environment variable
2. **GitHub CLI**: Use `gh auth login` to login

## Version Pattern Matching

The `--version-pattern` parameter allows you to specify which versions of charts can be deleted:

- `^0\.0\.0-dev$`: Only matches "0.0.0-dev"
- `^0\.0\.0-.*$`: Matches any version starting with "0.0.0-"
- `^.*-dev$`: Matches any version ending with "-dev"
- `^(0\.0\.0-dev|1\.0\.0-beta)$`: Matches "0.0.0-dev" or "1.0.0-beta"
- `.*`: Matches any version

## Error Handling

The script includes comprehensive error handling mechanisms:

- Detailed error messages when GitHub API requests fail
- Appropriate prompts when Git operations fail
- Exit with error message when version patterns don't match

## Differences from Original Shell Scripts

1. **Unified Command**: The original two scripts are merged into one, with functionality distinguished by subcommands
2. **Better Error Handling**: Provides more detailed error messages and exception handling
3. **Code Structure**: Uses object-oriented design, making code more maintainable
4. **Dependency Management**: Uses requirements.txt to manage Python dependencies
5. **Cross-platform**: Not dependent on specific shell environments, can run on various systems

## Examples

Usage in GitHub Actions:

```yaml
- name: Delete chart releases
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    python scripts/chart_release_manager.py delete-release \
      --repository ${{ github.repository }} \
      --version-pattern "^0\.0\.0-dev$"
```
