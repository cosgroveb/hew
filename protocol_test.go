package hew

import (
	"testing"
)

func TestParseTurn(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
		wantErr  bool
	}{
		{
			name:     "valid act turn",
			input:    `{"type":"act","command":"ls -la"}`,
			wantType: "act",
		},
		{
			name:     "valid clarify turn",
			input:    `{"type":"clarify","question":"Which directory?"}`,
			wantType: "clarify",
		},
		{
			name:     "valid done turn",
			input:    `{"type":"done","summary":"Created the file and verified it works."}`,
			wantType: "done",
		},
		{
			name:     "act with extra fields ignored",
			input:    `{"type":"act","command":"echo hi","reasoning":"testing output"}`,
			wantType: "act",
		},
		{
			name:    "invalid json",
			input:   `not json at all`,
			wantErr: true,
		},
		{
			name:    "missing type field",
			input:   `{"command":"ls"}`,
			wantErr: true,
		},
		{
			name:    "unknown type",
			input:   `{"type":"explode","data":"boom"}`,
			wantErr: true,
		},
		{
			name:    "act missing command",
			input:   `{"type":"act"}`,
			wantErr: true,
		},
		{
			name:    "act empty command",
			input:   `{"type":"act","command":""}`,
			wantErr: true,
		},
		{
			name:    "clarify missing question",
			input:   `{"type":"clarify"}`,
			wantErr: true,
		},
		{
			name:    "clarify empty question",
			input:   `{"type":"clarify","question":""}`,
			wantErr: true,
		},
		{
			name:    "done missing summary",
			input:   `{"type":"done"}`,
			wantErr: true,
		},
		{
			name:    "done empty summary",
			input:   `{"type":"done","summary":""}`,
			wantErr: true,
		},
		{
			name:     "json wrapped in prose with json fence",
			input:    "Here is my response:\n\n```json\n{\"type\":\"act\",\"command\":\"ls\"}\n```\n\nLet me know.",
			wantType: "act",
		},
		{
			name:     "json wrapped in plain code fence",
			input:    "```\n{\"type\":\"done\",\"summary\":\"All finished.\"}\n```",
			wantType: "done",
		},
		{
			name:     "bare json with surrounding prose",
			input:    "I'll run this command:\n{\"type\":\"act\",\"command\":\"echo hello\"}\nThat should work.",
			wantType: "act",
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   \n\n  ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			turn, err := ParseTurn(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got turn: %+v", turn)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			switch tt.wantType {
			case "act":
				act, ok := turn.(*ActTurn)
				if !ok {
					t.Fatalf("expected *ActTurn, got %T", turn)
				}
				if act.Command == "" {
					t.Error("ActTurn.Command should not be empty")
				}
			case "clarify":
				cl, ok := turn.(*ClarifyTurn)
				if !ok {
					t.Fatalf("expected *ClarifyTurn, got %T", turn)
				}
				if cl.Question == "" {
					t.Error("ClarifyTurn.Question should not be empty")
				}
			case "done":
				d, ok := turn.(*DoneTurn)
				if !ok {
					t.Fatalf("expected *DoneTurn, got %T", turn)
				}
				if d.Summary == "" {
					t.Error("DoneTurn.Summary should not be empty")
				}
			}
		})
	}
}

func TestParseTurnTypes(t *testing.T) {
	t.Run("ActTurn implements Turn", func(t *testing.T) {
		var turn Turn = &ActTurn{Command: "ls"}
		turn.turn() // compile-time check
	})
	t.Run("ClarifyTurn implements Turn", func(t *testing.T) {
		var turn Turn = &ClarifyTurn{Question: "which dir?"}
		turn.turn()
	})
	t.Run("DoneTurn implements Turn", func(t *testing.T) {
		var turn Turn = &DoneTurn{Summary: "done"}
		turn.turn()
	})
}
