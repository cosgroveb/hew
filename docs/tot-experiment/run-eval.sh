#!/usr/bin/env bash
# run-eval.sh — Run one eval configuration against a task fixture.
#
# Usage:
#   ./run-eval.sh <mode> <fixture-dir> <run-id> [--approaches "approach1|approach2|approach3"]
#
# Modes:
#   baseline      Single hu run with GPT-OSS
#   tot-split     Orchestrator creates worktrees + divergent TASK.md, GPT-OSS implements
#   tot-native    GPT-OSS orchestrates ToT itself via system prompt
#
# Results written to: docs/tot-experiment/results/<fixture-name>/<mode>/<run-id>/

set -euo pipefail

MODE="${1:?Usage: run-eval.sh <mode> <fixture-dir> <run-id>}"
FIXTURE_DIR="${2:?Usage: run-eval.sh <mode> <fixture-dir> <run-id>}"
RUN_ID="${3:?Usage: run-eval.sh <mode> <fixture-dir> <run-id>}"
APPROACHES="${5:-}"

FIXTURE_NAME=$(basename "$FIXTURE_DIR")
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RESULTS_DIR="$SCRIPT_DIR/results/${FIXTURE_NAME}/${MODE}/${RUN_ID}"
mkdir -p "$RESULTS_DIR"

export HEW_BASE_URL="http://localhost:8000/v1"
export HEW_API_KEY="not-needed"
export HEW_MODEL="gpt-oss-120b"

TASK_PROMPT=$(cat "$FIXTURE_DIR/README.md")

echo "=== $MODE | $FIXTURE_NAME | run $RUN_ID ==="
echo "Results: $RESULTS_DIR"

case "$MODE" in
  baseline)
    # Copy fixture to a working directory
    WORK_DIR=$(mktemp -d "/tmp/tot-eval-${FIXTURE_NAME}-XXXXXX")
    cp -r "$FIXTURE_DIR"/* "$WORK_DIR/"
    cd "$WORK_DIR"

    /usr/bin/time -o "$RESULTS_DIR/time.txt" -f "%e" \
      hu -p "$TASK_PROMPT" \
        --trajectory "$RESULTS_DIR/trajectory.json" \
        --event-log "$RESULTS_DIR/events.jsonl" \
        --max-steps 30 \
        2>"$RESULTS_DIR/stderr.txt" || true

    "$SCRIPT_DIR/score.sh" "$FIXTURE_DIR" "$WORK_DIR" "$RESULTS_DIR/trajectory.json" \
      > "$RESULTS_DIR/score.json"

    # Save the final state
    diff -ru "$FIXTURE_DIR" "$WORK_DIR" --exclude='.git' --exclude='go.sum' \
      > "$RESULTS_DIR/diff.patch" 2>/dev/null || true

    echo "Score: $(cat "$RESULTS_DIR/score.json")"
    rm -rf "$WORK_DIR"
    ;;

  tot-split)
    # Orchestrator (this script) creates 3 worktrees with divergent instructions.
    # Each approach gets a separate TASK.md with a specific strategy.
    # GPT-OSS implements each one independently.

    IFS='|' read -ra APPROACH_LIST <<< "$APPROACHES"
    NUM_APPROACHES=${#APPROACH_LIST[@]}

    if [ "$NUM_APPROACHES" -lt 2 ]; then
      echo "Error: tot-split requires --approaches 'approach1|approach2|approach3'"
      exit 1
    fi

    BEST_SCORE=-1
    BEST_IDX=-1

    for i in $(seq 0 $((NUM_APPROACHES - 1))); do
      BRANCH_DIR=$(mktemp -d "/tmp/tot-eval-${FIXTURE_NAME}-branch${i}-XXXXXX")
      cp -r "$FIXTURE_DIR"/* "$BRANCH_DIR/"

      # Write approach-specific instructions
      APPROACH="${APPROACH_LIST[$i]}"
      cat > "$BRANCH_DIR/APPROACH.md" << AEOF
# Implementation Approach

$APPROACH

## Instructions

Read README.md for the full problem statement.
Implement the solution using the approach described above.
Run \`make test\` to verify your work.
Do NOT modify files marked DO NOT MODIFY.
AEOF

      BRANCH_PROMPT="Read APPROACH.md and README.md in the current directory. Implement the solution using the specified approach. Run make test to verify. Only modify files you're allowed to change."

      cd "$BRANCH_DIR"
      echo "  Branch $i: ${APPROACH:0:60}..."

      /usr/bin/time -o "$RESULTS_DIR/branch-${i}-time.txt" -f "%e" \
        hu -p "$BRANCH_PROMPT" \
          --trajectory "$RESULTS_DIR/branch-${i}-trajectory.json" \
          --event-log "$RESULTS_DIR/branch-${i}-events.jsonl" \
          --max-steps 30 \
          2>"$RESULTS_DIR/branch-${i}-stderr.txt" || true

      "$SCRIPT_DIR/score.sh" "$FIXTURE_DIR" "$BRANCH_DIR" "$RESULTS_DIR/branch-${i}-trajectory.json" \
        > "$RESULTS_DIR/branch-${i}-score.json"

      diff -ru "$FIXTURE_DIR" "$BRANCH_DIR" --exclude='.git' --exclude='go.sum' \
        > "$RESULTS_DIR/branch-${i}-diff.patch" 2>/dev/null || true

      echo "  Branch $i score: $(cat "$RESULTS_DIR/branch-${i}-score.json")"

      # Track best by tests_passed
      PASSED=$(jq '.tests_passed' "$RESULTS_DIR/branch-${i}-score.json")
      if [ "$PASSED" -gt "$BEST_SCORE" ]; then
        BEST_SCORE=$PASSED
        BEST_IDX=$i
      fi

      rm -rf "$BRANCH_DIR"
    done

    echo "  Best branch: $BEST_IDX (${BEST_SCORE} tests passed)"
    cp "$RESULTS_DIR/branch-${BEST_IDX}-score.json" "$RESULTS_DIR/score.json"
    echo "$BEST_IDX" > "$RESULTS_DIR/winner.txt"
    ;;

  tot-native)
    # GPT-OSS tries to orchestrate ToT itself via system prompt.
    WORK_DIR=$(mktemp -d "/tmp/tot-eval-${FIXTURE_NAME}-XXXXXX")
    cp -r "$FIXTURE_DIR"/* "$WORK_DIR/"
    cd "$WORK_DIR"

    TOT_APPEND="For this task, explore multiple implementation approaches:
1. Generate 2-3 distinct strategies before implementing anything.
2. For each strategy, create a git branch, implement it, and run tests.
3. Compare results across branches and select the best one.
4. Apply the winning branch.
Use git worktree or git branch for isolation."

    /usr/bin/time -o "$RESULTS_DIR/time.txt" -f "%e" \
      hu -p "$TASK_PROMPT" \
        --system-prompt-append "$TOT_APPEND" \
        --trajectory "$RESULTS_DIR/trajectory.json" \
        --event-log "$RESULTS_DIR/events.jsonl" \
        --max-steps 50 \
        2>"$RESULTS_DIR/stderr.txt" || true

    "$SCRIPT_DIR/score.sh" "$FIXTURE_DIR" "$WORK_DIR" "$RESULTS_DIR/trajectory.json" \
      > "$RESULTS_DIR/score.json"

    diff -ru "$FIXTURE_DIR" "$WORK_DIR" --exclude='.git' --exclude='go.sum' \
      > "$RESULTS_DIR/diff.patch" 2>/dev/null || true

    echo "Score: $(cat "$RESULTS_DIR/score.json")"
    rm -rf "$WORK_DIR"
    ;;

  *)
    echo "Unknown mode: $MODE (use baseline, tot-split, or tot-native)"
    exit 1
    ;;
esac

echo "Done: $RESULTS_DIR"
