package anthropic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cosgroveb/hew"
	"github.com/cosgroveb/hew/anthropic"
)

func TestModel(t *testing.T) {
	t.Run("sends correct request", func(t *testing.T) {
		var gotBody map[string]interface{}
		var gotHeaders http.Header

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotHeaders = r.Header
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": "response"}},
				"usage":   map[string]int{"input_tokens": 10, "output_tokens": 20},
			})
		}))
		defer server.Close()

		m := anthropic.NewModel(server.URL, "test-key", "claude-test", "sys prompt")
		_, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hello"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotHeaders.Get("x-api-key") != "test-key" {
			t.Errorf("got api key %q, want %q", gotHeaders.Get("x-api-key"), "test-key")
		}
		if gotHeaders.Get("anthropic-version") == "" {
			t.Error("missing anthropic-version header")
		}
		if gotBody["system"] != "sys prompt" {
			t.Errorf("got system %v, want %q", gotBody["system"], "sys prompt")
		}
	})

	t.Run("parses response with usage", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": "hello back"}},
				"usage":   map[string]int{"input_tokens": 50, "output_tokens": 30},
			})
		}))
		defer server.Close()

		m := anthropic.NewModel(server.URL, "test-key", "claude-test", "sys")
		resp, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hi"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Message.Role != "assistant" {
			t.Errorf("got role %q, want assistant", resp.Message.Role)
		}
		if resp.Message.Content != "hello back" {
			t.Errorf("got content %q, want %q", resp.Message.Content, "hello back")
		}
		if resp.Usage.InputTokens != 50 || resp.Usage.OutputTokens != 30 {
			t.Errorf("got usage %+v, want 50/30", resp.Usage)
		}
	})

	t.Run("extracts error message from JSON error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "rate_limit_error",
					"message": "Rate limit exceeded",
				},
			})
		}))
		defer server.Close()

		m := anthropic.NewModel(server.URL, "test-key", "claude-test", "sys")
		_, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hi"}})
		if err == nil {
			t.Fatal("expected error on 429")
		}
		want := "api error (status 429): Rate limit exceeded"
		if err.Error() != want {
			t.Errorf("got %q, want error containing %q", err.Error(), want)
		}
	})

	t.Run("falls back to raw body for non-JSON errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("Bad Gateway"))
		}))
		defer server.Close()

		m := anthropic.NewModel(server.URL, "bad-key", "claude-test", "sys")
		_, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hi"}})
		if err == nil {
			t.Fatal("expected error on 502")
		}
		want := "api error (status 502): Bad Gateway"
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
	})
}

// writeLine writes a line to w, discarding errors (test SSE servers only).
func writeLine(w http.ResponseWriter, s string) { _, _ = fmt.Fprintln(w, s) }
func writeBlank(w http.ResponseWriter)          { _, _ = fmt.Fprintln(w) }

func TestQueryStream(t *testing.T) {
	t.Run("streams tokens and returns assembled response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			_ = json.Unmarshal(body, &req)
			if req["stream"] != true {
				t.Error("expected stream: true in request")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			writeLine(w, `event: message_start`)
			writeLine(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-test","usage":{"input_tokens":25,"output_tokens":1}}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: content_block_start`)
			writeLine(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: content_block_delta`)
			writeLine(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: content_block_delta`)
			writeLine(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: content_block_stop`)
			writeLine(w, `data: {"type":"content_block_stop","index":0}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: message_delta`)
			writeLine(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":15}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: message_stop`)
			writeLine(w, `data: {"type":"message_stop"}`)
			writeBlank(w)
			f.Flush()
		}))
		defer server.Close()

		m := anthropic.NewModel(server.URL, "test-key", "claude-test", "sys prompt")
		var tokens []string
		resp, err := m.QueryStream(context.Background(), []hew.Message{{Role: "user", Content: "hi"}}, func(text string) {
			tokens = append(tokens, text)
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 2 {
			t.Fatalf("expected 2 tokens, got %d: %v", len(tokens), tokens)
		}
		if tokens[0] != "Hello" || tokens[1] != " world" {
			t.Errorf("got tokens %v, want [Hello, world]", tokens)
		}
		if resp.Message.Content != "Hello world" {
			t.Errorf("got content %q, want %q", resp.Message.Content, "Hello world")
		}
		if resp.Usage.InputTokens != 25 || resp.Usage.OutputTokens != 15 {
			t.Errorf("got usage %+v, want 25/15", resp.Usage)
		}
	})

	t.Run("skips ping events", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			writeLine(w, `event: message_start`)
			writeLine(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-test","usage":{"input_tokens":10,"output_tokens":1}}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: ping`)
			writeLine(w, `data: {"type":"ping"}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: content_block_start`)
			writeLine(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: content_block_delta`)
			writeLine(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: content_block_stop`)
			writeLine(w, `data: {"type":"content_block_stop","index":0}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: message_delta`)
			writeLine(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: message_stop`)
			writeLine(w, `data: {"type":"message_stop"}`)
			writeBlank(w)
			f.Flush()
		}))
		defer server.Close()

		m := anthropic.NewModel(server.URL, "test-key", "claude-test", "sys")
		var tokens []string
		resp, err := m.QueryStream(context.Background(), []hew.Message{{Role: "user", Content: "hi"}}, func(text string) {
			tokens = append(tokens, text)
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 1 || tokens[0] != "Hi" {
			t.Errorf("got tokens %v, want [Hi]", tokens)
		}
		if resp.Message.Content != "Hi" {
			t.Errorf("got content %q, want %q", resp.Message.Content, "Hi")
		}
	})

	t.Run("returns error on non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "rate_limit_error",
					"message": "Rate limit exceeded",
				},
			})
		}))
		defer server.Close()

		m := anthropic.NewModel(server.URL, "test-key", "claude-test", "sys")
		_, err := m.QueryStream(context.Background(), []hew.Message{{Role: "user", Content: "hi"}}, func(string) {})
		if err == nil {
			t.Fatal("expected error on 429")
		}
	})

	t.Run("returns error on mid-stream error event", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			writeLine(w, `event: message_start`)
			writeLine(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-test","usage":{"input_tokens":10,"output_tokens":1}}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: error`)
			writeLine(w, `data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
			writeBlank(w)
			f.Flush()
		}))
		defer server.Close()

		m := anthropic.NewModel(server.URL, "test-key", "claude-test", "sys")
		_, err := m.QueryStream(context.Background(), []hew.Message{{Role: "user", Content: "hi"}}, func(string) {})
		if err == nil {
			t.Fatal("expected error on overloaded")
		}
		if !strings.Contains(err.Error(), "Overloaded") {
			t.Errorf("error should contain 'Overloaded', got: %v", err)
		}
	})

	t.Run("returns error on context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			writeLine(w, `event: message_start`)
			writeLine(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-test","usage":{"input_tokens":10,"output_tokens":1}}}`)
			writeBlank(w)
			f.Flush()
			<-r.Context().Done()
		}))
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		m := anthropic.NewModel(server.URL, "test-key", "claude-test", "sys")
		_, err := m.QueryStream(ctx, []hew.Message{{Role: "user", Content: "hi"}}, func(string) {})
		if err == nil {
			t.Fatal("expected error on cancelled context")
		}
	})

	t.Run("returns error on partial connection drop", func(t *testing.T) {
		var tokenCount int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			writeLine(w, `event: message_start`)
			writeLine(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-test","usage":{"input_tokens":10,"output_tokens":1}}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: content_block_start`)
			writeLine(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `event: content_block_delta`)
			writeLine(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`)
			writeBlank(w)
			f.Flush()
			// Server closes connection without message_stop
		}))
		defer server.Close()

		m := anthropic.NewModel(server.URL, "test-key", "claude-test", "sys")
		_, err := m.QueryStream(context.Background(), []hew.Message{{Role: "user", Content: "hi"}}, func(text string) {
			tokenCount++
		})
		if err == nil {
			t.Fatal("expected error on incomplete stream")
		}
		if tokenCount != 1 {
			t.Errorf("expected 1 token before drop, got %d", tokenCount)
		}
	})
}
