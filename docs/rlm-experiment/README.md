# RLM Experiment: Next Steps

## Goal

Get hew's RLM (Recursive Language Model) task decomposition pattern working
with a stronger model (Claude via Anthropic API) instead of GPT-OSS-120B.

## Background

We've iterated through 7+ prompt versions trying to get GPT-OSS-120B (MXFP4
quantized, running on DGX Spark) to follow hew's RLM decomposition pattern.
The pattern works architecturally — the model can spawn child `hu -p` processes —
but GPT-OSS consistently writes bash for-loops instead of dispatching children,
and when it does dispatch children, they write bad bash (wrong grep flags,
wrong output formats).

Full experiment log: [experiment-log.md](experiment-log.md)
Hew design doc: [hew-design.md](hew-design.md)

## What works

- `hu` executes all bash blocks in a single LLM response (v0.5.3+)
- `--trajectory` captures full conversation for scoring
- `--system-prompt-append` allows injecting custom prompt text
- `--no-system-prompt` (`-S`) allows replacing the entire system prompt
- `--dump-system-prompt` shows the composed prompt for debugging
- AGENTS.md layering loads RLM instructions automatically
- The eval task and ground truth generation are solid

## What to try with Claude

1. **Baseline**: Run the eval task with Claude Sonnet via Anthropic API.
   Claude has much stronger instruction-following than GPT-OSS-120B.
   The current RLM prompt in ~/AGENTS.md may just work.

2. **If baseline fails**: Use `--system-prompt-append` to inject stronger
   decomposition instructions without modifying AGENTS.md.

3. **If children write bad bash**: The prompt already has prescriptive
   algorithm examples. Claude should follow these more faithfully.

## Setup

```bash
# Configure hu for Anthropic
export HEW_API_KEY="$ANTHROPIC_API_KEY"
# No HEW_BASE_URL needed — defaults to https://api.anthropic.com
# No HEW_MODEL needed — defaults to claude-sonnet-4-20250514

# Verify
hu --version
hu --dump-system-prompt | tail -20  # Should show RLM section

# Shallow clone Linux kernel (if not already present)
git clone --depth 1 https://github.com/torvalds/linux.git /tmp/linux
```

## Running the eval

```bash
cd /tmp/linux

# Generate ground truth (one-time, ~53s)
grep -rhoP 'EXPORT_SYMBOL(?:_GPL)?\s*\(\s*\K[a-zA-Z_][a-zA-Z0-9_]*' \
  drivers/gpu/drm/ | sort -u > /tmp/all-exports.txt

# Check each symbol for external references
cat /tmp/all-exports.txt | xargs -P $(nproc) -I{} bash -c '
  if ! grep -rw --include="*.c" --include="*.h" "$1" . \
    | grep -v "/drivers/gpu/drm/" | head -1 | grep -q .; then
    echo "$1"
  fi' _ {} > /tmp/ground-truth.txt
sort -u -o /tmp/ground-truth.txt /tmp/ground-truth.txt

echo "Ground truth: $(wc -l < /tmp/ground-truth.txt) dead exports"

# Run hu
PROMPT="Find all EXPORT_SYMBOL and EXPORT_SYMBOL_GPL in drivers/gpu/drm/ \
that are never referenced anywhere else in this repository. List just the \
symbol names, one per line, nothing else."

hu -p "$PROMPT" \
  --trajectory /tmp/eval-result.json \
  --max-steps 100

# Extract answer
jq -r '.[-1].content' /tmp/eval-result.json \
  | grep -oP '^[a-zA-Z_][a-zA-Z0-9_]*$' \
  | sort -u > /tmp/answer.txt

# Score
GT=/tmp/ground-truth.txt
ANS=/tmp/answer.txt
TP=$(comm -12 "$GT" "$ANS" | wc -l)
FP=$(comm -13 "$GT" "$ANS" | wc -l)
FN=$(comm -23 "$GT" "$ANS" | wc -l)
echo "TP: $TP  FP: $FP  FN: $FN"
echo "Precision: $(awk "BEGIN{printf \"%.2f\", $TP/($TP+$FP)}")"
echo "Recall: $(awk "BEGIN{printf \"%.2f\", $TP/($TP+$FN)}")"
```

## The RLM prompt

The RLM task decomposition prompt is loaded from ~/AGENTS.md automatically.
To see what the model receives: `hu --dump-system-prompt`

Key behaviors the prompt requires:
- Estimate scale before writing any solution
- Partition work into chunks of 50-200 items
- Dispatch child `hu -p` processes (NOT bash for-loops)
- Give children prescriptive algorithms with exact commands and flags
- Validate one child's output before dispatching the batch
- Collect and merge results

## Known failure patterns (from GPT-OSS experiments)

These are the traps GPT-OSS fell into. Watch for Claude doing the same:

1. **Bash for-loop instead of child dispatch**: `while read sym; do grep...; done`
   fills context with intermediate output and gets killed.
2. **grep --exclude vs --exclude-dir**: `--exclude='dir/*'` doesn't exclude
   directories. Must use `--exclude-dir=NAME`.
3. **grep -o output format**: Outputs `file:match`, not bare symbol names.
   Breaks comm/sort comparisons.
4. **Backgrounding with &**: Children need sequential execution so parent
   can read each trajectory before dispatching next batch.
5. **Giving up early**: Model calls `<done/>` after one failed attempt
   instead of iterating.

## Expected outcome with Claude

Claude Sonnet should be able to:
- Follow the RLM decomposition instructions faithfully
- Write correct grep/comm pipelines for children
- Produce clean, one-symbol-per-line output
- Complete the task in ~10-20 parent steps with 10-20 child dispatches

Target: Precision > 0.80, Recall > 0.80 on the 132 dead exports.
