package hew

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultMaxSteps caps agent iterations when MaxSteps is not set.
const DefaultMaxSteps = 100

// ErrClarificationNeeded means the model stopped to request more user input.
var ErrClarificationNeeded = errors.New("clarification needed")

// Agent runs the query-parse-execute loop.
type Agent struct {
	model    Model
	executor Executor
	messages []Message
	cwd      string
	started  bool
	Notify   func(Event)
	MaxSteps int
}

// NewAgent wires up an agent with the given model, executor, and working directory.
func NewAgent(model Model, executor Executor, cwd string) *Agent {
	return &Agent{
		model:    model,
		executor: executor,
		cwd:      cwd,
		MaxSteps: DefaultMaxSteps,
	}
}

func (a *Agent) notify(e Event) {
	if a.Notify != nil {
		a.Notify(e)
	}
}

// Messages returns a copy of the conversation history.
func (a *Agent) Messages() []Message {
	cp := make([]Message, len(a.messages))
	copy(cp, a.messages)
	return cp
}

// AddMessages prepends msgs to the conversation history.
// Must be called before Run or Step; returns an error if the agent has already started.
func (a *Agent) AddMessages(msgs []Message) error {
	if a.started {
		return fmt.Errorf("add messages: agent already started")
	}
	if len(msgs) == 0 {
		return nil
	}
	combined := make([]Message, len(msgs)+len(a.messages))
	copy(combined, msgs)
	copy(combined[len(msgs):], a.messages)
	a.messages = combined
	return nil
}

func formatCommandOutput(result CommandResult) string {
	var b strings.Builder
	b.WriteString("[command]\n")
	b.WriteString(escapeSectionContent(result.Command))
	b.WriteString("\n[/command]\n")
	b.WriteString("[exit_code]\n")
	fmt.Fprintf(&b, "%d", result.ExitCode)
	b.WriteString("\n[/exit_code]\n")
	b.WriteString("[stdout]\n")
	b.WriteString(escapeSectionContent(result.Stdout))
	b.WriteString("\n[/stdout]\n")
	b.WriteString("[stderr]\n")
	b.WriteString(escapeSectionContent(result.Stderr))
	b.WriteString("\n[/stderr]")
	return b.String()
}

func escapeSectionContent(content string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"[", "&#91;",
		"]", "&#93;",
	)
	return replacer.Replace(content)
}

// Step runs one query-parse-execute cycle. It does not enforce step limits
// or track protocol failures — that is Run's job.
func (a *Agent) Step(ctx context.Context) (StepResult, error) {
	a.started = true
	a.notify(EventDebug{Message: "querying model..."})
	resp, err := a.model.Query(ctx, a.messages)
	if err != nil {
		return StepResult{}, fmt.Errorf("query model: %w", err)
	}
	a.notify(EventDebug{Message: fmt.Sprintf("usage: %d input, %d output tokens", resp.Usage.InputTokens, resp.Usage.OutputTokens)})
	a.messages = append(a.messages, resp.Message)
	a.notify(EventResponse{Message: resp.Message, Usage: resp.Usage})

	turn, parseErr := ParseTurn(resp.Message.Content)
	if parseErr != nil {
		a.notify(EventProtocolFailure{
			Reason:  errorToReason(parseErr),
			Message: parseErr.Error(),
		})
		// Append correction message
		a.messages = append(a.messages, Message{
			Role:    "user",
			Content: protocolCorrectionMessage(parseErr),
		})
		return StepResult{Response: resp}, nil
	}

	switch turn.Type {
	case TurnTypeClarify:
		a.notify(EventDebug{Message: "awaiting user clarification"})
		return StepResult{Response: resp, Turn: turn}, nil

	case TurnTypeDone:
		a.notify(EventDebug{Message: "done signal received"})
		return StepResult{Response: resp, Turn: turn}, nil

	case TurnTypeAct:
		a.updateCwd(turn.Command)
		a.notify(EventCommandStart{Command: turn.Command, Dir: a.cwd})
		execResult, execErr := a.executor.Execute(ctx, turn.Command, a.cwd)
		payload := formatCommandOutput(execResult)
		a.notify(EventCommandDone{
			Command:  turn.Command,
			Stdout:   execResult.Stdout,
			Stderr:   execResult.Stderr,
			ExitCode: execResult.ExitCode,
			Err:      execErr,
		})
		content := payload
		if content == "" {
			content = "(command completed with no output)"
		}
		a.messages = append(a.messages, Message{Role: "user", Content: content})
		return StepResult{Response: resp, Turn: turn, Output: payload, ExecErr: execErr}, nil
	}

	return StepResult{Response: resp}, nil
}

// Run loops Step until exit or step limit. Message history persists across calls.
func (a *Agent) Run(ctx context.Context, task string) error {
	a.messages = append(a.messages, Message{Role: "user", Content: task})
	steps := 0
	protocolFailures := 0

	for {
		result, err := a.Step(ctx)
		if err != nil {
			return err
		}

		switch result.Turn.Type {
		case TurnTypeDone:
			return nil
		case TurnTypeClarify:
			return ErrClarificationNeeded
		case TurnTypeAct:
			protocolFailures = 0
			steps++
			a.notify(EventDebug{Message: fmt.Sprintf("step %d/%d", steps, a.MaxSteps)})
			if a.MaxSteps > 0 && steps >= a.MaxSteps {
				a.notify(EventDebug{Message: "step limit reached, requesting summary"})
				a.messages = append(a.messages, Message{
					Role:    "user",
					Content: `Step limit reached. Summarize your progress and signal completion: {"type":"done","summary":"..."}`,
				})
				resp, err := a.model.Query(ctx, a.messages)
				if err != nil {
					return fmt.Errorf("query model (final): %w", err)
				}
				a.messages = append(a.messages, resp.Message)
				a.notify(EventResponse{Message: resp.Message, Usage: resp.Usage})
				return nil
			}
		default:
			// Protocol failure — Step already sent correction
			protocolFailures++
			a.notify(EventDebug{Message: fmt.Sprintf("consecutive protocol failures: %d", protocolFailures)})
			if protocolFailures >= 2 {
				return fmt.Errorf("consecutive protocol failures, exiting")
			}
		}
	}
}

func (a *Agent) updateCwd(command string) {
	// Only track standalone cd commands — compound commands (cd /tmp && ls)
	// change directory in the subprocess but we can't reliably parse them.
	trimmed := strings.TrimSpace(command)
	if !strings.HasPrefix(trimmed, "cd ") {
		return
	}
	// Reject compound commands: &&, ||, ;, pipes, or multi-line
	if strings.ContainsAny(trimmed, "&|;\n") {
		return
	}
	parts := strings.Fields(trimmed)
	if len(parts) != 2 {
		return
	}
	target := parts[1]

	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	var newCwd string
	switch {
	case target == "~":
		if home == "" {
			return
		}
		newCwd = home
	case strings.HasPrefix(target, "~/"):
		if home == "" {
			return
		}
		newCwd = filepath.Join(home, target[2:])
	case filepath.IsAbs(target):
		newCwd = target
	default:
		newCwd = filepath.Join(a.cwd, target)
	}

	if info, err := os.Stat(newCwd); err == nil && info.IsDir() {
		a.cwd = newCwd
	}
}
