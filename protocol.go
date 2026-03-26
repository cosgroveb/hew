package hew

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Turn is a sealed interface for structured model turns.
// Only types in this package can implement it (unexported marker method).
type Turn interface{ turn() }

// ActTurn carries a bash command to execute.
type ActTurn struct {
	Command string `json:"command"`
}

// ClarifyTurn asks the user a question.
type ClarifyTurn struct {
	Question string `json:"question"`
}

// DoneTurn signals task completion with a summary.
type DoneTurn struct {
	Summary string `json:"summary"`
}

func (ActTurn) turn()     {}
func (ClarifyTurn) turn() {}
func (DoneTurn) turn()    {}

// jsonEnvelope is the raw JSON structure before type dispatch.
type jsonEnvelope struct {
	Type     string `json:"type"`
	Command  string `json:"command,omitempty"`
	Question string `json:"question,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

// jsonBlock matches a ```json or ``` fenced block.
var jsonBlock = regexp.MustCompile("(?s)```(?:json)?\\s*\\n(.*?)\\n?```")

// extractJSON finds the JSON object in model output.
// Models may wrap JSON in prose, code fences, or return it bare.
func extractJSON(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty response")
	}

	// Try fenced code block first.
	if m := jsonBlock.FindStringSubmatch(raw); len(m) >= 2 {
		return strings.TrimSpace(m[1]), nil
	}

	// Try to find a bare JSON object in the text.
	start := strings.IndexByte(raw, '{')
	if start < 0 {
		return "", fmt.Errorf("no JSON object found in response")
	}
	// Find the matching closing brace (simple depth counter).
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		c := raw[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("unbalanced JSON braces in response")
}

// ParseTurn extracts and validates a structured turn from model output.
func ParseTurn(raw string) (Turn, error) {
	jsonStr, err := extractJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse turn: %w", err)
	}

	var env jsonEnvelope
	if err := json.Unmarshal([]byte(jsonStr), &env); err != nil {
		return nil, fmt.Errorf("parse turn: invalid JSON: %w", err)
	}

	switch env.Type {
	case "act":
		if env.Command == "" {
			return nil, fmt.Errorf("parse turn: act turn missing command")
		}
		return &ActTurn{Command: env.Command}, nil
	case "clarify":
		if env.Question == "" {
			return nil, fmt.Errorf("parse turn: clarify turn missing question")
		}
		return &ClarifyTurn{Question: env.Question}, nil
	case "done":
		if env.Summary == "" {
			return nil, fmt.Errorf("parse turn: done turn missing summary")
		}
		return &DoneTurn{Summary: env.Summary}, nil
	case "":
		return nil, fmt.Errorf("parse turn: missing type field")
	default:
		return nil, fmt.Errorf("parse turn: unknown type %q", env.Type)
	}
}
