package main

import (
	"strings"
	"testing"

	hew "github.com/cosgroveb/hew"
)

func TestChatAppendEvent_Response(t *testing.T) {
	s := defaultStyles()
	c := newChatModel(80, 24, s, false)

	c.appendEvent(hew.EventResponse{Message: hew.Message{Role: "assistant", Content: "hello world"}})

	if !strings.Contains(c.content.String(), "hello world") {
		t.Errorf("content should contain response text, got: %q", c.content.String())
	}
}

func TestChatStreamingBuffer(t *testing.T) {
	s := defaultStyles()
	c := newChatModel(80, 24, s, false)

	c.appendToken("Hello")
	if !c.streaming {
		t.Error("expected streaming=true after first token")
	}
	if c.streamBuf.String() != "Hello" {
		t.Errorf("streamBuf: got %q, want %q", c.streamBuf.String(), "Hello")
	}

	c.appendToken(" world")
	if c.streamBuf.String() != "Hello world" {
		t.Errorf("streamBuf: got %q, want %q", c.streamBuf.String(), "Hello world")
	}

	c.commitStream("Hello world!")
	if c.streaming {
		t.Error("expected streaming=false after commit")
	}
	if c.streamBuf.Len() != 0 {
		t.Error("streamBuf should be empty after commit")
	}
	if !strings.Contains(c.content.String(), "Hello world!") {
		t.Errorf("committed content should contain authoritative text, got: %q", c.content.String())
	}
}

func TestChatPendingCommandBuffer(t *testing.T) {
	s := defaultStyles()
	c := newChatModel(80, 24, s, false)

	c.appendEvent(hew.EventCommandStart{Command: "ls -la", Dir: "/tmp"})
	if c.pendingCmd == "" {
		t.Error("pendingCmd should be set after EventCommandStart")
	}
	if strings.Contains(c.content.String(), "ls -la") {
		t.Error("pending command should not be in committed content")
	}

	c.appendEvent(hew.EventCommandDone{Output: "total 0\n", Err: nil})
	if c.pendingCmd != "" {
		t.Error("pendingCmd should be cleared after EventCommandDone")
	}
	if !strings.Contains(c.content.String(), "ls -la") {
		t.Error("committed content should contain the command after EventCommandDone")
	}
}
