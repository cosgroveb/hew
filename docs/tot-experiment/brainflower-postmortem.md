# brainflower ToT Demo — Why No ToT Actions Fired

## Summary

The ToT run completed the `capture_eligible?` task without triggering a single ToT branch. The agent bypassed the confidence-scoring path entirely by routing all writes through `eval_ruby`.

## Root Cause

`get_tool_policy()` in `approval_system.c` assigns:

| Tool | Policy |
|------|--------|
| `run_command` | `APPROVE_ALWAYS` |
| `read_file` | `APPROVE_NEVER` |
| `eval_ruby` | `APPROVE_NEVER` |
| `write_file` / `edit_file` | `APPROVE_MODEL` ← ToT trigger |
| `runner` | `APPROVE_MODEL` ← ToT trigger |

Only `APPROVE_MODEL` tools go through `bf_auto_approve()` and can receive `COMP_TOT_PENDING`. Everything else bypasses the confidence scorer entirely.

## What Actually Happened

The agent used one large `eval_ruby` block to:
1. Research the AASM state machine, Status constants, existing validators, and insertion points — via `BF.tool("read_file", ...)` and `BF.tool("run_command", ...)` internally
2. Write both changes — via `BF.tool("edit_file", ...)` internally

`BF.tool()` calls dispatched from within `eval_ruby` never surface to the ECS approval system; they run inside the Ruby scripting sandbox, invisible to `approval_system`. So `edit_file` appearing as "[done]" in the TUI was a result of an inner call, not a top-level tool dispatch.

The only top-level `write_file` call was the final summary write to `~/capture_eligible_implementation.md` — after all code changes were already committed.

## Evidence

From `.brainflower/bf.log`: only `approval_system: spawning` and `approval_system: EVT_APPROVAL` entries — no `model-approve`, `tot-pending`, or `model-defer` entries. The ToT threshold and `bf_auto_approve()` were never reached.

From the decoded TUI log: all research and both `edit_file` writes were nested inside a single `eval_ruby` tool call.

## Fix Required

`eval_ruby` must be reclassified as `APPROVE_MODEL`. It is strictly more capable than `write_file` — it can write arbitrary files, run commands, and make multiple changes in one call. Leaving it as `APPROVE_NEVER` creates a trivial bypass of the entire ToT pipeline.

Options:
1. **Change `eval_ruby` policy to `APPROVE_MODEL`** — scores the call before execution, allowing ToT to branch on the full script
2. **Intercept `BF.tool()` at the scripting layer** — surface inner tool calls through the ECS approval system individually (more surgical but more complex)
3. **Both** — score `eval_ruby` as a unit AND intercept inner calls for nested ToT

Option 1 is the minimal fix. Option 2 would give finer-grained branching but requires changes to the scripting substrate.
