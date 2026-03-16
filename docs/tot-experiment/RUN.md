# Running the ToT Experiment with Claude-backed hew

## Prerequisites

```bash
# Install hu
cd ~/code/hew && make install

# Configure for Anthropic
source ~/.secrets.zsh
export HEW_API_KEY="$ANTHROPIC_API_KEY"

# Verify
hu --version
hu --dump-system-prompt | tail -5
```

## One-liner

```bash
cd ~/code/hew/docs/tot-experiment/fixtures && hu -p "$(cat <<'PROMPT'
You are running a Tree of Thought (ToT) evaluation across 3 Go project fixtures
in the current directory: task1-ratelimiter/, task2-refactor/, task3-multibug/.

For EACH fixture, do the following:

1. Read the fixture's README.md to understand the task.

2. Generate 3 structurally different implementation approaches. Not cosmetic
   variations — each approach should use a fundamentally different algorithm
   or decomposition strategy. Write a one-paragraph description of each.

3. For each approach, create a temp work directory and copy the fixture into it:
   ```
   WORK=$(mktemp -d /tmp/tot-TASKNAME-branchN-XXXXXX)
   cp -r FIXTURE_DIR/* "$WORK/"
   ```

4. Write an APPROACH.md into each work directory describing the specific strategy.

5. Spawn a child hu process for each branch. Children run SEQUENTIALLY (not backgrounded):
   ```
   cd "$WORK" && hu -p "Read APPROACH.md and README.md. Implement the solution \
   using the specified approach. Run make test to verify. Only modify files you \
   are allowed to change." --trajectory /tmp/tot-TASK-branch-N.json --max-steps 30
   ```

6. After all 3 children complete, score each branch:
   ```
   cd "$WORK" && go test ./... -v -count=1 -race 2>&1 | grep -cE '^--- PASS:'
   ```

7. Select the branch with the most passing tests as the winner.

8. Print a summary table:
   ```
   Task                 Branch 0    Branch 1    Branch 2    Winner
   task1-ratelimiter    13/13       0/13        13/13       0
   task2-refactor       15/15       15/15       12/15       0
   task3-multibug       8/8         5/8         8/8         0
   ```

9. Clean up temp directories.

After completing all 3 tasks, print the full results and say <done/>.

IMPORTANT:
- Run children SEQUENTIALLY. Do not use & or background processes.
- Each child is a separate hu process — do not implement the solutions yourself.
- The child hu will use the same model and API key you are using.
- Do not skip any task. Run all 3.
PROMPT
)" --trajectory /tmp/tot-eval-full.json --event-log /tmp/tot-eval-events.jsonl --max-steps 100
```

## What to expect

The parent hu instance should:
1. Read each fixture's README.md
2. Generate 3 approaches per task (9 total)
3. Spawn 9 child hu processes sequentially
4. Score each child's work
5. Report a summary table

Total time: ~30-60 minutes depending on model speed.

## Scoring the results

```bash
# Extract the summary table from the trajectory
jq -r '.[-2].content' /tmp/tot-eval-full.json

# Or check individual branch trajectories
for f in /tmp/tot-*-branch-*.json; do
  echo "=== $f ==="
  jq -r '.[-1].content' "$f" 2>/dev/null | tail -5
done
```

## Comparison with initial experiment

The initial experiment (2026-03-16) used hardcoded approaches and a Python
orchestrator. This run uses Claude to generate approaches dynamically and
hu for orchestration. Compare:

| Metric | Initial (hardcoded) | This run (dynamic) |
|--------|--------------------|--------------------|
| Approach generation | Human-designed | Claude-generated |
| Orchestration | Python script | hu (Claude-backed) |
| Children | GPT-OSS | Claude (same model) |
| Winner selection | Script (test count) | hu (test count) |
