#!/usr/bin/env bash
# score.sh — Score a hu eval run against a task fixture.
#
# Usage:
#   ./score.sh <fixture-dir> <worktree-dir> [trajectory.json]
#
# Outputs JSON to stdout with metrics:
#   tests_total, tests_passed, tests_failed, race_clean,
#   vet_clean, max_file_lines, package_count, diff_lines

set -euo pipefail

FIXTURE_DIR="${1:?Usage: score.sh <fixture-dir> <worktree-dir> [trajectory.json]}"
WORK_DIR="${2:?Usage: score.sh <fixture-dir> <worktree-dir> [trajectory.json]}"
TRAJECTORY="${3:-}"

cd "$WORK_DIR"

# --- Tests ---
test_output=$(go test ./... -v -count=1 -race 2>&1) || true
tests_total=$(echo "$test_output" | grep -cE '--- (PASS|FAIL):' || echo 0)
tests_passed=$(echo "$test_output" | grep -c '--- PASS:' || echo 0)
tests_failed=$(echo "$test_output" | grep -c '--- FAIL:' || echo 0)

# Race detector
if echo "$test_output" | grep -q 'DATA RACE'; then
    race_clean=false
else
    race_clean=true
fi

# --- Vet ---
if go vet ./... 2>&1 | grep -q .; then
    vet_clean=false
else
    vet_clean=true
fi

# --- Metrics ---
max_file_lines=$(find . -name '*.go' -not -path './vendor/*' -exec wc -l {} \; \
    | awk '{print $1}' | sort -rn | head -1 || echo 0)

package_count=$(find . -name '*.go' -not -path './vendor/*' \
    -exec dirname {} \; | sort -u | wc -l)

# Diff from fixture (lines changed)
if [ -d "$FIXTURE_DIR/.git" ]; then
    diff_lines=$(diff -rq "$FIXTURE_DIR" "$WORK_DIR" --exclude='.git' --exclude='go.sum' \
        2>/dev/null | wc -l || echo 0)
else
    diff_lines=$(diff -ru "$FIXTURE_DIR" "$WORK_DIR" --exclude='.git' --exclude='go.sum' \
        2>/dev/null | grep -cE '^\+|^-' || echo 0)
fi

# Trajectory stats
if [ -n "$TRAJECTORY" ] && [ -f "$TRAJECTORY" ]; then
    steps=$(( $(jq 'length' "$TRAJECTORY") / 2 ))
else
    steps="null"
fi

# --- Output ---
cat <<EOF
{
  "tests_total": $tests_total,
  "tests_passed": $tests_passed,
  "tests_failed": $tests_failed,
  "race_clean": $race_clean,
  "vet_clean": $vet_clean,
  "max_file_lines": $max_file_lines,
  "package_count": $package_count,
  "diff_lines": $diff_lines,
  "steps": $steps
}
EOF
