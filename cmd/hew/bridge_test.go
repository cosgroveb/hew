package main

import (
	"os"
	"testing"

	hew "github.com/cosgroveb/hew"
)

func TestEventBridgeDeliversEvents(t *testing.T) {
	ch := make(chan hew.Event, eventChSize)
	ch <- hew.EventDebug{Message: "hello"}

	cmd := eventBridge(ch)
	msg := cmd()

	em, ok := msg.(eventMsg)
	if !ok {
		t.Fatalf("expected eventMsg, got %T", msg)
	}
	dbg, ok := em.event.(hew.EventDebug)
	if !ok {
		t.Fatalf("expected EventDebug, got %T", em.event)
	}
	if dbg.Message != "hello" {
		t.Errorf("expected %q, got %q", "hello", dbg.Message)
	}
}

func TestEventBridgeReturnsNilOnClose(t *testing.T) {
	ch := make(chan hew.Event, eventChSize)
	close(ch)

	cmd := eventBridge(ch)
	msg := cmd()

	if msg != nil {
		t.Fatalf("expected nil, got %T", msg)
	}
}

func TestNotifyNonBlockingWhenFull(t *testing.T) {
	ch := make(chan hew.Event, 2)
	// Fill channel
	ch <- hew.EventDebug{Message: "1"}
	ch <- hew.EventDebug{Message: "2"}

	notify := makeNotify(ch, nil)
	notify(hew.EventDebug{Message: "dropped"})

	e1 := <-ch
	e2 := <-ch
	if e1.(hew.EventDebug).Message != "1" || e2.(hew.EventDebug).Message != "2" {
		t.Error("channel contents were modified")
	}
}

func TestNotifyWritesEventLogWhenChannelFull(t *testing.T) {
	ch := make(chan hew.Event, 1)
	ch <- hew.EventDebug{Message: "fill"}

	f, err := os.CreateTemp("", "eventlog")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	notify := makeNotify(ch, f)
	// Use EventDebug to trigger log write
	notify(hew.EventDebug{Message: "log entry"})

	f.Seek(0, 0)
	data, _ := os.ReadFile(f.Name())
	if len(data) == 0 {
		t.Fatal("event log file is empty — write was dropped")
	}
}
