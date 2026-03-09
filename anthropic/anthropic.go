package anthropic

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

// Model talks to the Anthropic Messages API.
type Model struct {
	baseURL      string
	apiKey       string
	model        string
	systemPrompt string
	maxTokens    int
	client       *http.Client
	streamClient *http.Client // no timeout — context controls cancellation
}

// NewModel sets up an Anthropic adapter.
func NewModel(baseURL, apiKey, model, systemPrompt string) *Model {
	return &Model{
		baseURL:      baseURL,
		apiKey:       apiKey,
		model:        model,
		systemPrompt: systemPrompt,
		maxTokens:    4096,
		client:       &http.Client{Timeout: 120 * time.Second},
		streamClient: &http.Client{},
	}
}

type request struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []hew.Message `json:"messages"`
}

type response struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
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
	body, err := json.Marshal(request{
		Model:     m.model,
		MaxTokens: m.maxTokens,
		System:    m.systemPrompt,
		Messages:  messages,
	})
	if err != nil {
		return hew.Response{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return hew.Response{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", m.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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

	var text string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return hew.Response{
		Message: hew.Message{Role: "assistant", Content: text},
		Usage:   hew.Usage{InputTokens: apiResp.Usage.InputTokens, OutputTokens: apiResp.Usage.OutputTokens},
	}, nil
}

type streamRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []hew.Message `json:"messages"`
	Stream    bool          `json:"stream"`
}

type messageStartBody struct {
	Type    string `json:"type"`
	Message struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type contentBlockDelta struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

type messageDelta struct {
	Type  string `json:"type"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type streamError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (m *Model) QueryStream(ctx context.Context, messages []hew.Message, onToken func(string)) (hew.Response, error) {
	body, err := json.Marshal(streamRequest{
		Model:     m.model,
		MaxTokens: m.maxTokens,
		System:    m.systemPrompt,
		Messages:  messages,
		Stream:    true,
	})
	if err != nil {
		return hew.Response{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return hew.Response{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", m.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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
	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := []byte(strings.TrimPrefix(line, "data: "))

		switch currentEvent {
		case "message_start":
			var msg messageStartBody
			if err := json.Unmarshal(data, &msg); err == nil {
				inputTokens = msg.Message.Usage.InputTokens
			}
		case "content_block_delta":
			var delta contentBlockDelta
			if err := json.Unmarshal(data, &delta); err == nil && delta.Delta.Type == "text_delta" {
				onToken(delta.Delta.Text)
				text.WriteString(delta.Delta.Text)
			}
		case "message_delta":
			var md messageDelta
			if err := json.Unmarshal(data, &md); err == nil {
				outputTokens = md.Usage.OutputTokens
			}
		case "message_stop":
			complete = true
		case "error":
			var se streamError
			if err := json.Unmarshal(data, &se); err == nil && se.Error.Message != "" {
				return hew.Response{}, fmt.Errorf("stream error: %s", se.Error.Message)
			}
			return hew.Response{}, fmt.Errorf("stream error: %s", string(data))
		default:
			// skip unknown events (ping, content_block_start, content_block_stop, future types)
		}
	}
	if err := scanner.Err(); err != nil {
		return hew.Response{}, fmt.Errorf("read stream: %w", err)
	}
	if !complete {
		return hew.Response{}, fmt.Errorf("stream interrupted: response ended before completion")
	}

	return hew.Response{
		Message: hew.Message{Role: "assistant", Content: text.String()},
		Usage:   hew.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
	}, nil
}
