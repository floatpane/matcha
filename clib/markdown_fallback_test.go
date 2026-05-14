package clib

import (
	"strings"
	"testing"
)

func TestMarkdownPlainTextHTMLFallbackEscapesInput(t *testing.T) {
	input := []byte("# Heading\n\n<script>alert(\"x\")</script>\n<b>literal</b>")

	got := string(markdownPlainTextHTML(input))

	if !strings.HasPrefix(got, "<pre>") || !strings.HasSuffix(got, "</pre>") {
		t.Fatalf("fallback should wrap content in pre tag, got %q", got)
	}
	if strings.Contains(got, "<script>") || strings.Contains(got, "<b>") {
		t.Fatalf("fallback should escape HTML-looking input, got %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;") {
		t.Fatalf("fallback missing escaped script text, got %q", got)
	}
	if !strings.Contains(got, "# Heading") {
		t.Fatalf("fallback should preserve markdown text, got %q", got)
	}
}
