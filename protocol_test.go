package hew

import (
	"errors"
	"testing"
)

func TestParseTurn(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    TurnType
		wantErr error
	}{
		{
			name:  "act with command",
			input: `{"type":"act","command":"pwd","reasoning":"checking cwd"}`,
			want:  TurnTypeAct,
		},
		{
			name:  "clarify with question",
			input: `{"type":"clarify","question":"Which directory?"}`,
			want:  TurnTypeClarify,
		},
		{
			name:  "done with summary",
			input: `{"type":"done","summary":"Created the file."}`,
			want:  TurnTypeDone,
		},
		{
			name:    "act missing command rejected",
			input:   `{"type":"act","reasoning":"hmm"}`,
			wantErr: ErrMissingCommand,
		},
		{
			name:    "clarify with empty question rejected",
			input:   `{"type":"clarify","question":""}`,
			wantErr: ErrEmptyClarify,
		},
		{
			name:    "done with empty summary rejected",
			input:   `{"type":"done","summary":""}`,
			wantErr: ErrEmptySummary,
		},
		{
			name:    "unknown turn type rejected",
			input:   `{"type":"plan","todos":[]}`,
			wantErr: ErrUnknownTurnType,
		},
		{
			name:    "invalid json rejected",
			input:   `{"type":"act"`,
			wantErr: ErrInvalidJSON,
		},
		{
			name:  "extract from prose wrapping",
			input: "I'll inspect the directory.\n" + `{"type":"act","command":"ls -la"}`,
			want:  TurnTypeAct,
		},
		{
			name:  "extract from code fence",
			input: "```json\n{\"type\":\"act\",\"command\":\"pwd\"}\n```",
			want:  TurnTypeAct,
		},
		{
			name:    "no json at all",
			input:   "Here is my analysis of the problem.",
			wantErr: ErrInvalidJSON,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			turn, err := ParseTurn(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if turn.Type != tt.want {
				t.Fatalf("want type %q, got %q", tt.want, turn.Type)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"raw object", `{"type":"act"}`, `{"type":"act"}`},
		{"prose before", "Let me check.\n" + `{"type":"act","command":"ls"}`, `{"type":"act","command":"ls"}`},
		{"code fenced", "```json\n{\"type\":\"done\",\"summary\":\"ok\"}\n```", `{"type":"done","summary":"ok"}`},
		{"nested braces", `{"type":"act","command":"echo '{}'"}`, `{"type":"act","command":"echo '{}'"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractJSON(tt.input)
			if !ok {
				t.Fatal("extractJSON returned false")
			}
			if got != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got)
			}
		})
	}
}
