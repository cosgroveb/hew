# ToT (Tree of Thought) Experiment

## Goal

Evaluate whether Tree of Thought — parallel exploration of multiple
implementation strategies with evaluation and selection — improves
coding agent outcomes on ambiguous tasks compared to single-path execution.

## Naming

- **hew** is the project name and the TUI binary (Bubble Tea)
- **hu** is the plain CLI binary — this is what you run and what child processes use
- Both share the same flags and system prompt

## How hu works

1. User provides a task via `-p "task"`
2. hu sends the task + system prompt to the LLM
3. LLM responds with reasoning + one or more ```bash code blocks
4. hu executes ALL bash blocks sequentially, captures combined stdout/stderr
5. Output is appended to conversation history as the next user message
6. Loop repeats until LLM emits `<done/>` or `--max-steps` is hit

System prompt is composed from: base prompt (hardcoded) + ~/AGENTS.md +
~/.config/hew/AGENTS.md + ./AGENTS.md (working directory).
Use `hu --dump-system-prompt` to see the composed prompt.

## How ToT works in hew

ToT emerges from hew's existing primitives — no new code is needed:

1. **Research phase**: Parent hu researches the problem (normal agent loop)
2. **Branch generation**: Parent asks the LLM to generate N distinct approaches
3. **Isolation**: Parent creates N git worktrees via bash
4. **Parallel exploration**: Parent spawns `hu -p "implement approach X"
   --load-messages research.json --trajectory /tmp/child-N.json` in each worktree
5. **Evaluation**: Parent collects trajectories and diffs, asks LLM to evaluate
   with anchored rubrics and forced ranking
6. **Selection**: Parent applies the winning worktree

Key properties:
- Children are seeded with shared research context via `--load-messages`
- Each child works in an isolated git worktree (divergent state)
- Children get different strategy prompts (divergent reasoning)
- Evaluation is convergent: compare diffs, test results, complexity
- Everything is bash commands — no new Go code or orchestration types

## Key flags

```
-p, --prompt string              Task to run
-m, --model string               Model identifier
-u, --base-url string            LLM endpoint
    --max-steps int              Maximum agent steps (default: 100)
    --load-messages string       Seed conversation from a JSON file
    --trajectory string          Save message history as JSON on exit
    --event-log string           Stream JSONL events to file
-S, --no-system-prompt           Skip the built-in system prompt entirely
    --system-prompt-append string  Append text to the built-in system prompt
    --dump-system-prompt         Print the composed system prompt and exit
```

## Evaluation methodology

### What makes a good ToT eval task?

Unlike RLM (which tests decomposition over large data), ToT tests
**strategy selection on ambiguous problems**. Good ToT tasks have:

- Multiple valid implementation approaches (not just one right answer)
- Approaches that differ structurally (not just cosmetic variations)
- Measurable quality differences between approaches (tests, perf, complexity)
- A clear "best" answer that a single-path agent might miss

### Proposed eval tasks

**Task 1: Implement a rate limiter**
Given a Go project skeleton with tests, implement a rate limiter.
Valid approaches: token bucket, sliding window, fixed window, leaky bucket.
Evaluation: tests pass, throughput benchmark, code complexity.

**Task 2: Refactor a coupled module**
Given a Go module with high coupling (circular imports, god objects),
refactor to reduce coupling. Multiple valid decomposition strategies.
Evaluation: tests pass, dependency graph metrics, diff size.

**Task 3: Fix a bug with multiple root causes**
A test suite with 3 failing tests that have overlapping but distinct
root causes. Single-path agents tend to fix one cause and break another.
Evaluation: all tests pass, minimal diff, no regressions.

### Scoring

For each task, compare:
- **Baseline**: Single hu run (no ToT)
- **ToT-2**: 2 branches, evaluate, select best
- **ToT-3**: 3 branches, evaluate, select best

Metrics:
- **Correctness**: Do tests pass? (binary gate)
- **Quality**: Benchmark scores, complexity metrics, diff size
- **Cost**: Total LLM calls (parent + all children)
- **Time**: Wall clock

## Setup

```bash
# Ensure hu is installed
cd ~/code/hew && make install  # or go install ./cmd/hu/

# Configure for your provider
export HEW_API_KEY="$ANTHROPIC_API_KEY"
# Or for local models:
# export HEW_BASE_URL="http://localhost:8000/v1"
# export HEW_MODEL="gpt-oss-120b"

# Verify
hu --version
hu --dump-system-prompt | tail -20
```

## Lessons from RLM experiment

The RLM experiment (see `docs/rlm-experiment/`) established several
principles that apply to ToT:

1. **Model capability matters.** Claude Opus followed RLM instructions
   (F1=0.99). GPT-OSS-120B ignored them entirely. ToT may have the
   same model-capability floor.

2. **Scope leak is real.** Instructions meant for children get consumed
   by the parent. Every instruction must explicitly name its target level.

3. **Sequential dispatch beats mass backgrounding.** Concurrent children
   compete for resources. Start sequential, add concurrency later.

4. **Prescriptive > descriptive.** "Generate 3 approaches" is vague.
   "Generate 3 approaches: one using X, one using Y, one using Z" works.

5. **Prompt dilution is secondary to model capability.** Shorter prompts
   didn't help GPT-OSS. But they do help: the winning RLM prompt was
   230 lines, not 550.

## Prior art in this project

- `hew-tot-composition-analysis` (atuin): Full design doc for how ToT
  composes from hew's primitives. Covers evaluation methodology,
  brainflower lessons, implementation features.
- `hew-tot-ipc-analysis` (atuin): Why file-based IPC beats pipes/sockets
  for LLM-driven agent orchestration.
- `brainflower-tot-demo-analysis` (atuin): Post-mortem on ToT in a C99
  ECS agent. Concept validated, drowned in accidental complexity.

## What's next

1. Build eval task fixtures (Go project skeletons with tests)
2. Write the ToT system prompt (branch generation, evaluation rubrics)
3. Run baseline (single-path) against each task
4. Run ToT-2 and ToT-3 against each task
5. Compare scores
