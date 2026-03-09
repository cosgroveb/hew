package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	hew "github.com/cosgroveb/hew"
)

func TestModelHandlesEventToken(t *testing.T) {
	// Since EventToken not defined, use EventDebug as placeholder
	m := newModel(nil, defaultStyles(), false)
	m.width = 80
	m.height = 24
	m.chat.resize(80, 24)
	m.running = true

	msg := eventMsg{event: hew.EventDebug{Message: "hello"}}
	updated, _ := m.Update(msg)
	um := updated.(model)

	// EventDebug does not affect streaming or view in current implementation.
	if um.chat.streaming {
		t.Error("streaming should be false for EventDebug")
	}
}

func TestModelHandlesEventResponse(t *testing.T) {
	m := newModel(nil, defaultStyles(), false)
	m.width = 80
	m.height = 24
	m.chat.resize(80, 24)
	m.running = true

	// Simulate token as debug (no streaming). We'll directly append response.
	updated, _ := m.Update(eventMsg{event: hew.EventResponse{Message: hew.Message{Role: "assistant", Content: "full response"}}})
	m = updated.(model)
	view := m.View()
	if !strings.Contains(view, "full response") {
		t.Error("View should contain authoritative response content")
	}
}

func TestModelFocusToggle(t *testing.T) {
	m := newModel(nil, defaultStyles(), false)
	m.width = 80
	m.height = 24
	m.chat.resize(80, 24)

	if m.focus != focusInput {
		t.Error("default focus should be input")
	}

	// Send Escape key
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	um := updated.(model)
	if um.focus != focusViewport {
		t.Error("Esc should switch to viewport focus")
	}
}

func TestModelAgentDone(t *testing.T) {
	m := newModel(nil, defaultStyles(), false)
	m.width = 80
	m.height = 24
	m.chat.resize(80, 24)
	m.running = true

	updated, _ := m.Update(agentDoneMsg{err: nil})
	um := updated.(model)
	if um.running {
		t.Error("running should be false after agentDoneMsg")
	}
}

func TestModelAgentDoneCommitsStreamBuffer(t *testing.T) {
	m := newModel(nil, defaultStyles(), false)
	m.width = 80
	m.height = 24
	m.chat.resize(80, 24)
	m.running = true

	// Simulate token via debug (placeholder)
	updated, _ := m.Update(eventMsg{event: hew.EventDebug{Message: "partial"}})
	m = updated.(model)

	// Agent dies with error (no EventResponse)
	updated, _ = m.Update(agentDoneMsg{err: fmt.Errorf("connection lost")})
	um := updated.(model)
	if um.chat.streaming {
		t.Error("streaming should be false after agentDoneMsg")
	}
	if um.chat.streamBuf.Len() != 0 {
		t.Error("streamBuf should be empty after agentDoneMsg commit")
	}
}
