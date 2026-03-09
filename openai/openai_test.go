package openai_test

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
	"github.com/cosgroveb/hew/openai"
)

// writeLine writes a line to w, discarding errors (test SSE servers only).
func writeLine(w http.ResponseWriter, s string) { _, _ = fmt.Fprintln(w, s) }
func writeBlank(w http.ResponseWriter)          { _, _ = fmt.Fprintln(w) }

func TestModel(t *testing.T) {
	t.Run("prepends system message", func(t *testing.T) {
		var gotBody map[string]interface{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": "resp"}},
				},
				"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 20},
			})
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys prompt")
		_, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hello"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		msgs := gotBody["messages"].([]interface{})
		first := msgs[0].(map[string]interface{})
		if first["role"] != "system" || first["content"] != "sys prompt" {
			t.Errorf("first message should be system prompt, got %v", first)
		}
	})

	t.Run("parses response with usage", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": "hello back"}},
				},
				"usage": map[string]int{"prompt_tokens": 15, "completion_tokens": 25},
			})
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
		resp, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hi"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Message.Content != "hello back" {
			t.Errorf("got %q, want %q", resp.Message.Content, "hello back")
		}
		if resp.Usage.InputTokens != 15 || resp.Usage.OutputTokens != 25 {
			t.Errorf("got usage %+v, want 15/25", resp.Usage)
		}
	})

	t.Run("posts to base URL plus chat/completions", func(t *testing.T) {
		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": "ok"}},
				},
				"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1},
			})
		}))
		defer server.Close()

		m := openai.NewModel(server.URL+"/v1beta/openai", "test-key", "gemini-2.0-flash", "sys")
		_, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hi"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotPath != "/v1beta/openai/chat/completions" {
			t.Errorf("got path %q, want %q", gotPath, "/v1beta/openai/chat/completions")
		}
	})

	t.Run("extracts error message from JSON error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    429,
					"message": "Rate limit exceeded",
				},
			})
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
		_, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hi"}})
		if err == nil {
			t.Fatal("expected error on 429")
		}
		want := "api error (status 429): Rate limit exceeded"
		if err.Error() != want {
			t.Errorf("got %q, want error containing %q", err.Error(), want)
		}
	})

	t.Run("extracts error message from array-wrapped response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`[{"error":{"code":429,"message":"Quota exceeded"}}]`))
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
		_, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hi"}})
		if err == nil {
			t.Fatal("expected error on 429")
		}
		want := "api error (status 429): Quota exceeded"
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
	})

	t.Run("falls back to raw body for non-JSON errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("Bad Gateway"))
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
		_, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hi"}})
		if err == nil {
			t.Fatal("expected error on 502")
		}
		want := "api error (status 502): Bad Gateway"
		if err.Error() != want {
			t.Errorf("got %q, want error containing %q", err.Error(), want)
		}
	})

	t.Run("returns error on empty choices", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []interface{}{},
				"usage":   map[string]int{"prompt_tokens": 0, "completion_tokens": 0},
			})
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
		_, err := m.Query(context.Background(), []hew.Message{{Role: "user", Content: "hi"}})
		if err == nil {
			t.Error("expected error on empty choices")
		}
	})
}

func TestQueryStream(t *testing.T) {
	t.Run("streams tokens and returns assembled response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			_ = json.Unmarshal(body, &req)
			if req["stream"] != true {
				t.Error("expected stream: true in request")
			}
			opts, _ := req["stream_options"].(map[string]interface{})
			if opts == nil || opts["include_usage"] != true {
				t.Error("expected stream_options.include_usage: true")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			writeLine(w, `data: {"choices":[{"delta":{"role":"assistant","content":"Hello"}}],"usage":null}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `data: {"choices":[{"delta":{"content":" world"}}],"usage":null}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":10}}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `data: [DONE]`)
			writeBlank(w)
			f.Flush()
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys prompt")
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
		if resp.Usage.InputTokens != 20 || resp.Usage.OutputTokens != 10 {
			t.Errorf("got usage %+v, want 20/10", resp.Usage)
		}
	})

	t.Run("returns error on non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{"message": "Rate limit exceeded"},
			})
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
		_, err := m.QueryStream(context.Background(), []hew.Message{{Role: "user", Content: "hi"}}, func(string) {})
		if err == nil {
			t.Fatal("expected error on 429")
		}
	})

	t.Run("returns error on context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			<-r.Context().Done()
		}))
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
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
			writeLine(w, `data: {"choices":[{"delta":{"content":"partial"}}],"usage":null}`)
			writeBlank(w)
			f.Flush()
			// Close without [DONE]
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
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

	t.Run("handles missing usage gracefully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			writeLine(w, `data: {"choices":[{"delta":{"role":"assistant","content":"ok"}}]}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `data: [DONE]`)
			writeBlank(w)
			f.Flush()
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
		resp, err := m.QueryStream(context.Background(), []hew.Message{{Role: "user", Content: "hi"}}, func(string) {})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Usage.InputTokens != 0 || resp.Usage.OutputTokens != 0 {
			t.Errorf("expected zero usage when not provided, got %+v", resp.Usage)
		}
	})

	t.Run("handles data prefix without space", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			writeLine(w, `data:{"choices":[{"delta":{"content":"ok"}}],"usage":null}`)
			writeBlank(w)
			f.Flush()
			writeLine(w, `data:[DONE]`)
			writeBlank(w)
			f.Flush()
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
		var tokens []string
		resp, err := m.QueryStream(context.Background(), []hew.Message{{Role: "user", Content: "hi"}}, func(text string) {
			tokens = append(tokens, text)
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) != 1 || tokens[0] != "ok" {
			t.Errorf("got tokens %v, want [ok]", tokens)
		}
		if resp.Message.Content != "ok" {
			t.Errorf("got content %q, want %q", resp.Message.Content, "ok")
		}
	})

	t.Run("surfaces non-SSE JSON error mid-stream", func(t *testing.T) {
		var tokenCount int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			f := w.(http.Flusher)
			writeLine(w, `data: {"choices":[{"delta":{"content":"partial"}}],"usage":null}`)
			writeBlank(w)
			f.Flush()
			// Provider sends raw JSON error outside the SSE framing (e.g. Gemini)
			writeLine(w, `[{`)
			writeLine(w, `  "error": {`)
			writeLine(w, `    "code": 500,`)
			writeLine(w, `    "message": "Internal error encountered.",`)
			writeLine(w, `    "status": "INTERNAL"`)
			writeLine(w, `  }`)
			writeLine(w, `}]`)
		}))
		defer server.Close()

		m := openai.NewModel(server.URL, "test-key", "gpt-test", "sys")
		_, err := m.QueryStream(context.Background(), []hew.Message{{Role: "user", Content: "hi"}}, func(string) {
			tokenCount++
		})
		if err == nil {
			t.Fatal("expected error on mid-stream JSON error")
		}
		if !strings.Contains(err.Error(), "Internal error encountered") {
			t.Errorf("expected error to contain API message, got: %v", err)
		}
		if tokenCount != 1 {
			t.Errorf("expected 1 token before error, got %d", tokenCount)
		}
	})
}
