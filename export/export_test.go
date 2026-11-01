package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/floatpane/matcha/fetcher"
)

func sampleRawEmail() []byte {
	return []byte(`From: sender@example.com
To: recipient@example.com
Cc: cc@example.com
Subject: Test Email Export
Date: Mon, 02 Jan 2024 15:04:05 -0700
Message-ID: <test123@example.com>
In-Reply-To: <prev456@example.com>
References: <prev456@example.com> <thread789@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="boundary123"

--boundary123
Content-Type: text/plain; charset=utf-8

Hello, this is a test email.
It has multiple lines.

--boundary123
Content-Type: text/html; charset=utf-8

<html><body><h1>Hello</h1><p>This is a <b>test</b> email.</p></body></html>

--boundary123--
`)
}

func sampleEmail() fetcher.Email {
	return fetcher.Email{
		UID:          42,
		From:         "sender@example.com",
		To:           []string{"recipient@example.com"},
		Subject:      "Test Email Export",
		Body:         "Hello, this is a test email.",
		BodyMIMEType: "text/plain",
		MessageID:    "<test123@example.com>",
		InReplyTo:    "<prev456@example.com>",
		References:   []string{"<prev456@example.com>", "<thread789@example.com>"},
	}
}

func TestParseMetadata(t *testing.T) {
	raw := sampleRawEmail()
	meta, err := ParseMetadata(raw)
	if err != nil {
		t.Fatalf("ParseMetadata failed: %v", err)
	}
	if meta.Subject != "Test Email Export" {
		t.Errorf("expected subject 'Test Email Export', got %q", meta.Subject)
	}
	if meta.From != "sender@example.com" {
		t.Errorf("expected from 'sender@example.com', got %q", meta.From)
	}
	if len(meta.To) != 1 || meta.To[0] != "recipient@example.com" {
		t.Errorf("expected To to contain 'recipient@example.com', got %v", meta.To)
	}
	if len(meta.Cc) != 1 || meta.Cc[0] != "cc@example.com" {
		t.Errorf("expected Cc to contain 'cc@example.com', got %v", meta.Cc)
	}
	if !meta.HasDate {
		t.Error("expected HasDate to be true")
	}
	if meta.MessageID != "<test123@example.com>" {
		t.Errorf("expected MessageID '<test123@example.com>', got %q", meta.MessageID)
	}
	if meta.InReplyTo != "<prev456@example.com>" {
		t.Errorf("expected InReplyTo '<prev456@example.com>', got %q", meta.InReplyTo)
	}
	if len(meta.RawHeaders) == 0 {
		t.Error("expected RawHeaders to be non-empty")
	}
}

func TestExtractBodyHTML(t *testing.T) {
	raw := sampleRawEmail()
	htmlBody := ExtractBodyHTML(raw)
	if !strings.Contains(htmlBody, "<h1>Hello</h1>") {
		t.Errorf("expected HTML body to contain '<h1>Hello</h1>', got %q", htmlBody)
	}
}

func TestExtractBodyText(t *testing.T) {
	raw := sampleRawEmail()
	textBody := ExtractBodyText(raw)
	if !strings.Contains(textBody, "Hello, this is a test email") {
		t.Errorf("expected text body to contain 'Hello, this is a test email', got %q", textBody)
	}
}

func TestEmailToHTML(t *testing.T) {
	raw := sampleRawEmail()
	email := sampleEmail()
	data, err := EmailToHTML(raw, email)
	if err != nil {
		t.Fatalf("EmailToHTML failed: %v", err)
	}
	htmlStr := string(data)
	if !strings.Contains(htmlStr, "<!DOCTYPE html>") {
		t.Error("expected HTML output to contain DOCTYPE")
	}
	if !strings.Contains(htmlStr, "Test Email Export") {
		t.Error("expected HTML output to contain subject")
	}
	if !strings.Contains(htmlStr, "sender@example.com") {
		t.Error("expected HTML output to contain From address")
	}
	if !strings.Contains(htmlStr, "All Headers") {
		t.Error("expected HTML output to contain all headers section")
	}
	if !strings.Contains(htmlStr, "<h1>Hello</h1>") {
		t.Error("expected HTML output to contain original HTML body")
	}
}

func TestEmailToMarkdown(t *testing.T) {
	raw := sampleRawEmail()
	email := sampleEmail()
	data, err := EmailToMarkdown(raw, email)
	if err != nil {
		t.Fatalf("EmailToMarkdown failed: %v", err)
	}
	mdStr := string(data)
	if !strings.Contains(mdStr, "# Test Email Export") {
		t.Error("expected Markdown output to contain subject as H1")
	}
	if !strings.Contains(mdStr, "sender@example.com") {
		t.Error("expected Markdown output to contain From address")
	}
	if !strings.Contains(mdStr, "All Headers") {
		t.Error("expected Markdown output to contain all headers section")
	}
	if !strings.Contains(mdStr, "Hello, this is a test email") {
		t.Error("expected Markdown output to contain body text")
	}
}

func TestSuggestFilename(t *testing.T) {
	tests := []struct {
		subject string
		format  string
		want    string
	}{
		{"Test Email", "html", "Test Email.html"},
		{"Test Email", "markdown", "Test Email.md"},
		{"Test Email", "md", "Test Email.md"},
		{"Test Email", "eml", "Test Email.eml"},
		{"", "html", "email.html"},
		{"Re: Hello/World", "html", "Re- Hello-World.html"},
		{"Test:Email*With?Bad\"Chars<>|", "html", "Test-Email-With-Bad-Chars---.html"},
	}
	for _, tt := range tests {
		got := SuggestFilename(tt.subject, tt.format)
		if got != tt.want {
			t.Errorf("SuggestFilename(%q, %q) = %q, want %q", tt.subject, tt.format, got, tt.want)
		}
	}
}

func TestWriteToFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "export", "test.html")
	data := []byte("<html>test</html>")
	if err := WriteToFile(path, data); err != nil {
		t.Fatalf("WriteToFile failed: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(written) != string(data) {
		t.Errorf("expected %q, got %q", data, written)
	}
}
