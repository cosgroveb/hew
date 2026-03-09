package main

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	hew "github.com/cosgroveb/hew"
)

type focusTarget int

const (
	focusInput focusTarget = iota
	focusViewport
)

// ggTimeout is the maximum delay between the two 'g' presses for the gg chord.
const ggTimeout = 500 * time.Millisecond

type agentDoneMsg struct{ err error }

type model struct {
	chat     chatModel
	styles   *styles
	eventCh  <-chan hew.Event
	cancel   context.CancelFunc
	width    int
	height   int
	focus    focusTarget
	running  bool
	quitting bool
	agentErr error
	pendingG bool
	gTimer   time.Time
	verbose  bool
}

func newModel(eventCh <-chan hew.Event, s *styles, verbose bool, cancel context.CancelFunc) model {
	return model{
		chat:    newChatModel(0, 0, s, verbose),
		styles:  s,
		eventCh: eventCh,
		cancel:  cancel,
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
		m.agentErr = msg.err
		if m.chat.streaming {
			m.chat.commitPartialStream()
		}
		if msg.err != nil {
			m.chat.content.WriteString(
				m.styles.Chat.Warning.Render(fmt.Sprintf("\n%s Agent error: %s", iconError, msg.err)),
			)
			m.chat.content.WriteString("\n\n")
		}
		m.chat.updateViewport()
		return m, nil

	case eventMsg:
		m.chat.appendEvent(msg.event)
		m.chat.updateViewport()
		return m, eventBridge(m.eventCh)

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	default:
		return m, nil
	}
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// gg chord handling
	if m.pendingG {
		m.pendingG = false
		if msg.Code == 'g' && time.Since(m.gTimer) < ggTimeout {
			m.chat.viewport.GotoTop()
			m.chat.hasNew = false
			m.chat.updateViewport()
			return m, nil
		}
		// Timeout or different key — fall through to normal handling
	}

	switch {
	case msg.Code == 'c' && msg.Mod == tea.ModCtrl:
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit

	case msg.Code == tea.KeyEscape:
		m.focus = focusViewport
		return m, nil

	case msg.Code == 'i' && m.focus == focusViewport:
		m.focus = focusInput
		return m, nil

	case msg.Code == 'q' && m.focus == focusViewport:
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}

	// Viewport-focused keys
	if m.focus == focusViewport {
		return m.handleViewportKey(msg)
	}

	return m, nil
}

func (m model) handleViewportKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Code == 'j':
		m.chat.viewport.ScrollDown(1)
	case msg.Code == 'k':
		m.chat.viewport.ScrollUp(1)
	case msg.Code == 'G':
		m.chat.viewport.GotoBottom()
		m.chat.hasNew = false
	case msg.Code == 'g':
		m.pendingG = true
		m.gTimer = time.Now()
		return m, nil
	case msg.Mod == tea.ModCtrl && msg.Code == 'd':
		m.chat.viewport.HalfPageDown()
	case msg.Mod == tea.ModCtrl && msg.Code == 'u':
		m.chat.viewport.HalfPageUp()
	case msg.Code == tea.KeyPgUp:
		m.chat.viewport.HalfPageUp()
	case msg.Code == tea.KeyPgDown:
		m.chat.viewport.HalfPageDown()
	case msg.Code == tea.KeyHome:
		m.chat.viewport.GotoTop()
		m.chat.hasNew = false
	case msg.Code == tea.KeyEnd:
		m.chat.viewport.GotoBottom()
		m.chat.hasNew = false
	}

	if m.chat.viewport.AtBottom() {
		m.chat.hasNew = false
	}

	return m, nil
}

func (m model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	content := m.chat.viewport.View()

	// New content indicator
	if m.chat.hasNew {
		indicator := m.styles.Chat.NewContent.Render(
			fmt.Sprintf(" %s new content ", iconNewContent),
		)
		content += "\n" + indicator
	}

	return tea.NewView(content)
}
