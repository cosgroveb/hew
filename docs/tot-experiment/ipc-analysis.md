# hew ToT IPC Analysis: Why File-Based I/O Beats Pipes, FIFOs, and Sockets

## Context

hew is a minimal coding agent CLI where an LLM generates bash commands. For Tree of Thought (ToT), a parent hew instance spawns N child hew processes via its bash executor. Three IPC needs arise: early termination, streaming observation, and result collection.

## The Key Constraint

The parent agent is an LLM issuing bash commands. Any IPC mechanism must be expressible as simple bash commands the LLM can generate reliably. This eliminates most "clever" Unix IPC.

## Early Termination: Process Groups (Not Signals/IPC)

**Winner: `Setpgid` + `Kill(-pgid)` in Go**

Each child runs in its own process group via `SysProcAttr{Setpgid: true}`. To kill a child and everything it spawned: `syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)`.

This is handled entirely in Go's executor — the LLM never sees it. The parent agent's "kill child 2" decision triggers a Go-level cancellation, not a bash command. Works identically on Linux and macOS.

No IPC needed for this capability at all.

## Streaming Observation: Per-Child JSONL Files

**Winner: Children write JSONL to `--event-log /path`, parent reads with `cat`/`tail`**

### Why Files Win

- **LLM-friendly**: Parent reads progress with `tail -n 5 /tmp/hew-tot/child-1.jsonl`. Simple, familiar, no setup.
- **Crash-safe**: If a child dies, the file persists. No reader/writer lifecycle dependencies.
- **Cross-platform**: No behavioral differences between Linux and macOS.
- **No coordination**: Children write independently, parent reads independently. No blocking, no deadlocks.
- **Atomic writes**: Go's `os.OpenFile` with `O_APPEND` gives atomic writes up to `PIPE_BUF` (4096 Linux, 512 macOS). JSONL event lines are well under either limit.

### Why Not Pipes (Anonymous)

- Anonymous pipes require parent-child file descriptor inheritance.
- The parent spawns children via `bash -c "hew ..."` through Go's `os/exec`. Pipe setup requires the parent to create the pipe before spawning, wire the fd, and manage the reader. This is Go code, not bash — the LLM can't express it.
- Pipe reads block. If the parent reads from a child's pipe and the child is slow, the parent's executor blocks and can't issue other commands.
- If the reader isn't consuming, the writer blocks when the pipe buffer fills (~64KB Linux, ~16KB macOS). A child generating lots of output could stall.

### Why Not Named Pipes (FIFOs)

- `mkfifo` creates the pipe, but a writer blocks (or gets SIGPIPE) if no reader is connected.
- The parent LLM would need to: (1) `mkfifo /tmp/child-1.fifo`, (2) start a background reader `cat /tmp/child-1.fifo > /tmp/child-1.log &`, (3) spawn the child writing to the FIFO, (4) manage the background reader. Too many moving parts for an LLM to generate reliably.
- If the parent forgets to open the reader first, the child blocks on open(). Deadlock.
- If the child exits, the reader gets EOF and exits. The FIFO becomes unusable — you'd need to recreate it. This makes polling impractical.
- macOS and Linux have subtly different FIFO semantics around O_NONBLOCK and multiple writers.

### Why Not Unix Domain Sockets

- Require a listener (server) to accept connections. The parent or a broker process must set up the listener before children connect.
- The LLM would need to generate socket setup commands or the Go code would need a socket server — both violate the "simple bash" constraint.
- Connection handling, message framing, and cleanup are all ceremony that files avoid.
- UDS are designed for bidirectional communication. We only need unidirectional (child → parent). Massive overkill.
- Path length limits (104-108 chars on macOS) can bite with deep temp directories.

### Why Not Shared Memory / mmap

- Requires coordination primitives (semaphores, mutexes) for safe access.
- No natural "append a line" semantic — you'd need to implement a ring buffer or similar.
- The LLM can't interact with shared memory via bash commands at all.
- Zero benefit over files for the throughput levels we need (a few JSON events per second).

## Result Collection: Trajectory Files + Git Diff

**Winner: `--trajectory /path` flag + `git diff` in worktree**

Children write their full message history as JSON to a specified path on exit. The parent collects with `cat /path/to/child-N.trajectory.json`.

For code diffs, the parent runs `git -C /tmp/worktree/child-N diff main` after the child exits. Worktrees persist, so there's no race.

No IPC mechanism needed — just file I/O and git commands the LLM already knows.

## Summary

| IPC Mechanism | Verdict | Why |
|---|---|---|
| Anonymous pipes | No | Require Go-level fd wiring, block on both ends, LLM can't express setup |
| Named pipes (FIFOs) | No | Reader-before-writer requirement causes deadlocks, EOF kills the pipe, LLM must manage background readers |
| Unix domain sockets | No | Server setup ceremony, bidirectional overkill, path length limits, LLM can't generate socket code |
| Shared memory / mmap | No | Coordination primitives needed, no bash interface, zero benefit at our throughput |
| Signals | Partial | Process groups handle termination cleanly, but signals can't carry data for observation |
| **Regular files (JSONL)** | **Yes** | LLM reads with cat/tail, crash-safe, cross-platform, no coordination, atomic appends |
| **Process groups** | **Yes** | Clean termination of child + descendants, handled in Go executor, invisible to LLM |

## The Principle

The IPC mechanism must be expressible as bash commands an LLM can generate. Files are the universal bash interface — `cat`, `tail`, `wc -l` are in every LLM's training data. Everything else requires setup ceremony that's fragile when generated by an LLM.

For hew specifically: the parent doesn't need real-time streaming. It needs point-in-time reads ("what's child 2 doing right now?"). Files serve this perfectly. The parent issues `tail -n 5 /tmp/child-2.jsonl` whenever it wants to check, which is just another bash command in its normal loop.
