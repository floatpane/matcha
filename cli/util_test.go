package cli

import (
	"flag"
	"testing"
)

func TestNormalizeFolder(t *testing.T) {
	// Test standard values
	tests := []struct {
		input    string
		expected string
	}{
		{"", "INBOX"},
		{"inbox", "INBOX"},
		{"INBOX", "INBOX"},
		{"Sent", "Sent"},
		{"sent", "sent"},
		{"Trash", "Trash"},
		{"Archive", "Archive"},
		{"custom-folder", "custom-folder"},
		{"Posteingang", "Posteingang"},
		{"Gesendet", "Gesendet"},
	}

	for _, tt := range tests {
		if got := NormalizeFolder(tt.input); got != tt.expected {
			t.Errorf("NormalizeFolder(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSanitizeTextForTable(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello\tworld", "hello world"},
		{"line1\nline2", "line1 line2"},
		{"line1\r\nline2", "line1  line2"},
		{"normal text", "normal text"},
	}

	for _, tt := range tests {
		if got := sanitizeTextForTable(tt.input); got != tt.expected {
			t.Errorf("sanitizeTextForTable(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseInterspersed(t *testing.T) {
	importFlag := func(args []string) ([]string, string, bool, error) {
		fs := flag.NewFlagSet("test-interspersed", flag.ContinueOnError)
		from := fs.String("from", "", "")
		jsonOut := fs.Bool("json", false, "")
		pos, err := parseInterspersed(fs, args)
		return pos, *from, *jsonOut, err
	}

	tests := []struct {
		args         []string
		expectedPos  []string
		expectedFrom string
		expectedJSON bool
	}{
		{
			args:         []string{"INBOX", "--json"},
			expectedPos:  []string{"INBOX"},
			expectedFrom: "",
			expectedJSON: true,
		},
		{
			args:         []string{"INBOX", "1,2,3", "--from", "work@example.com"},
			expectedPos:  []string{"INBOX", "1,2,3"},
			expectedFrom: "work@example.com",
			expectedJSON: false,
		},
		{
			args:         []string{"--from", "work@example.com", "--json", "INBOX", "123"},
			expectedPos:  []string{"INBOX", "123"},
			expectedFrom: "work@example.com",
			expectedJSON: true,
		},
	}

	for _, tt := range tests {
		pos, from, jsonOut, err := importFlag(tt.args)
		if err != nil {
			t.Fatalf("unexpected error parsing interspersed: %v", err)
		}
		if len(pos) != len(tt.expectedPos) {
			t.Errorf("for args %v: expected positionals %v, got %v", tt.args, tt.expectedPos, pos)
			continue
		}
		for i := range pos {
			if pos[i] != tt.expectedPos[i] {
				t.Errorf("for args %v: expected positional %d to be %q, got %q", tt.args, i, tt.expectedPos[i], pos[i])
			}
		}
		if from != tt.expectedFrom {
			t.Errorf("for args %v: expected from %q, got %q", tt.args, tt.expectedFrom, from)
		}
		if jsonOut != tt.expectedJSON {
			t.Errorf("for args %v: expected json %v, got %v", tt.args, tt.expectedJSON, jsonOut)
		}
	}
}
