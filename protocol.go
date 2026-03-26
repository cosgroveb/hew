package hew

import (
	"encoding/json"
	"errors"
	"fmt"
)

// TurnType identifies the kind of structured turn the model emitted.
type TurnType string

const (
	TurnTypeClarify TurnType = "clarify"
	TurnTypeAct     TurnType = "act"
	TurnTypeDone    TurnType = "done"
)

// Turn is a parsed structured turn from model output.
type Turn struct {
	Type      TurnType `json:"type"`
	Question  string   `json:"question,omitempty"`  // required when type=clarify
	Command   string   `json:"command,omitempty"`   // required when type=act
	Reasoning string   `json:"reasoning,omitempty"` // optional free-text reasoning (any turn)
	Summary   string   `json:"summary,omitempty"`   // required when type=done
}

var (
	ErrInvalidJSON     = errors.New("invalid JSON")
	ErrUnknownTurnType = errors.New("unknown turn type")
	ErrMissingCommand  = errors.New("act turn requires command")
	ErrEmptyClarify    = errors.New("clarify turn requires non-empty question")
	ErrEmptySummary    = errors.New("done turn requires non-empty summary")
)

// extractJSON finds the first top-level JSON object in raw text.
// Handles prose wrapping, code fences, and nested braces.
func extractJSON(raw string) (string, bool) {
	start := -1
	for i, ch := range raw {
		if ch == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1], true
			}
		}
	}
	return "", false
}

// ParseTurn extracts and validates a JSON turn from model output.
func ParseTurn(raw string) (Turn, error) {
	jsonStr, ok := extractJSON(raw)
	if !ok {
		return Turn{}, ErrInvalidJSON
	}
	var turn Turn
	if err := json.Unmarshal([]byte(jsonStr), &turn); err != nil {
		return Turn{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	switch turn.Type {
	case TurnTypeAct:
		if turn.Command == "" {
			return Turn{}, ErrMissingCommand
		}
	case TurnTypeClarify:
		if turn.Question == "" {
			return Turn{}, ErrEmptyClarify
		}
	case TurnTypeDone:
		if turn.Summary == "" {
			return Turn{}, ErrEmptySummary
		}
	default:
		return Turn{}, fmt.Errorf("%w: %q", ErrUnknownTurnType, turn.Type)
	}
	return turn, nil
}
