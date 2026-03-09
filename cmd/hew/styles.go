package main

import (
	"github.com/charmbracelet/lipgloss"
)

const (
	iconPending    = "●"
	iconSuccess    = "✓"
	iconError      = "✗"
	iconPrompt     = "❯"
	iconNewContent = "↓"
	iconWarning    = "⚠"
)

type chatStyles struct {
	UserMessage      lipgloss.Style
	AssistantMessage lipgloss.Style
	StreamingText    lipgloss.Style
	CommandPending   lipgloss.Style
	CommandSuccess   lipgloss.Style
	CommandError     lipgloss.Style
	CommandOutput    lipgloss.Style
	Debug            lipgloss.Style
	Warning          lipgloss.Style
	NewContent       lipgloss.Style
}

type statusStyles struct {
	Bar            lipgloss.Style
	ModelName      lipgloss.Style
	Tokens         lipgloss.Style
	StepCount      lipgloss.Style
	Elapsed        lipgloss.Style
	FocusIndicator lipgloss.Style
	Separator      lipgloss.Style
}

type inputStyles struct {
	Prompt  lipgloss.Style
	Running lipgloss.Style
}

type styles struct {
	Chat   chatStyles
	Status statusStyles
	Input  inputStyles
}

func defaultStyles() *styles {
	muted := lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"}
	dim := lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"}
	warn := lipgloss.AdaptiveColor{Light: "#CC6600", Dark: "#FFAA00"}
	errColor := lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF4444"}
	success := lipgloss.AdaptiveColor{Light: "#006600", Dark: "#44FF44"}
	pending := lipgloss.AdaptiveColor{Light: "#666600", Dark: "#FFFF44"}
	info := lipgloss.AdaptiveColor{Light: "#0066CC", Dark: "#44AAFF"}

	return &styles{
		Chat: chatStyles{
			UserMessage:      lipgloss.NewStyle().Bold(true),
			AssistantMessage: lipgloss.NewStyle(),
			StreamingText:    lipgloss.NewStyle(),
			CommandPending:   lipgloss.NewStyle().Foreground(pending),
			CommandSuccess:   lipgloss.NewStyle().Foreground(success),
			CommandError:     lipgloss.NewStyle().Foreground(errColor),
			CommandOutput:    lipgloss.NewStyle().Foreground(muted),
			Debug:            lipgloss.NewStyle().Foreground(dim),
			Warning:          lipgloss.NewStyle().Foreground(warn),
			NewContent:       lipgloss.NewStyle().Foreground(info).Bold(true),
		},
		Status: statusStyles{
			Bar:            lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#EEEEEE", Dark: "#333333"}),
			ModelName:      lipgloss.NewStyle().Bold(true),
			Tokens:         lipgloss.NewStyle().Foreground(muted),
			StepCount:      lipgloss.NewStyle().Foreground(muted),
			Elapsed:        lipgloss.NewStyle().Foreground(muted),
			FocusIndicator: lipgloss.NewStyle().Bold(true),
			Separator:      lipgloss.NewStyle().Foreground(dim),
		},
		Input: inputStyles{
			Prompt:  lipgloss.NewStyle().Foreground(info).Bold(true),
			Running: lipgloss.NewStyle().Foreground(dim).Italic(true),
		},
	}
}
