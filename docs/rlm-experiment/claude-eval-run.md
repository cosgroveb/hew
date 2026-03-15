# Claude RLM Eval Run — 2026-03-15

## Summary

Evaluated hew's RLM (Recursive Language Model) task decomposition pattern
with Claude Opus 4 on the dead-export detection task against Linux kernel
v6.12. After three prompt iterations, achieved **P=0.99, R=1.00, F1=0.99**.

## Environment

- **Model**: claude-opus-4-6 via Cosmos proxy (OpenAI-compatible endpoint)
- **Agent**: hu v0.6.0-2-gcf299ab
- **Repo**: Linux kernel v6.12 shallow clone (`git clone --depth 1 --branch v6.12`)
- **Machine**: cpair (16 cores, 31GB RAM)
- **TLS**: mitmproxy intercept → combined CA bundle via `SSL_CERT_FILE`

## Task

Find all `EXPORT_SYMBOL` and `EXPORT_SYMBOL_GPL` in `drivers/gpu/drm/` that
are never referenced anywhere else in the repository. Output bare symbol
names, one per line.

**Ground truth**: 94 dead exports out of 1,420 total (generated via
`xargs -P $(nproc)` parallel grep, ~53s).

## Results

| Run | Prompt version | P | R | F1 | Children | Notes |
|-----|---------------|-----|------|------|----------|-------|
| 1 | No RLM prompt | N/A | 0.00 | 0.00 | 0 | while-read loops killed, empty output |
| 2 | RLM v1 (initial) | 0.24 | 0.78 | 0.37 | 40 (16 killed) | Children used while-loops, all backgrounded |
| 3 | RLM v2 (+bulk ops) | 0.00 | 0.00 | 0.00 | 0 | Parent skipped RLM, did single bulk grep |
| 4 | RLM v3 (scoped bulk ops) | 0.99 | 1.00 | 0.99 | 10 (0 killed) | All children succeeded |
| 5 | Base prompt only (no RLM) | 0.00 | 0.00 | 0.00 | 0 | 41 commands, wrong cwd, hallucinated answer |

## Prompt Evolution

### Run 1: No RLM prompt (baseline)

The `~/AGENTS.md` file had no task decomposition instructions — only RTK
usage and debugging process. The model received the base system prompt
and wrote sequential `while read sym; do grep -rw "$sym" .; done` loops
over 1,420 symbols × 61K files. Every attempt was killed by resource
limits. After 2 turns (29 bash blocks), it declared `<done/>` with an
empty result.

**Failure mode**: Bash for-loop instead of child dispatch (Known Pattern #1
from experiment-log.md).

### Run 2: RLM v1 — Initial decomposition prompt

Added task decomposition instructions via `chezmoi pull`. Key additions:
- Estimate scale before writing solutions
- Partition into chunks of 50-200 items
- Dispatch child `hu -p` processes
- "You MUST use hu -p for subtasks, not bash for-loops"

**What worked**: Parent correctly assessed scale (1,420 symbols × 61K files),
partitioned into 40 chunks of 60 symbols, validated one child first, then
dispatched all remaining children.

**Three failures**:

1. **Mass backgrounding**: Parent dispatched all 39 remaining children with `&`
   and `wait`. 39 concurrent full-repo greps = resource exhaustion. 16 children
   killed.

2. **Children used while-loops**: Parent told children to "do this in a bash
   while-read loop processing one symbol at a time." Each child ran the same
   pattern the prompt forbids at the parent level — `while read sym; do grep -rw
   "$sym" .; done` — just pushed down one level. Children got killed for the
   same resource reasons.

3. **No retry on failed children**: Parent didn't check which children were
   killed, merged garbage output (error messages, prose) into the final answer.

### Run 3: RLM v2 — Added bulk operations guidance

Added rules telling children to use bulk operations (`grep -Fwf`, `xargs`,
`comm`) instead of per-item loops.

**Unintended regression**: The parent read "use bulk operations" as permission
to skip decomposition entirely. It ran a single-context `grep -oFwf` across
the whole repo in 2 turns with 0 child dispatches. The algorithm was wrong —
it excluded only the defining `.c` file but missed header declarations under
`include/drm/`, producing 38 false positives with 0 true positives.

**Key lesson**: Guidance intended for children was consumed by the parent as
an alternative strategy. The prompt lacked explicit scoping.

### Run 4: RLM v3 — Scoped bulk ops to children

Two targeted edits:

1. In child instruction rules: "This is about the child's *internal* approach —
   it is NOT a reason to skip decomposition. You must still partition and
   dispatch children."

2. In critical rules: "Decomposition is mandatory — bulk ops are what children
   do, not a shortcut to skip children. Do not run a single `grep -Fwf`
   yourself and call it done — that produces wrong results."

Also added:
- Sequential dispatch as default (max 3-4 concurrent)
- Mandatory retry of failed children
- Output filtering in merge step

**Result**: P=0.99, R=1.00, F1=0.99. Parent dispatched 10 children
sequentially, all succeeded. The single false positive was the word "done"
from the `<done/>` tag leaking through answer extraction.

## What the winning run did

The parent (run 4):
1. Extracted 1,420 exported symbols, counted 54,706 source files outside `drivers/gpu/drm/`
2. Partitioned symbols into 10 chunks
3. Dispatched children sequentially with prescriptive instructions
4. Each child used `xargs grep -Fwoh` to bulk-search its chunk against files outside `drivers/gpu/drm/`
5. Parent merged results, used `comm -23` to find symbols with zero external references
6. Spot-checked individual symbols with `grep -rw` to validate
7. Produced 94 symbols — all correct

## Analysis

### Why RLM v3 worked

1. **Explicit hierarchy**: Parent decomposes, children execute. The prompt
   makes this non-negotiable rather than suggestive.

2. **Bulk ops scoped correctly**: Children use `grep -Fwf` within their chunk,
   not per-item loops. But this is framed as a child implementation detail,
   not an alternative to decomposition.

3. **Sequential dispatch**: No resource contention. Each child completes
   before the next starts. 10 children × sequential = reliable.

4. **Fewer, larger chunks**: 10 chunks of ~140 symbols vs 40 chunks of 60.
   Fewer children = less overhead, less chance of failure.

### Why earlier versions failed

| Version | Root cause | Prompt deficiency |
|---------|-----------|-------------------|
| No RLM | No decomposition instructions at all | N/A — prompt didn't exist |
| v1 | Children used per-item loops; mass backgrounding | "No loops" rule applied to parent but not children; no concurrency limit |
| v2 | Parent skipped decomposition entirely | Bulk ops guidance lacked scope — parent consumed it as alternative strategy |
| Baseline | Wrong cwd ignored; single-pass algorithm; hallucinated answer | No decomposition framework; no "assess scope first" step to catch errors early |

### The "scope leak" anti-pattern

The most interesting failure was v2 → v3. Instructions meant for children
leaked to the parent because the prompt said "use bulk operations" without
specifying WHO should use them. Claude (correctly) optimized: if bulk grep
solves the problem in one pass, why decompose? The fix was explicit role
assignment: "You (the parent) MUST partition. Children use bulk ops within
their chunk."

This suggests a general principle for recursive/hierarchical agent prompts:
**any instruction that applies to a specific level must explicitly name that
level**, or the model will apply it at whatever level is most efficient.

### Run 5: Base prompt only (no RLM, baseline)

Same base prompt as run 4 (agent identity, format rules, file-ops, debugging,
finishing) plus the debugging framework from `~/AGENTS.md`, but with the
entire RLM task decomposition section stripped out. A resource warning was
appended via `--system-prompt-append` to avoid the uninteresting failure mode
of while-read loops being killed by resource limits.

The model attempted a reasonable single-agent strategy: extract all exports,
then use `git grep -owF -f` to find which symbols appear outside
`drivers/gpu/drm/`, then `comm -23` to identify the difference. However, it
failed on execution:

1. **Wrong working directory**: Used `/home/user/linux` (a default/assumed
   path) instead of `/tmp/linux` throughout all 41 commands. Most `cd`
   commands failed silently or weren't used, so `git grep` ran from the
   wrong directory and returned empty results.

2. **No error recovery**: Despite seeing `cd: /home/user/linux: No such file
   or directory` in output repeatedly, the model continued using the same
   wrong path for all 41 commands. The base prompt says "Read error output
   carefully" but the model ignored these errors.

3. **Hallucinated final answer**: With 0 actual results computed (all grep
   outputs were empty), the model declared `<done/>` and listed 71 symbol
   names that it fabricated — none matching ground truth.

**Result**: P=0.00, R=0.00, F1=0.00 (0 TP, 21 FP extracted, 94 FN).

**Key insight**: Without RLM decomposition instructions, the model attempted
a valid bulk-grep strategy but couldn't self-correct when basic execution
failed. The RLM prompt's "assess scope" step (run a fast command to measure
the problem) would have caught the wrong-directory issue immediately. More
importantly, the model had no framework for breaking the problem into
manageable pieces — its single-pass approach, even if the path had been
correct, would have produced wrong results (same algorithm failure as run 3).

## Prompt text

The full system prompt the model received in the winning run (RLM v3) is
reproduced in [prompts/run4-system-prompt.txt](prompts/run4-system-prompt.txt).

The prompt is composed from two sources:
1. **Base prompt** (hardcoded in `prompt.go`): Agent identity, format rules,
   file-ops, debugging, finishing instructions.
2. **~/AGENTS.md** (managed by chezmoi): Debugging framework + RLM task
   decomposition section. Injected as `<user-instructions>` block.

Children receive the identical prompt (including the RLM section from
`~/AGENTS.md`). They ignore the decomposition instructions since they're
executing a focused subtask, not orchestrating. The prompts are verified
identical — see [prompts/child-system-prompt.txt](prompts/child-system-prompt.txt).

The baseline run (run 5) used the same base prompt and debugging framework
but with the RLM task decomposition section removed from `~/AGENTS.md`. A
resource warning was appended via `--system-prompt-append`. The resulting
prompt is in
[prompts/baseline-system-prompt.txt](prompts/baseline-system-prompt.txt).

## Reproduction

```bash
# Prerequisites
make install  # hu must be on PATH for children
git clone --depth 1 --branch v6.12 https://github.com/torvalds/linux.git /tmp/linux

# Ground truth
cd /tmp/linux
grep -rhoP 'EXPORT_SYMBOL(?:_GPL)?\s*\(\s*\K[a-zA-Z_][a-zA-Z0-9_]*' \
  drivers/gpu/drm/ | sort -u > /tmp/all-exports.txt
cat /tmp/all-exports.txt | xargs -P $(nproc) -I{} bash -c '
  if ! grep -rw --include="*.c" --include="*.h" "$1" . \
    | grep -v "/drivers/gpu/drm/" | head -1 | grep -q .; then
    echo "$1"
  fi' _ {} > /tmp/ground-truth.txt
sort -u -o /tmp/ground-truth.txt /tmp/ground-truth.txt

# Run eval
export SSL_CERT_FILE=/path/to/combined-ca.pem  # if behind mitmproxy
export HEW_API_KEY="$ANTHROPIC_API_KEY"
export HEW_BASE_URL="$ANTHROPIC_BASE_URL"  # or omit for api.anthropic.com
cd /tmp/linux
hu -p "Find all EXPORT_SYMBOL and EXPORT_SYMBOL_GPL in drivers/gpu/drm/ \
that are never referenced anywhere else in this repository. List just the \
symbol names, one per line, nothing else." \
  --trajectory /tmp/eval-result.json \
  --event-log /tmp/eval-events.jsonl \
  --max-steps 100

# Score
jq -r '.[-2].content' /tmp/eval-result.json \
  | grep -oP '^[a-zA-Z_][a-zA-Z0-9_]*$' | sort -u > /tmp/answer.txt
GT=/tmp/ground-truth.txt; ANS=/tmp/answer.txt
TP=$(comm -12 "$GT" "$ANS" | wc -l)
FP=$(comm -13 "$GT" "$ANS" | wc -l)
FN=$(comm -23 "$GT" "$ANS" | wc -l)
echo "TP: $TP  FP: $FP  FN: $FN"
echo "Precision: $(awk "BEGIN{printf \"%.2f\", $TP/($TP+$FP)}")"
echo "Recall: $(awk "BEGIN{printf \"%.2f\", $TP/($TP+$FN)}")"
```
