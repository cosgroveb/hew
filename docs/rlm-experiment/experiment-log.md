# RLM (Recursive Language Model) Experiment with Hew — Final Results

## Conclusion

RLM task decomposition via child process spawning works with Claude Opus/Sonnet-class models (F1=0.99) but GPT-OSS-120B (MXFP4 quantized) cannot follow the decomposition instructions regardless of prompt quality. The same prompt that achieves near-perfect scores with Claude produces identical failure behavior with GPT-OSS: bash for-loops, context overflow, zero child dispatch.

## Claude Results (cpair, Opus 4 via Cosmos proxy)

| Run | Prompt | P | R | F1 | Children | Notes |
|-----|--------|------|------|------|----------|-------|
| 1 | No RLM | 0.00 | 0.00 | 0.00 | 0 | while-loops killed |
| 2 | RLM v1 | 0.24 | 0.78 | 0.37 | 40 (16 killed) | Mass backgrounding, children used while-loops |
| 3 | RLM v2 (+bulk ops) | 0.00 | 0.00 | 0.00 | 0 | Parent skipped decomposition (scope leak) |
| 4 | RLM v3 (scoped bulk) | 0.99 | 1.00 | 0.99 | 10 (0 killed) | All children succeeded |
| 5 | Base only (no RLM) | 0.00 | 0.00 | 0.00 | 0 | Wrong cwd, hallucinated answer |

## GPT-OSS-120B Results (spark, MXFP4 via vLLM)

| Run | Prompt | Children | Notes |
|-----|--------|----------|-------|
| v1-v3 | Various | 0 | Ignored RLM, wrote bash loops |
| v4 | MUST use hu -p | 10 | Children spawned but wrong grep logic (7 TP/200) |
| v5-v6 | Prescriptive child instructions | 10 | Wrong grep flags, fell back to bash |
| v7-v7c | Same as Claude winning v3 | 0 | Ignored RLM entirely, context overflow |
| v8 | Exact Claude winning prompt | 0 | 6 steps, 21min, context overflow, 0 children |

## Key Findings

1. **RLM is a model capability test, not just a prompt engineering problem.** The v3 prompt encodes everything needed — estimation, partitioning, dispatch, validation, retry, merge. Claude follows it. GPT-OSS ignores it.

2. **The "scope leak" anti-pattern is real.** Instructions for children get consumed by the parent unless explicitly scoped. Fix: "This is about the child's internal approach — NOT a reason to skip decomposition."

3. **Sequential dispatch beats mass backgrounding.** 10 sequential children: 0 killed. 40 backgrounded: 16 killed.

4. **Prompt dilution is real but secondary.** Removing 380 lines of planning workflow from the prompt didn't help GPT-OSS. The model's instruction-following capacity is the bottleneck, not prompt length.

5. **MXFP4 quantization may degrade instruction-following.** GPT-OSS at full precision might perform differently, but we can't test this on DGX Spark hardware.

## Winning Prompt Location

~/AGENTS.md (managed by chezmoi) — the RLM v3 prompt with scoped bulk ops. Verified identical to docs/rlm-experiment/prompts/run4-system-prompt.txt.

## Infrastructure

- hew v0.6.0: --no-system-prompt, --system-prompt-append, --dump-system-prompt
- Eval task: dead export detection in Linux kernel drivers/gpu/drm/
- Ground truth: parallel grep, ~53s, deterministic
- Branch: rlm-experiment (cosgroveb/hew)
- Eval artifacts: ~/code/private-evals/hew/
