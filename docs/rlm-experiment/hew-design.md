# hew — Design Document

## Philosophy

hew is a minimal coding agent CLI inspired by Mini-SWE-Agent. The LLM is the agent; the scaffold gets out of its way. Radical simplicity — minimal lines of code, zero runtime dependencies, single binary.

Dual purpose: personal daily driver coding agent + research vehicle for agent experiments.

## Core Architecture

A single Go binary implementing a minimal agent loop: query an LLM, parse a bash action from the response, execute it via `os/exec`, append the output to the message history, repeat. The loop terminates when the LLM emits an `exit` action.

Two provider adapters — Anthropic and OpenAI-compatible — sit behind a `Model` interface, selected by the base URL. Configuration is env vars for secrets (`HEW_API_KEY`) and CLI flags for everything else (`--model`, `--base-url`). No config file.

System prompt is a hardcoded base with optional project-specific instructions appended from an `AGENTS.md` file in the working directory, if present.

Default mode is conversational (REPL). `-p "task"` runs a single prompt and exits.

All LLM responses and command outputs print to stdout as they happen. No TUI, no filtering.

## Components

1. **`Model` interface** — `Query(ctx context.Context, messages []Message) (Response, error)`. Two implementations: `AnthropicModel` and `OpenAIModel`. Raw HTTP to respective APIs. Adapter chosen at startup based on base URL (Anthropic's domain → Anthropic adapter, everything else → OpenAI-compatible).

2. **`Parser`** — extracts action from fenced ```bash code block via regex. Returns command string or exit sentinel. Malformatted response → one format reminder to LLM, second failure → exit with error.

3. **`Executor` interface** — runs command via `exec.CommandContext`, captures combined stdout/stderr, enforces timeout. Fresh subprocess per command. Agent tracks working directory and sets `cmd.Dir`. Accepting an interface makes the loop testable.

4. **`Agent`** — the loop. Holds message history (`[]Message`), a `Model`, the system prompt, and current working directory. Calls Model.Query → Parser → Executor → append output → repeat. On `exit`, prints final message and stops.

## Key Types

```go
type Message struct {
    Role    string
    Content string
}

type Usage struct {
    InputTokens  int
    OutputTokens int
}

type Response struct {
    Message Message
    Usage   Usage
}

type Model interface {
    Query(ctx context.Context, messages []Message) (Response, error)
}
```

## Error Handling

- Command timeout/failure → error output sent to LLM as next message, LLM adjusts
- Malformatted LLM response → one format reminder, second consecutive failure exits
- LLM API transient errors (429/500/503) → retry with backoff + jitter, max 3 attempts
- LLM API permanent errors (401/403) → exit immediately with clear message
- `--max-steps` flag for runaway prevention. LLM gets one more turn to summarize before exit
- Signal handling via `signal.NotifyContext` in main — Ctrl-C cancels in-flight requests, kills child processes

## Configuration

- `HEW_API_KEY` — env var, required
- `--model` — CLI flag, model identifier
- `--base-url` — CLI flag, provider endpoint URL
- `--max-steps` — CLI flag, optional step limit
- `-p "task"` — single prompt mode (non-interactive)
- No config file

## Working Directory Tracking

Agent tracks cwd as state. When the LLM runs `cd` commands, the agent updates its tracked directory and sets `cmd.Dir` on subsequent exec calls. System prompt also instructs LLM to prefer absolute paths.

## History Truncation

Simple max-messages cap for v1. When approaching the limit, oldest non-system messages are dropped. Prevents context window errors in long sessions.

## What's Explicitly Deferred

- Streaming (SSE) — no UX benefit when next step is "run a command"
- `--max-cost` — requires per-model pricing tables, `--max-steps` sufficient for v1
- Tool calling / function calling — regex parsing is provider-agnostic
- Persistent shell state — cwd tracking covers the main footgun
- Config file — env vars + flags are sufficient

## File Layout

```
hew/
├── go.mod
├── cmd/
│   └── hew/
│       └── main.go          # CLI parsing, signal handling, startup, REPL
└── internal/
    ├── types.go             # Message, Response, Usage, Model/Executor interfaces
    ├── agent.go             # Agent struct, loop, message history, cwd tracking
    ├── anthropic.go         # Anthropic adapter (raw HTTP)
    ├── openai.go            # OpenAI-compatible adapter (raw HTTP)
    ├── parser.go            # Action extraction regex
    ├── executor.go          # Command execution (implements Executor interface)
    └── prompt.go            # System prompt base + AGENTS.md loading
```

No Go source files in project root. `cmd/hew/main.go` is a thin entry point. All logic lives in `internal/`.

Estimated total: ~600-800 lines of Go.
