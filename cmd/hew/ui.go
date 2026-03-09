package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	hew "github.com/cosgroveb/hew"
)

// Compile-time assertion that model implements tea.Model.
var _ tea.Model = model{}

type focusTarget int

const (
	focusInput focusTarget = iota
	focusViewport
)

// ggTimeout is the maximum delay between the two 'g' presses for the gg chord.
const ggTimeout = 500 * time.Millisecond

// ctrlCInterval is the maximum gap between two Ctrl-C presses for a forced quit.
const ctrlCInterval = 500 * time.Millisecond

type agentDoneMsg struct{ err error }

// shared holds references that must survive bubbletea's value-copy of model.
// Bubbletea copies the model on NewProgram, so pointers stored directly in
// model fields are stale after that copy. This struct is allocated once and
// shared via pointer.
type shared struct {
	agent    *hew.Agent
	program  *tea.Program
	eventLog *os.File
}

type model struct {
	chat      chatModel
	input     inputModel
	styles    *styles
	shared    *shared
	eventCh   <-chan hew.Event
	cancel    context.CancelFunc
	width     int
	height    int
	focus     focusTarget
	running   bool
	quitting  bool
	agentErr  error
	pendingG  bool
	gTimer    time.Time
	verbose   bool
	lastCtrlC time.Time
}

func newModel(eventCh <-chan hew.Event, s *styles, verbose bool, cancel context.CancelFunc) model {
	return model{
		chat:    newChatModel(0, 0, s, verbose),
		input:   newInputModel(0, &s.Input),
		styles:  s,
		shared:  &shared{},
		eventCh: eventCh,
		cancel:  cancel,
		focus:   focusInput,
		verbose: verbose,
	}
}

// inputHeight returns the height the input area should occupy.
func (m model) inputHeight() int {
	h := m.input.lineCount() + 1
	if h > 5 {
		h = 5
	}
	return h
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.input.textarea.Focus()}
	if m.eventCh != nil {
		cmds = append(cmds, eventBridge(m.eventCh))
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.QuitMsg:
		m.quitting = true
		return m, tea.Quit

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
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
		// Forward unrecognized messages (e.g. cursor blink) to the textarea
		// so its internal commands keep working.
		newTA, cmd := m.input.textarea.Update(msg)
		m.input.textarea = newTA
		return m, cmd
	}
}

func (m *model) recalcLayout() {
	ih := m.inputHeight()
	chatHeight := m.height - ih
	if chatHeight < 1 {
		chatHeight = 1
	}
	m.chat.resize(m.width, chatHeight)
	m.chat.updateViewport()
	m.input.setWidth(m.width)
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
		return m.handleCtrlC()

	case msg.Code == tea.KeyEscape:
		m.focus = focusViewport
		m.input.textarea.Blur()
		return m, nil

	case msg.Code == 'i' && m.focus == focusViewport:
		m.focus = focusInput
		return m, m.input.textarea.Focus()

	case msg.Code == 'q' && m.focus == focusViewport:
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}

	// Focus-specific key handling
	if m.focus == focusViewport {
		return m.handleViewportKey(msg)
	}

	return m.handleInputKey(msg)
}

func (m model) handleCtrlC() (tea.Model, tea.Cmd) {
	now := time.Now()
	doublePress := now.Sub(m.lastCtrlC) < ctrlCInterval
	m.lastCtrlC = now

	if doublePress || !m.running {
		// Double rapid Ctrl-C or idle: always quit
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}

	// Running: cancel the agent but don't quit
	if m.cancel != nil {
		m.cancel()
	}
	return m, nil
}

func (m model) handleInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Code == tea.KeyEnter && msg.Mod == 0:
		// Submit on Enter (no modifiers)
		if m.running {
			return m, nil // reject while running
		}
		text := m.input.submit()
		if text == "" {
			return m, nil
		}
		return m.startTask(text)

	case msg.Code == 'j' && msg.Mod == tea.ModCtrl:
		// Ctrl+J inserts a newline
		m.input.textarea.InsertRune('\n')
		m.recalcLayout()
		return m, nil

	case msg.Code == tea.KeyUp && m.input.textarea.Line() == 0:
		// Up at first line: history
		m.input.historyUp()
		return m, nil

	case msg.Code == tea.KeyDown && m.input.textarea.Line() == m.input.textarea.LineCount()-1:
		// Down at last line: history
		m.input.historyDown()
		return m, nil

	case msg.Mod == tea.ModCtrl && msg.Code == 'u':
		// Ctrl+U scrolls viewport even in input mode
		m.chat.viewport.HalfPageUp()
		if m.chat.viewport.AtBottom() {
			m.chat.hasNew = false
		}
		return m, nil

	case msg.Mod == tea.ModCtrl && msg.Code == 'd':
		// Ctrl+D: scroll viewport when empty or running, otherwise pass to textarea
		if m.input.textarea.Value() == "" || m.running {
			m.chat.viewport.HalfPageDown()
			if m.chat.viewport.AtBottom() {
				m.chat.hasNew = false
			}
			return m, nil
		}
	}

	// Pass through to textarea
	newTA, cmd := m.input.textarea.Update(msg)
	m.input.textarea = newTA
	m.recalcLayout()
	return m, cmd
}

func (m model) startTask(task string) (tea.Model, tea.Cmd) {
	// Show user's task in chat
	m.chat.content.WriteString(m.styles.Chat.UserMessage.Render(task))
	m.chat.content.WriteString("\n\n")
	m.chat.updateViewport()

	m.running = true

	// Create new context for this task
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// Set up event channel for this run
	eventCh := make(chan hew.Event, eventChSize)
	m.eventCh = eventCh
	m.shared.agent.Notify = makeNotify(eventCh, m.shared.eventLog)

	sh := m.shared

	cmd := func() tea.Msg {
		runErr := sh.agent.Run(ctx, task)
		close(eventCh)
		sh.program.Send(agentDoneMsg{err: runErr})
		return nil
	}

	return m, tea.Batch(cmd, eventBridge(eventCh))
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

	chatView := m.chat.viewport.View()

	// New content indicator
	if m.chat.hasNew {
		indicator := m.styles.Chat.NewContent.Render(
			fmt.Sprintf(" %s new content ", iconNewContent),
		)
		chatView += "\n" + indicator
	}

	inputView := m.input.view()

	return tea.NewView(chatView + "\n" + inputView)
}
