#!/usr/bin/env bash
# Creates a verified commit via GitHub GraphQL API.
# Called by semantic-release (via @semantic-release/exec) during the prepare phase.
# Commits created with a GitHub App token through this API are automatically "Verified".
#
# Usage: api-commit.sh <commit-message>
# Required env vars: GITHUB_REPOSITORY, GITHUB_REF_NAME, GITHUB_TOKEN

set -euo pipefail

COMMIT_MESSAGE="${1:?Usage: api-commit.sh <commit-message>}"

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"
: "${GITHUB_REF_NAME:?GITHUB_REF_NAME must be set}"
: "${GITHUB_TOKEN:?GITHUB_TOKEN must be set}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
QUERY_FILE="$REPO_ROOT/.github/api/queries/createCommit.graphql"

if [ ! -f "$QUERY_FILE" ]; then
  echo "Error: GraphQL query file not found: $QUERY_FILE" >&2
  exit 1
fi

# Collect modified and untracked files
files=()
while IFS= read -r f; do
  files+=(--field "files[][path]=$f")
  files+=(--field "files[][contents]=$(base64 -w0 "$f")")
done < <(git status --porcelain | awk '{print $2}' | xargs --no-run-if-empty -I{} find {} -type f 2>/dev/null)

if [ ${#files[@]} -eq 0 ]; then
  echo "No modified files to commit, skipping" >&2
  exit 0
fi

# Create verified commit via GraphQL API
new_sha=$(
  gh api graphql \
    --jq '.data.createCommitOnBranch.commit.oid' \
    --field "query=@$QUERY_FILE" \
    --field "githubRepository=$GITHUB_REPOSITORY" \
    --field "branchName=$GITHUB_REF_NAME" \
    --field "expectedHeadOid=$(git rev-parse HEAD)" \
    --field "commitMessage=$COMMIT_MESSAGE" \
    "${files[@]}"
)

echo "Created verified commit: $new_sha" >&2

# Sync local HEAD so semantic-release tags the correct commit
git fetch origin "$GITHUB_REF_NAME"
git reset --hard "origin/$GITHUB_REF_NAME"
