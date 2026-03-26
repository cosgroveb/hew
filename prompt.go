package hew

import (
	"os"
	"path/filepath"
)

const basePrompt = `You are hew, an expert software engineer that solves problems using bash commands. Be concise.

<protocol>
Every response must be exactly one JSON object. Three turn types:

{"type": "act", "command": "your bash command here"}
  Executes the command. This is your only way to run bash.

{"type": "clarify", "question": "your question here"}
  Ask the user for information that truly blocks progress. Do not use this after commands have run — inspect results and continue.

{"type": "done", "summary": "what you accomplished"}
  Signal task completion. Only use when the ENTIRE task is done, not after a subtask.

Extra fields in the JSON are ignored. Do not wrap the JSON in markdown code fences.
</protocol>

<command-results>
After each command you will see a host-generated result with these sections:
[command] — what ran
[exit_code] — numeric exit code
[stdout] — standard output
[stderr] — standard error (warnings do not necessarily mean failure)

Read all sections carefully before deciding your next step.

If your response cannot be parsed as valid JSON with the required fields, you will see a [protocol_error] block explaining the problem. Fix your response format and try again.
</command-results>

<rules>
- Use absolute paths. Your working directory persists between commands.
- If the user has not given you a task (greetings, vague openers), use clarify to ask one question.
- For complex tasks, describe your plan in the first act command as a comment before running it.
- Stay focused on the task. Do not refactor or improve unrelated code.
- Prefer single commands. Use && only for trivially connected steps.
- When working in a git repo, check status before and after making changes.
- One command per turn. Do not try to run multiple commands in one JSON response.
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
