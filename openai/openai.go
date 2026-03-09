package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cosgroveb/hew"
)

// Model talks to any OpenAI-compatible chat completions API.
type Model struct {
	baseURL      string
	apiKey       string
	model        string
	systemPrompt string
	client       *http.Client
	streamClient *http.Client // no timeout — context controls cancellation
}

// NewModel sets up an OpenAI-compatible adapter.
func NewModel(baseURL, apiKey, model, systemPrompt string) *Model {
	return &Model{
		baseURL:      baseURL,
		apiKey:       apiKey,
		model:        model,
		systemPrompt: systemPrompt,
		client:       &http.Client{Timeout: 120 * time.Second},
		streamClient: &http.Client{},
	}
}

type request struct {
	Model    string        `json:"model"`
	Messages []hew.Message `json:"messages"`
}

type response struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

const maxResponseBytes = 1 << 20 // 1MB

// extractErrorMessage pulls the message from an API error response body.
// Handles both {"error":{"message":"..."}} and [{...}] array-wrapped forms.
// Falls back to the raw body if parsing fails.
func extractErrorMessage(body []byte) string {
	type errorBody struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	var single errorBody
	if err := json.Unmarshal(body, &single); err == nil && single.Error.Message != "" {
		return single.Error.Message
	}

	var arr []errorBody
	if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 && arr[0].Error.Message != "" {
		return arr[0].Error.Message
	}

	return string(body)
}

func (m *Model) Query(ctx context.Context, messages []hew.Message) (hew.Response, error) {
	allMessages := make([]hew.Message, 0, len(messages)+1)
	allMessages = append(allMessages, hew.Message{Role: "system", Content: m.systemPrompt})
	allMessages = append(allMessages, messages...)

	body, err := json.Marshal(request{Model: m.model, Messages: allMessages})
	if err != nil {
		return hew.Response{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return hew.Response{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return hew.Response{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return hew.Response{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return hew.Response{}, fmt.Errorf("api error (status %d): %s", resp.StatusCode, extractErrorMessage(respBody))
	}

	var apiResp response
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return hew.Response{}, fmt.Errorf("unmarshal response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return hew.Response{}, fmt.Errorf("no choices in response")
	}

	choice := apiResp.Choices[0]
	return hew.Response{
		Message: hew.Message{Role: choice.Message.Role, Content: choice.Message.Content},
		Usage:   hew.Usage{InputTokens: apiResp.Usage.PromptTokens, OutputTokens: apiResp.Usage.CompletionTokens},
	}, nil
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type streamRequest struct {
	Model         string         `json:"model"`
	Messages      []hew.Message  `json:"messages"`
	Stream        bool           `json:"stream"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// parseSSEData extracts the data payload from an SSE line.
// Returns the data string and true if the line is a data line, or ("", false) otherwise.
// Handles both "data: value" and "data:value" forms per the SSE spec.
func parseSSEData(line string) (string, bool) {
	if strings.HasPrefix(line, "data: ") {
		return strings.TrimPrefix(line, "data: "), true
	}
	if strings.HasPrefix(line, "data:") {
		return strings.TrimPrefix(line, "data:"), true
	}
	return "", false
}

func (m *Model) QueryStream(ctx context.Context, messages []hew.Message, onToken func(string)) (hew.Response, error) {
	allMessages := make([]hew.Message, 0, len(messages)+1)
	allMessages = append(allMessages, hew.Message{Role: "system", Content: m.systemPrompt})
	allMessages = append(allMessages, messages...)

	body, err := json.Marshal(streamRequest{
		Model:         m.model,
		Messages:      allMessages,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	})
	if err != nil {
		return hew.Response{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return hew.Response{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.streamClient.Do(req)
	if err != nil {
		return hew.Response{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		return hew.Response{}, fmt.Errorf("api error (status %d): %s", resp.StatusCode, extractErrorMessage(respBody))
	}

	var text strings.Builder
	var inputTokens, outputTokens int
	complete := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxResponseBytes)
	var nonSSELines strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		data, ok := parseSSEData(line)
		if !ok {
			if line != "" {
				nonSSELines.WriteString(line)
				nonSSELines.WriteByte('\n')
			}
			continue
		}
		if data == "[DONE]" {
			complete = true
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onToken(chunk.Choices[0].Delta.Content)
			text.WriteString(chunk.Choices[0].Delta.Content)
		}
		if chunk.Usage != nil {
			inputTokens = chunk.Usage.PromptTokens
			outputTokens = chunk.Usage.CompletionTokens
		}
	}
	if err := scanner.Err(); err != nil {
		return hew.Response{}, fmt.Errorf("read stream: %w", err)
	}
	if !complete {
		if msg := nonSSELines.String(); msg != "" {
			return hew.Response{}, fmt.Errorf("stream error: %s", extractErrorMessage([]byte(msg)))
		}
		return hew.Response{}, fmt.Errorf("stream interrupted: response ended before completion")
	}

	return hew.Response{
		Message: hew.Message{Role: "assistant", Content: text.String()},
		Usage:   hew.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
	}, nil
}
