#!/usr/bin/env bash
set -euo pipefail

BINARY="${1:-./bin/klaudiush}"
PAYLOADS="cmd/klaudiush/testdata/bench-payloads"
REPORT_DIR="/tmp/klaudiush-bench"

# Check prerequisites
command -v hyperfine >/dev/null || { echo "install hyperfine: brew install hyperfine"; exit 1; }
[[ -x "$BINARY" ]] || { echo "build first: mise run build"; exit 1; }

# Create temp git repo for git-related payloads
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

git -C "$TMPDIR" init -b main >/dev/null 2>&1
git -C "$TMPDIR" config user.email "bench@test.local"
git -C "$TMPDIR" config user.name "Bench"
echo "package main" > "$TMPDIR/main.go"
git -C "$TMPDIR" add main.go

mkdir -p "$REPORT_DIR"

echo "Running end-to-end benchmarks with hyperfine..."
echo "Binary: $BINARY"
echo "Git repo: $TMPDIR"
echo ""

hyperfine \
  --warmup 3 \
  --runs 30 \
  --export-markdown "$REPORT_DIR/hyperfine.md" \
  --export-json "$REPORT_DIR/hyperfine.json" \
  -n "empty" \
    "echo '{}' | HOME=$TMPDIR KLAUDIUSH_USE_SDK_GIT=false $BINARY --hook-type PreToolUse" \
  -n "non-git bash" \
    "cat $PAYLOADS/non-git-bash.json | HOME=$TMPDIR KLAUDIUSH_USE_SDK_GIT=false $BINARY --hook-type PreToolUse" \
  -n "write tool" \
    "cat $PAYLOADS/write-tool.json | HOME=$TMPDIR KLAUDIUSH_USE_SDK_GIT=false $BINARY --hook-type PreToolUse" \
  -n "git commit pass" \
    "cat $PAYLOADS/git-commit-pass.json | HOME=$TMPDIR KLAUDIUSH_USE_SDK_GIT=false $BINARY --hook-type PreToolUse" \
  -n "git commit fail" \
    "cat $PAYLOADS/git-commit-fail.json | HOME=$TMPDIR KLAUDIUSH_USE_SDK_GIT=false $BINARY --hook-type PreToolUse" \
  -n "git push" \
    "cat $PAYLOADS/git-push.json | HOME=$TMPDIR KLAUDIUSH_USE_SDK_GIT=false $BINARY --hook-type PreToolUse"

echo ""
echo "Reports saved to $REPORT_DIR/"
echo "  hyperfine.md   - markdown table"
echo "  hyperfine.json - raw data"
