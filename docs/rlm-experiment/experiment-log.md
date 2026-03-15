# RLM (Recursive Language Model) Experiment with Hew

## What RLM Is

RLM (arXiv:2512.24601, Zhang/Kraska/Khattab, Dec 2025) is an inference paradigm where the LLM programmatically examines, decomposes, and recursively calls itself over its input rather than stuffing everything into one context window.

## Our Adaptation: hew + RLM

hew (bash-only coding agent) implements RLM by spawning child `hu -p` processes. Each child gets a focused instruction, operates in its own 128K context window, and returns results via `--trajectory` JSON files. Parent orchestrates: estimate → partition → dispatch → validate → collect → merge.

Children don't get the RLM prompt, limiting recursion depth to 1.

## Prompt Evolution

| Ver | Change | Model Behavior | Failure Mode |
|-----|--------|---------------|--------------|
| v1 | "Most tasks fit in a single context" | Ignored RLM, wrote bash pipelines | Opt-out framing gave permission to skip |
| v2 | Removed opt-out, added size estimation | Followed RLM pattern conceptually | hu only executed first bash block per turn |
| v3 | hu executes all bash blocks | Chunked by subdir, processed all 71 | Wrote bash for-loop, not hu -p children. Context overflow |
| v4 | "MUST use hu -p, not for-loops" | Spawned 10 hu -p children (high-water mark: 7 TP/200) | Children's grep -o outputs file:match, not bare symbols |
| v5 | Prescriptive child instructions, bad/good example | Spawned children, followed structure | Used --exclude='dir/*' instead of --exclude-dir |
| v6 | Added --exclude-dir guidance | Backgrounded children with &, cnt==1 approach | Race condition: parent didn't wait. Raw grep output, fell back to bash pipeline |

## GPT-OSS-120B Behavioral Patterns

- **Prefers bash idioms over subprocess orchestration.** Every version where the model had a choice, it wrote for-loops/pipes/backgrounding rather than sequential hu -p calls.
- **Doesn't self-correct CLI flag errors.** --exclude vs --exclude-dir, grep -o output format — the model doesn't test its assumptions about tool behavior.
- **Loses focus on output format.** Algorithm may be correct but output is raw grep lines instead of the requested bare symbol names.
- **Backgrounds by default.** v6 used & unprompted. Must explicitly require sequential execution.
- **Follows prescriptive instructions literally but misses implied constraints.** Structural compliance without semantic understanding of what the instructions protect against.
- **MXFP4 quantized (128K context).** Quantization may contribute to reasoning degradation on multi-step CLI tool usage.

## Evaluation Task: Dead Export Detection

**Task**: Find EXPORT_SYMBOL/EXPORT_SYMBOL_GPL in drivers/gpu/drm/ never referenced elsewhere in the Linux kernel.

**Ground truth** (132 dead exports of 1644 total, 53s with xargs -P):
1. Extract symbols: `grep -rhoP 'EXPORT_SYMBOL(?:_GPL)?\s*\(\s*\K[a-zA-Z_][a-zA-Z0-9_]*' drivers/gpu/drm/ | sort -u`
2. For each, check external refs: `grep -rw --include='*.c' --include='*.h' "$sym" . | grep -v '/drivers/gpu/drm/' | head -1`
3. Zero matches = dead export

**Scoring**: Precision = TP/(TP+FP), Recall = TP/(TP+FN), F1 = harmonic mean. Answer extraction: last message from trajectory JSON, parse bare C identifiers, sort -u, compare with comm.

## Results

| Run | Prec | Rec | F1 | Steps | Time | Notes |
|-----|------|-----|-----|-------|------|-------|
| v1 GPT-OSS baseline | 0.08 | 0.93 | 0.15 | 3 | 132s | Dumped all exports |
| v1 GPT-OSS +rlm | 0.08 | 1.00 | 0.15 | 11 | 748s | Ignored RLM |
| v4 +rlm | 0.04 | 0.05 | 0.04 | 14 | 804s | Best: children spawned, wrong grep |
| v5 +rlm | N/A | 0.00 | 0.00 | 7 | 303s | --exclude bug |
| v6 +rlm | N/A | 0.00 | 0.00 | 7 | 532s | Fell back to bash |

## Key Lessons

1. **Opt-out framing kills adoption.** "Most tasks fit in a single context" = permission to skip.
2. **Computational triggers matter.** Context-window triggers alone aren't enough; add iteration count and wall-time thresholds.
3. **Children need prescriptive algorithms including exact CLI flags and output format specs.** Prescribing the algorithm without prescribing tool invocations is insufficient for this model.
4. **grep flags are a systematic failure point.** --exclude vs --exclude-dir, -o format, -f bulk matching — GPT-OSS-120B consistently gets these wrong.
5. **Multi-block execution enables both good patterns (dispatch+collect) and bad (long bash pipelines).**
6. **GPT-OSS-120B follows RLM structure when heavily constrained but makes systematic CLI tool errors.** The prompt must compensate for weak tool-use reasoning. This is not "model can't do it" — v4 spawned children and got 7 TP.
7. **No few-shot example has been tried yet.** All six versions used zero-shot instruction-only prompting.
8. **Chain-of-thought for parent planning is untried.** The parent needs to reason about partitioning strategy before writing code.

## Suggested Next Directions

- **Few-shot example**: Show a complete worked example of RLM decomposition (parent prompt → child dispatch → trajectory collection → merge) for any task, not just dead exports.
- **Template constraint**: Provide a fill-in-the-blank template for child dispatch instead of describing what to do:
  ```
  hu -p "INSTRUCTION" --trajectory /tmp/rlm-child-N.json --max-steps 10
  # Wait for completion before next child
  ```
- **Output format in child instructions**: Every child instruction must end with "Your output MUST be: one item per line, nothing else."
- **Sequential execution constraint**: Explicitly ban backgrounding children with &.
- **Validation step**: After children return, parent sanity-checks a sample before merging.

## Infrastructure

- GPT-OSS-120B on DGX Spark via vLLM (MXFP4, 128K context)
- Linux kernel shallow clone at /tmp/linux
- Eval artifacts: ~/code/private-evals/hew/
- Current RLM prompt: ~/AGENTS.md (moved from hew prompt.go)
- hew repo: ~/code/hew
