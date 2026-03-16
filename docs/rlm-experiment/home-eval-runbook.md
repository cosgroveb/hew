# Home Eval Runbook — GLM-4.7-Flash on DGX Spark

Run the RLM eval against a weaker model (GLM-4.7-Flash, 30B-A3B MoE) on
the home DGX Spark to test whether the RLM prompt generalizes beyond
Claude Sonnet/Opus-class models.

## Context

The cpair eval (2026-03-15) achieved P=0.99, R=1.00, F1=0.99 with Claude
Sonnet via RLM v3 prompting. Baseline without RLM scored P=0.00, R=0.00.
See [claude-eval-run.md](claude-eval-run.md) for full analysis.

## Infrastructure

- **spark** (DGX Spark): 128GB unified memory, GB10 GPU, vLLM on port 8000
- **Active model**: GLM-4.7-Flash (30B-A3B MoE, BF16, ~100+ tok/s)
- **Endpoint**: `http://192.168.87.246:8000/v1` (OpenAI-compatible, no auth)
- No mitmproxy, no SSL_CERT_FILE needed on home network
- Children inherit env vars and hit the same vLLM endpoint

## Setup

```bash
# Build and install hu (children need it on PATH)
cd ~/code/hew
git checkout rlm-experiment
make install

# Clone Linux kernel
git clone --depth 1 --branch v6.12 https://github.com/torvalds/linux.git /tmp/linux
```

## Generate ground truth

```bash
cd /tmp/linux
grep -rhoP 'EXPORT_SYMBOL(?:_GPL)?\s*\(\s*\K[a-zA-Z_][a-zA-Z0-9_]*' \
  drivers/gpu/drm/ | sort -u > /tmp/all-exports.txt
cat /tmp/all-exports.txt | xargs -P $(nproc) -I{} bash -c '
  if ! grep -rw --include="*.c" --include="*.h" "$1" . \
    | grep -v "/drivers/gpu/drm/" | head -1 | grep -q .; then
    echo "$1"
  fi' _ {} > /tmp/ground-truth.txt
sort -u -o /tmp/ground-truth.txt /tmp/ground-truth.txt
echo "Ground truth: $(wc -l < /tmp/ground-truth.txt) dead exports"
```

Expected: 94 dead exports out of ~1,420 total.

## Run eval

```bash
export HEW_API_KEY="ignored"
export HEW_BASE_URL="http://192.168.87.246:8000/v1"
export HEW_MODEL="glm-47-flash"
cd /tmp/linux
hu -p "Find all EXPORT_SYMBOL and EXPORT_SYMBOL_GPL in drivers/gpu/drm/ \
that are never referenced anywhere else in this repository. List just the \
symbol names, one per line, nothing else." \
  --trajectory /tmp/eval-result.json \
  --event-log /tmp/eval-events.jsonl \
  --max-steps 100
```

To watch progress in another terminal:

```bash
tail -f /tmp/eval-events.jsonl | jq -r '.type + ": " + (.payload | tostring)[:120]'
```

### Using LiteLLM instead (authenticated, model aliases)

```bash
export HEW_API_KEY="sk-master-780d6d0de70633a5b9894737fb58859a550006c1a017fb9f"
export HEW_BASE_URL="http://192.168.87.246:4000/v1"
export HEW_MODEL="gpt-cos"
```

## Score

```bash
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

## Things to watch for

- **vLLM throughput**: 10 sequential children = 10 inference requests, one
  at a time. Each child may take several turns. Total wall time could be
  significant at ~100 tok/s.
- **Model capability threshold**: GLM-4.7-Flash is a 30B MoE. The RLM
  prompt requires the model to: (1) correctly assess scale, (2) partition
  work, (3) write prescriptive child instructions with exact commands,
  (4) parse trajectory JSON, (5) merge and filter results. Weaker models
  may fail at any of these steps even with the prompt.
- **Child instruction following**: Children need to execute the exact
  algorithm the parent specifies. If the model can't follow multi-step
  bash instructions reliably, children will produce garbage.
- **Format compliance**: hu requires `` ```bash `` fenced blocks. If the
  model uses other formats, the parser rejects them as format errors.
  Two consecutive format errors = agent exits.

## After the run

Document results in [claude-eval-run.md](claude-eval-run.md) or a new
`glm-eval-run.md` alongside it. Key data points:

- P, R, F1 scores
- Number of children dispatched, how many succeeded
- Failure modes (if any) — compare against known patterns in
  [experiment-log.md](experiment-log.md)
- Whether the model followed the RLM decomposition or attempted a
  single-agent approach despite the prompt
