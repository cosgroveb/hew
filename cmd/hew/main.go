package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	hew "github.com/cosgroveb/hew"
	"github.com/cosgroveb/hew/anthropic"
	"github.com/cosgroveb/hew/openai"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	var (
		prompt       string
		modelName    string
		baseURL      string
		maxSteps     int
		verbose      bool
		showVersion  bool
		loadMessages string
		eventLogPath string
		trajectory   string
	)

	flag.StringVar(&prompt, "p", "", "")
	flag.StringVar(&prompt, "prompt", "", "")
	flag.StringVar(&modelName, "model", "", "")
	flag.StringVar(&baseURL, "base-url", "", "")
	flag.IntVar(&maxSteps, "max-steps", 0, "")
	flag.BoolVar(&verbose, "verbose", false, "")
	flag.BoolVar(&verbose, "v", false, "")
	flag.BoolVar(&showVersion, "version", false, "")
	flag.StringVar(&eventLogPath, "event-log", "", "")
	flag.StringVar(&trajectory, "trajectory", "", "")
	flag.StringVar(&loadMessages, "load-messages", "", "")

	flag.Parse()

	if showVersion {
		fmt.Printf("hew %s\n", version)
		os.Exit(0)
	}

	// Resolve prompt alias
	taskPrompt := prompt

	// Resolve model
	if modelName == "" {
		if env := os.Getenv("HEW_MODEL"); env != "" {
			modelName = env
		} else {
			modelName = "claude-sonnet-4-20250514"
		}
	}

	// Resolve base URL
	baseURLSource := "default"
	if baseURL == "" {
		if env := os.Getenv("HEW_BASE_URL"); env != "" {
			baseURL = env
			baseURLSource = "HEW_BASE_URL env var"
		} else {
			baseURL = "https://api.anthropic.com"
		}
	} else {
		baseURLSource = "--base-url flag"
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		fmt.Fprintf(os.Stderr, "Error: invalid base URL %q (from %s): must start with http:// or https://\n", baseURL, baseURLSource)
		os.Exit(1)
	}

	// Resolve API key
	apiKey := os.Getenv("HEW_API_KEY")
	if apiKey == "" && strings.Contains(baseURL, "anthropic.com") {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: No API key found.")
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot get working directory: %v\n", err)
		os.Exit(1)
	}

	var eventLogFile *os.File
	if eventLogPath != "" {
		var err error
		eventLogFile, err = os.OpenFile(eventLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening event log %q: %v\n", eventLogPath, err)
			os.Exit(1)
		}
		defer eventLogFile.Close() //nolint:errcheck
	}

	systemPrompt := hew.LoadPrompt(cwd)

	var model hew.Model
	if strings.Contains(baseURL, "anthropic.com") {
		model = anthropic.NewModel(baseURL, apiKey, modelName, systemPrompt)
	} else {
		model = openai.NewModel(baseURL, apiKey, modelName, systemPrompt)
	}

	executor := &hew.CommandExecutor{}
	agent := hew.NewAgent(model, executor, cwd)
	if maxSteps > 0 {
		agent.MaxSteps = maxSteps
	}

	// Load messages if provided
	if loadMessages != "" {
		data, err := os.ReadFile(loadMessages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading messages %q: %v\n", loadMessages, err)
			os.Exit(1)
		}
		var msgs []hew.Message
		if err := json.Unmarshal(data, &msgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing messages %q: %v\n", loadMessages, err)
			os.Exit(1)
		}
		if err := agent.AddMessages(msgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding messages: %v\n", err)
			os.Exit(1)
		}
	}

	// TTY detection: if stdout not a terminal, fallback to plain mode (reuse core logic)
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		// Fallback to core CLI (non-interactive)
		// Reuse core main logic by delegating to hew-core binary could be done via exec, but here we simply run the agent directly with plain output.
		if taskPrompt == "" {
			fmt.Fprintln(os.Stderr, "Error: non-interactive mode requires -p")
			os.Exit(1)
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		runErr := agent.Run(ctx, taskPrompt)
		if runErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", runErr)
			os.Exit(1)
		}
		if trajectory != "" {
			msgs := agent.Messages()
			data, _ := json.MarshalIndent(msgs, "", "  ")
			os.WriteFile(trajectory, data, 0o644) //nolint:errcheck
		}
		return
	}

	// TUI mode
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	eventCh := make(chan hew.Event, eventChSize)
	agent.Notify = makeNotify(eventCh, eventLogFile)

	s := defaultStyles()
	m := newModel(eventCh, s, verbose)

	p := tea.NewProgram(m, tea.WithAltScreen())

	var runErr error
	go func() {
		runErr = agent.Run(ctx, taskPrompt)
		close(eventCh)
		p.Send(agentDoneMsg{err: runErr})
	}()

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	_ = finalModel

	if trajectory != "" {
		msgs := agent.Messages()
		data, _ := json.MarshalIndent(msgs, "", "  ")
		os.WriteFile(trajectory, data, 0o644) //nolint:errcheck
	}
	if runErr != nil {
		os.Exit(1)
	}
}
