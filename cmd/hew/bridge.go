package main

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	hew "github.com/cosgroveb/hew"
)

// eventChSize is the buffer capacity for the agent→TUI event channel.
const eventChSize = 256

type eventMsg struct{ event hew.Event }
type agentDoneMsg struct{ err error }

// eventBridge returns a tea.Cmd that reads the next event from the channel.
func eventBridge(ch <-chan hew.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return nil
		}
		return eventMsg{event: e}
	}
}

// makeNotify creates a Notify function that writes to an optional event log
// and sends events to the TUI channel without blocking (drops if full).
func makeNotify(ch chan<- hew.Event, eventLog *os.File) func(hew.Event) {
	return func(e hew.Event) {
		if eventLog != nil {
			writeEventLog(eventLog, e)
		}
		select {
		case ch <- e:
		default:
		}
	}
}

// writeEventLog writes a JSONL representation of the event.
func writeEventLog(f *os.File, e hew.Event) {
	var je struct {
		Type    string      `json:"type"`
		Payload interface{} `json:"payload"`
	}
	switch ev := e.(type) {
	case hew.EventResponse:
		je.Type = "response"
		je.Payload = ev
	case hew.EventCommandStart:
		je.Type = "command_start"
		je.Payload = ev
	case hew.EventCommandDone:
		je.Type = "command_done"
		je.Payload = struct {
			Output string `json:"output"`
			Err    string `json:"err,omitempty"`
		}{Output: ev.Output, Err: errString(ev.Err)}
	case hew.EventFormatError:
		je.Type = "format_error"
		je.Payload = ev
		// EventToken not defined in this codebase; ignore.
	case hew.EventDebug:
		je.Type = "debug"
		je.Payload = ev
	default:
		return
	}
	data, err := json.Marshal(je)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s\n", data)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
