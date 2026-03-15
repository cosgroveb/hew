# RLM Experiment: Next Steps

## Goal

Get hew's RLM (Recursive Language Model) task decomposition pattern working
with a stronger model (Claude via Anthropic API) instead of GPT-OSS-120B.

## Naming

- **hew** is the project name and the TUI binary (Bubble Tea)
- **hu** is the plain CLI binary — this is what you run and what child processes use
- Both share the same flags and system prompt

## Background

We've iterated through 7+ prompt versions trying to get GPT-OSS-120B (MXFP4
quantized, running on DGX Spark) to follow hew's RLM decomposition pattern.
The pattern works architecturally — the model can spawn child `hu -p` processes —
but GPT-OSS consistently writes bash for-loops instead of dispatching children,
and when it does dispatch children, they write bad bash (wrong grep flags,
wrong output formats).

Full experiment log: [experiment-log.md](experiment-log.md)

## How hu works

1. User provides a task via `-p "task"`
2. hu sends the task + system prompt to the LLM
3. LLM responds with reasoning + one or more ```bash code blocks
4. hu executes ALL bash blocks sequentially, captures combined stdout/stderr
5. Output is appended to conversation history as the next user message
6. Loop repeats until LLM emits `<done/>` or `--max-steps` is hit
7. When `--max-steps` is reached, the LLM gets one more turn to summarize

System prompt is composed from: base prompt (hardcoded) + ~/AGENTS.md +
~/.config/hew/AGENTS.md + ./AGENTS.md (working directory). The RLM
decomposition instructions are in ~/AGENTS.md and load automatically
from any working directory.

Trajectory files (`--trajectory`) are JSON arrays of message objects:
`[{"role":"user","content":"..."},{"role":"assistant","content":"..."},...]`

## What works

- `hu` executes all bash blocks in a single LLM response (v0.5.3+)
- `--trajectory` captures full conversation for scoring
- `--system-prompt-append` allows injecting custom prompt text
- `--no-system-prompt` (`-S`) replaces entire system prompt (use with `--system-prompt-append` for full control)
- `--dump-system-prompt` shows the composed prompt for debugging
- AGENTS.md layering loads RLM instructions automatically

## The RLM prompt

The RLM task decomposition prompt is in ~/AGENTS.md (managed by chezmoi).
To see what the model receives: `hu --dump-system-prompt`

Key behaviors the prompt requires:
- Estimate scale before writing any solution
- Partition work into chunks of 50-200 items
- Dispatch child `hu -p` processes (NOT bash for-loops)
- Give children prescriptive algorithms with exact commands and flags
- Validate one child's output before dispatching the batch
- Collect and merge results

## Setup

```bash
# hu defaults to Anthropic — just needs an API key
# On work Mac, source secrets first:
source ~/.secrets.zsh
export HEW_API_KEY="$ANTHROPIC_API_KEY"

# Verify
hu --version          # Should be 0.6.0+
hu --dump-system-prompt | grep "Task decomposition"  # RLM section present

# Shallow clone Linux kernel (pin version for reproducible ground truth)
git clone --depth 1 --branch v6.12 https://github.com/torvalds/linux.git /tmp/linux
```

## Running the eval

```bash
cd /tmp/linux

# 1. Generate ground truth (~53s with parallel grep)
grep -rhoP 'EXPORT_SYMBOL(?:_GPL)?\s*\(\s*\K[a-zA-Z_][a-zA-Z0-9_]*' \
  drivers/gpu/drm/ | sort -u > /tmp/all-exports.txt

cat /tmp/all-exports.txt | xargs -P $(nproc) -I{} bash -c '
  if ! grep -rw --include="*.c" --include="*.h" "$1" . \
    | grep -v "/drivers/gpu/drm/" | head -1 | grep -q .; then
    echo "$1"
  fi' _ {} > /tmp/ground-truth.txt
sort -u -o /tmp/ground-truth.txt /tmp/ground-truth.txt
echo "Ground truth: $(wc -l < /tmp/ground-truth.txt) dead exports"

# 2. Run hu
PROMPT="Find all EXPORT_SYMBOL and EXPORT_SYMBOL_GPL in drivers/gpu/drm/ \
that are never referenced anywhere else in this repository. List just the \
symbol names, one per line, nothing else."

hu -p "$PROMPT" \
  --trajectory /tmp/eval-result.json \
  --max-steps 100

# 3. Extract answer from trajectory
# The last message content should be the symbol list or a <done/> summary.
# Check the second-to-last message too if the last is just <done/>.
jq -r '.[-1].content' /tmp/eval-result.json \
  | grep -oP '^[a-zA-Z_][a-zA-Z0-9_]*$' \
  | sort -u > /tmp/answer.txt

# If empty, try the second-to-last message:
if [ ! -s /tmp/answer.txt ]; then
  jq -r '.[-2].content' /tmp/eval-result.json \
    | grep -oP '^[a-zA-Z_][a-zA-Z0-9_]*$' \
    | sort -u > /tmp/answer.txt
fi

# 4. Score
GT=/tmp/ground-truth.txt
ANS=/tmp/answer.txt
TP=$(comm -12 "$GT" "$ANS" | wc -l)
FP=$(comm -13 "$GT" "$ANS" | wc -l)
FN=$(comm -23 "$GT" "$ANS" | wc -l)
echo "TP: $TP  FP: $FP  FN: $FN"
if [ $((TP+FP)) -gt 0 ]; then
  echo "Precision: $(awk "BEGIN{printf \"%.2f\", $TP/($TP+$FP)}")"
fi
if [ $((TP+FN)) -gt 0 ]; then
  echo "Recall: $(awk "BEGIN{printf \"%.2f\", $TP/($TP+$FN)}")"
fi
```

## Decision tree

- **P > 0.80, R > 0.80**: Success. Record results and trajectory.
- **Model used RLM but children wrote bad bash**: Check which failure
  pattern (see below). Adjust child instructions via AGENTS.md.
- **Model ignored RLM, wrote bash for-loops**: Try injecting stronger
  instructions: `hu -p "$PROMPT" --system-prompt-append "You MUST use
  hu -p for all subtasks. Do not write bash for-loops over large data."`
- **Model gave up early**: Increase `--max-steps` and add to prompt:
  "Do not exit until you have processed all chunks and produced a merged result."

## Known failure patterns (from GPT-OSS experiments)

1. **Bash for-loop instead of child dispatch**: `while read sym; do grep...; done`
   fills context with intermediate output and gets killed.
2. **grep --exclude vs --exclude-dir**: `--exclude='dir/*'` doesn't exclude
   directories. Must use `--exclude-dir=NAME`.
3. **grep -o output format**: Outputs `file:match`, not bare symbol names.
   Breaks comm/sort comparisons.
4. **Backgrounding with &**: Each child `hu -p` must complete and its
   trajectory must be readable before dispatching the next. Do not use `&`.
5. **Giving up early**: Model calls `<done/>` after one failed attempt
   instead of iterating on the approach.

## Claude-specific notes

- **Claude follows instructions precisely.** The AGENTS.md "MUST use hu -p"
  constraint is likely to be honored. The main risk is output format —
  add "output only symbol names, no explanations" to child instructions.
- **Claude's bash is generally correct.** The grep flag issues that plagued
  GPT-OSS are unlikely. You can probably relax the prescriptive CLI guidance
  if Claude's baseline shows correct flag usage.
- **Claude may add caveats.** Watch for "Here are the results..." prose
  mixed with symbol names. The answer extraction regex filters this, but
  check the raw trajectory if scores look wrong.
- **200K context window.** Claude Sonnet has 200K input context vs GPT-OSS's
  128K. Decomposition thresholds may need adjustment, but the point is to
  test the RLM pattern, not optimize for this specific task.

## Expected outcome

Claude Sonnet should be able to:
- Follow the RLM decomposition instructions faithfully
- Write correct grep/comm pipelines for children
- Produce clean, one-symbol-per-line output
- Complete the task in ~10-20 parent steps with 10-20 child dispatches

Target: Precision > 0.80, Recall > 0.80 on the dead exports.
