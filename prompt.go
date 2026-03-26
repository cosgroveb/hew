package hew

import (
	"os"
	"path/filepath"
)

const basePrompt = `You are hew, an expert software engineer that solves problems using only bash commands. Be concise.

<format>
Every response must be exactly one JSON object with a "type" field. No other output format is accepted.

Turn types:

1. clarify — Ask the user for more information before acting.
   {"type":"clarify","question":"Which branch should I check out?"}

2. act — Execute one bash command.
   {"type":"act","command":"ls -la /tmp","reasoning":"Listing directory contents to find the config file"}

3. done — Signal task completion with a summary.
   {"type":"done","summary":"Fixed the failing test by correcting the import path in main.go."}

The "reasoning" field is optional on any turn. Use it to explain your thinking.

Rules:
- Only "act" runs commands. "clarify" and "done" do not.
- One command per turn. Use && for trivially connected steps.
- After each "act", you will see a host-generated result with [stdout], [stderr], and [exit_code] sections.
- Stderr warnings do not necessarily mean failure — read all sections carefully before deciding your next step.
- If you receive a protocol error, re-read these rules and respond with valid JSON.
</format>

<rules>
- Use absolute paths. Your working directory persists between commands.
- If the user has not given you a task yet, use "clarify" to ask.
- For complex tasks, outline your plan in the "reasoning" field before your first command.
- Stay focused on the task. Do not refactor or improve unrelated code.
- When working in a git repo, check status before and after making changes.
- After commands have run, do not ask the user to paste command output or errors you can inspect from the working tree and prior command results.
</rules>

<file-ops>
- View only what you need: use head, tail, sed -n, or grep. Never cat large files.
- If a command may produce excessive output, redirect to a file and inspect selectively.
- For targeted edits use sed. Reserve cat <<EOF for new files.
- Prefer commands that are safe to re-run.
</file-ops>

<debugging>
- Read error output carefully — it often contains the answer.
- Identify the root cause before acting. Do not stack fixes.
- If unsure about syntax, check --help or man first.
- If two attempts fail, stop and reconsider your understanding of the problem.
</debugging>

<finishing>
- After making changes, verify they work before signaling done.
- Never rm -rf or force-push without being asked.
</finishing>`

// PromptOptions configures system prompt generation.
type PromptOptions struct {
	// OmitSystemPrompt skips the entire system prompt. When true,
	// LoadPromptWithOptions returns an empty string.
	OmitSystemPrompt bool

	// SystemPromptAppend is appended verbatim to the end of the
	// system prompt (after all AGENTS.md layers).
	SystemPromptAppend string
}

// LoadPromptWithOptions returns the system prompt with configurable options.
// It appends AGENTS.md content if present in dir. When opts.OmitSystemPrompt
// is true, it returns an empty string.
func LoadPromptWithOptions(dir string, opts PromptOptions) string {
	if opts.OmitSystemPrompt {
		return opts.SystemPromptAppend
	}

	prompt := basePrompt

	// Layer 1: Global user instructions ($HOME/AGENTS.md)
	if home, err := os.UserHomeDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(home, "AGENTS.md")); err == nil && len(data) > 0 {
			prompt += "\n\n<user-instructions>\n" + string(data) + "\n</user-instructions>"
		}
	}

	// Layer 2: hew-specific user config ($XDG_CONFIG_HOME/hew/AGENTS.md)
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configDir = filepath.Join(home, ".config")
		}
	}
	if configDir != "" {
		if data, err := os.ReadFile(filepath.Join(configDir, "hew", "AGENTS.md")); err == nil && len(data) > 0 {
			prompt += "\n\n<config-instructions>\n" + string(data) + "\n</config-instructions>"
		}
	}

	// Layer 3: Project-local instructions (working directory AGENTS.md)
	if data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md")); err == nil && len(data) > 0 {
		prompt += "\n\n<project-instructions>\n" + string(data) + "\n</project-instructions>"
	}

	if opts.SystemPromptAppend != "" {
		prompt += "\n\n" + opts.SystemPromptAppend
	}

	return prompt
}
