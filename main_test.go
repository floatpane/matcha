package main

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeFilenameTruncatesCJKOnUTF8Boundary(t *testing.T) {
	name := strings.Repeat("文", 100) + ".txt"

	got := sanitizeFilename(name)

	if !utf8.ValidString(got) {
		t.Fatalf("sanitizeFilename returned invalid UTF-8: %q", got)
	}
	if len(got) > 255 {
		t.Fatalf("sanitizeFilename returned %d bytes, want at most 255", len(got))
	}
	if filepath.Ext(got) != ".txt" {
		t.Fatalf("sanitizeFilename lost extension: got %q", got)
	}
}

func TestSanitizeFilenameTruncatesEmojiOnUTF8Boundary(t *testing.T) {
	name := strings.Repeat("🚀", 80) + ".log"

	got := sanitizeFilename(name)

	if !utf8.ValidString(got) {
		t.Fatalf("sanitizeFilename returned invalid UTF-8: %q", got)
	}
	if len(got) > 255 {
		t.Fatalf("sanitizeFilename returned %d bytes, want at most 255", len(got))
	}
	if filepath.Ext(got) != ".log" {
		t.Fatalf("sanitizeFilename lost extension: got %q", got)
	}
}
