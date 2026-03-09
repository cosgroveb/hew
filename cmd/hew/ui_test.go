package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	hew "github.com/cosgroveb/hew"
)

func setupModel() model {
	s := defaultStyles(true)
	m := newModel(nil, s, false, nil)
	m.width = 80
	m.height = 24
	m.chat.resize(80, 24)
	m.running = true
	return m
}

// updateModel is a test helper that calls Update and asserts the result is a model.
func updateModel(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	result, _ := m.Update(msg)
	um, ok := result.(model)
	if !ok {
		t.Fatalf("expected model, got %T", result)
	}
	return um
}

func TestModelHandlesEventResponse(t *testing.T) {
	m := setupModel()
	m = updateModel(t, m, eventMsg{event: hew.EventDebug{Message: "querying model..."}})

	msg := eventMsg{event: hew.EventResponse{
		Message: hew.Message{Role: "assistant", Content: "full response"},
		Usage:   hew.Usage{InputTokens: 100, OutputTokens: 50},
	}}
	um := updateModel(t, m, msg)

	if um.chat.streaming {
		t.Error("expected streaming=false after EventResponse")
	}
	view := um.View()
	if !strings.Contains(view.Content, "full response") {
		t.Error("View should contain authoritative response content")
	}
}

func TestModelFocusToggle(t *testing.T) {
	m := setupModel()

	if m.focus != focusInput {
		t.Error("default focus should be input")
	}

	um := updateModel(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if um.focus != focusViewport {
		t.Error("Esc should switch to viewport focus")
	}

	um = updateModel(t, um, tea.KeyPressMsg{Code: 'i'})
	if um.focus != focusInput {
		t.Error("i should switch to input focus")
	}
}

func TestModelAgentDone(t *testing.T) {
	m := setupModel()
	um := updateModel(t, m, agentDoneMsg{err: nil})

	if um.running {
		t.Error("running should be false after agentDoneMsg")
	}
	if um.agentErr != nil {
		t.Error("agentErr should be nil on success")
	}
}

func TestModelAgentDonePreservesError(t *testing.T) {
	m := setupModel()
	um := updateModel(t, m, agentDoneMsg{err: fmt.Errorf("connection lost")})

	if um.agentErr == nil {
		t.Error("agentErr should be preserved for exit code propagation")
	}
	if um.agentErr.Error() != "connection lost" {
		t.Errorf("expected %q, got %q", "connection lost", um.agentErr.Error())
	}
}

func TestModelAgentDoneCommitsStreamBuffer(t *testing.T) {
	m := setupModel()
	m = updateModel(t, m, eventMsg{event: hew.EventDebug{Message: "querying model..."}})

	um := updateModel(t, m, agentDoneMsg{err: fmt.Errorf("connection lost")})

	if um.chat.streaming {
		t.Error("streaming should be false after agentDoneMsg")
	}
	if um.chat.streamBuf.Len() != 0 {
		t.Error("streamBuf should be empty after agentDoneMsg commit")
	}
}

func TestModelWindowResize(t *testing.T) {
	m := setupModel()
	um := updateModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	if um.width != 120 || um.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", um.width, um.height)
	}
}

func TestModelViewportScrollKeys(t *testing.T) {
	m := setupModel()
	m = updateModel(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})

	m = updateModel(t, m, tea.KeyPressMsg{Code: 'j'})
	m = updateModel(t, m, tea.KeyPressMsg{Code: 'k'})
	m = updateModel(t, m, tea.KeyPressMsg{Code: 'G'})
	m = updateModel(t, m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	_ = updateModel(t, m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
}

func TestModelGGChord(t *testing.T) {
	m := setupModel()

	for i := range 50 {
		fmt.Fprintf(m.chat.content, "line %d\n", i)
	}
	m.chat.updateViewport()

	m = updateModel(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})

	// First g sets pending
	um := updateModel(t, m, tea.KeyPressMsg{Code: 'g'})
	if !um.pendingG {
		t.Error("first g should set pendingG=true")
	}

	// Second g within timeout triggers GotoTop
	um = updateModel(t, um, tea.KeyPressMsg{Code: 'g'})
	if um.pendingG {
		t.Error("second g should clear pendingG")
	}
}
