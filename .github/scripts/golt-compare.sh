#!/usr/bin/env bash
# Exits with golt's cold-run status so the job fails like a regular lint job;
# golangci-lint results are only reported for comparison.
set -euo pipefail

out="${RUNNER_TEMP:-/tmp}/golt-compare"
summary="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
rm -rf "${out}"
mkdir -p "${out}"

run() {
  local name=$1 bin=$2 mode=$3
  shift 3

  local cache="${out}/cache-${name}"
  if [[ "${mode}" == "cold" ]]; then
    rm -rf "${cache}"
  fi

  local status=0 start end
  start=$(date +%s.%N)
  GOLANGCI_LINT_CACHE="${cache}" "${bin}" run \
    --output.json.path="${out}/${name}-${mode}.json" \
    --output.text.path=stderr \
    "$@" 2>"${out}/${name}-${mode}.log" || status=$?
  end=$(date +%s.%N)

  local issues="-"
  if [[ -s "${out}/${name}-${mode}.json" ]]; then
    issues=$(jq '.Issues | length' "${out}/${name}-${mode}.json")
  fi

  awk -v n="${name}" -v m="${mode}" -v s="${start}" -v e="${end}" -v i="${issues}" -v c="${status}" \
    'BEGIN { printf "| %s | %s | %.2f | %s | %s |\n", n, m, e - s, i, c }' >>"${summary}"
  echo "${status}" >"${out}/${name}-${mode}.status"
}

issue_lines() {
  jq -r '.Issues[]? | "\(.FromLinter) \(.Pos.Filename):\(.Pos.Line): \(.Text)"' "$1" | sort
}

{
  echo "## golt vs golangci-lint"
  echo
  echo "golt: \`$("${GOLT_BIN}" version --short 2>/dev/null || echo unknown)\`, golangci-lint: \`$("${GOLANGCI_BIN}" version --short 2>/dev/null || echo unknown)\`"
  echo
  echo "| binary | run | wall (s) | issues | exit |"
  echo "|---|---|---|---|---|"
} >>"${summary}"

for mode in cold warm; do
  run golangci-lint "${GOLANGCI_BIN}" "${mode}"
  run golt "${GOLT_BIN}" "${mode}"
done

if [[ -n "${MERGE_BASE:-}" ]]; then
  run golangci-lint "${GOLANGCI_BIN}" new --new-from-merge-base="${MERGE_BASE}"
  run golt "${GOLT_BIN}" new --new-from-merge-base="${MERGE_BASE}"
fi

if [[ -s "${out}/golt-cold.json" && -s "${out}/golangci-lint-cold.json" ]]; then
  issue_lines "${out}/golangci-lint-cold.json" >"${out}/golangci-lint.issues"
  issue_lines "${out}/golt-cold.json" >"${out}/golt.issues"
  {
    echo
    if diff -q "${out}/golangci-lint.issues" "${out}/golt.issues" >/dev/null; then
      echo "Issues identical."
    else
      echo "<details><summary>Issue differences (golangci-lint → golt)</summary>"
      echo
      echo '```diff'
      diff "${out}/golangci-lint.issues" "${out}/golt.issues" || true
      echo '```'
      echo "</details>"
    fi
  } >>"${summary}"
fi

sed 's/^/golt: /' "${out}/golt-cold.log" >&2
exit "$(cat "${out}/golt-cold.status")"
