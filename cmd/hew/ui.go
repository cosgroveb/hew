package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	hew "github.com/cosgroveb/hew"
)

type focusTarget int

const (
	focusInput focusTarget = iota
	focusViewport
)

// gg chord timeout
const ggTimeout = 500 * time.Millisecond

type model struct {
	chat     chatModel
	styles   *styles
	eventCh  <-chan hew.Event
	width    int
	height   int
	focus    focusTarget
	running  bool
	quitting bool
	pendingG bool
	gTimer   time.Time
	verbose  bool
}

func newModel(eventCh <-chan hew.Event, s *styles, verbose bool) model {
	return model{
		chat:    newChatModel(0, 0, s, verbose),
		styles:  s,
		eventCh: eventCh,
		focus:   focusInput,
		verbose: verbose,
	}
}

func (m model) Init() tea.Cmd {
	if m.eventCh != nil {
		return eventBridge(m.eventCh)
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.QuitMsg:
		m.quitting = true
		return m, tea.Quit

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chat.resize(msg.Width, msg.Height)
		m.chat.updateViewport()
		return m, nil

	case agentDoneMsg:
		m.running = false
		if m.chat.streaming {
			m.chat.commitPartialStream()
		}
		if msg.err != nil {
			m.chat.content.WriteString(m.styles.Chat.Warning.Render(fmt.Sprintf("\n%s Agent error: %s", iconError, msg.err)))
			m.chat.content.WriteString("\n\n")
		}
		m.chat.updateViewport()
		return m, nil

	case eventMsg:
		m.chat.appendEvent(msg.event)
		m.chat.updateViewport()
		return m, eventBridge(m.eventCh)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// gg chord handling
	if m.pendingG {
		m.pendingG = false
		if msg.Type == tea.KeyRunes && msg.String() == "g" && time.Since(m.gTimer) < ggTimeout {
			m.chat.viewport.GotoTop()
			m.chat.hasNew = false
			m.chat.updateViewport()
			return m, nil
		}
	}

	if msg.Type == tea.KeyEsc {
		m.focus = focusViewport
		return m, nil
	}
	if msg.Type == tea.KeyRunes && len(msg.String()) > 0 {
		ch := msg.String()[0]
		if ch == 'i' && m.focus == focusViewport {
			m.focus = focusInput
			return m, nil
		}
		if ch == 'q' && m.focus == focusViewport {
			return m, tea.Quit
		}
	}

	if m.focus == focusViewport {
		return m.handleViewportKey(msg)
	}
	// Input-focused keys are not handled here (future input component)
	return m, nil
}

func (m model) handleViewportKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyRunes && len(msg.String()) > 0 {
		switch msg.String()[0] {
		case 'j':
			m.chat.viewport.LineDown(1)
		case 'k':
			m.chat.viewport.LineUp(1)
		case 'G':
			m.chat.viewport.GotoBottom()
			m.chat.hasNew = false
		case 'g':
			m.pendingG = true
			m.gTimer = time.Now()
			return m, nil
		}
	}
	// Ctrl+d / Ctrl+u not supported in this simplified version

	if m.chat.viewport.AtBottom() {
		m.chat.hasNew = false
	}
	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	view := m.chat.viewport.View()
	if m.chat.hasNew {
		indicator := m.styles.Chat.NewContent.Render(fmt.Sprintf(" %s new content ", iconNewContent))
		view += "\n" + indicator
	}
	return view
}
