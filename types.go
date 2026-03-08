package hew

import "context"

// Message is one message in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage tracks token consumption for a single LLM call.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Response pairs an LLM reply with its token usage.
type Response struct {
	Message Message
	Usage   Usage
}

// Model sends messages to an LLM and returns a response.
type Model interface {
	Query(ctx context.Context, messages []Message) (Response, error)
}

// Streamer extends Model with streaming support.
// QueryStream calls onToken with each text fragment as it arrives from the API,
// then returns the fully-assembled Response. onToken is called synchronously
// from the SSE read loop and must not block for extended periods.
// Calls already made to onToken cannot be retracted on error.
type Streamer interface {
	Model
	QueryStream(ctx context.Context, messages []Message, onToken func(string)) (Response, error)
}

// Executor runs a shell command and returns its output.
type Executor interface {
	Execute(ctx context.Context, command string, dir string) (string, error)
}
