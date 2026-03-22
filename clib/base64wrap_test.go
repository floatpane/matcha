package clib

import (
	"strings"
	"testing"
)

func TestWrapBase64Empty(t *testing.T) {
	result := WrapBase64("")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestWrapBase64ShortString(t *testing.T) {
	input := "SGVsbG8gV29ybGQ="
	result := WrapBase64(input)
	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestWrapBase64Exactly76(t *testing.T) {
	input := strings.Repeat("A", 76)
	result := WrapBase64(input)
	if result != input {
		t.Errorf("expected no wrapping for exactly 76 chars, got %q", result)
	}
}

func TestWrapBase64MultipleLines(t *testing.T) {
	input := strings.Repeat("A", 200)
	result := WrapBase64(input)

	lines := strings.Split(result, "\r\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if len(lines[0]) != 76 {
		t.Errorf("first line length: expected 76, got %d", len(lines[0]))
	}
	if len(lines[1]) != 76 {
		t.Errorf("second line length: expected 76, got %d", len(lines[1]))
	}
	if len(lines[2]) != 200-152 {
		t.Errorf("third line length: expected %d, got %d", 200-152, len(lines[2]))
	}
}

func TestWrapBase64MatchesGoImpl(t *testing.T) {
	// Reference Go implementation (the original from sender.go)
	goWrapBase64 := func(data string) string {
		const lineLength = 76
		var result strings.Builder
		for i := 0; i < len(data); i += lineLength {
			end := i + lineLength
			if end > len(data) {
				end = len(data)
			}
			result.WriteString(data[i:end])
			if end < len(data) {
				result.WriteString("\r\n")
			}
		}
		return result.String()
	}

	tests := []string{
		"",
		"A",
		strings.Repeat("B", 75),
		strings.Repeat("C", 76),
		strings.Repeat("D", 77),
		strings.Repeat("E", 152),
		strings.Repeat("F", 153),
		strings.Repeat("G", 1000),
		strings.Repeat("H", 10000),
	}

	for _, input := range tests {
		expected := goWrapBase64(input)
		got := WrapBase64(input)
		if got != expected {
			t.Errorf("mismatch for len=%d:\nexpected: %q\ngot:      %q", len(input), expected, got)
		}
	}
}
