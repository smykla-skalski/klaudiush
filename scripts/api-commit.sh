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

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

# File contents go into a request body file, not argv: Linux caps a single
# argument at 128 KiB, and the base64 of CHANGELOG.md alone exceeds that.
: >"$work_dir/files.jsonl"
while IFS= read -r f; do
  base64 -w0 "$f" >"$work_dir/contents"
  jq -cn --arg path "$f" --rawfile contents "$work_dir/contents" \
    '{path: $path, contents: $contents}' >>"$work_dir/files.jsonl"
done < <(git status --porcelain | awk '{print $2}' | xargs --no-run-if-empty -I{} find {} -type f 2>/dev/null)

if [ ! -s "$work_dir/files.jsonl" ]; then
  echo "No modified files to commit, skipping" >&2
  exit 0
fi

jq -s \
  --rawfile query "$QUERY_FILE" \
  --arg githubRepository "$GITHUB_REPOSITORY" \
  --arg branchName "$GITHUB_REF_NAME" \
  --arg expectedHeadOid "$(git rev-parse HEAD)" \
  --arg commitMessage "$COMMIT_MESSAGE" \
  '{query: $query, variables: {
      githubRepository: $githubRepository,
      branchName: $branchName,
      expectedHeadOid: $expectedHeadOid,
      commitMessage: $commitMessage,
      files: .
    }}' \
  "$work_dir/files.jsonl" >"$work_dir/body.json"

new_sha=$(
  gh api graphql \
    --jq '.data.createCommitOnBranch.commit.oid' \
    --input "$work_dir/body.json"
)

echo "Created verified commit: $new_sha" >&2

# Sync local HEAD so semantic-release tags the correct commit
git fetch origin "$GITHUB_REF_NAME"
git reset --hard "origin/$GITHUB_REF_NAME"
