package cli

import (
	"strings"
	"testing"
)

func TestRewriteToHeader(t *testing.T) {
	const raw = "From: sender@example.com\r\nSubject: [PATCH] fix\r\n\r\nbody"
	got := string(rewriteToHeader([]byte(raw), "to@example.com"))
	if !strings.Contains(got, "To: to@example.com") {
		t.Errorf("rewriteToHeader did not insert To header:\n%s", got)
	}
	if !strings.Contains(got, "From: sender@example.com") {
		t.Errorf("rewriteToHeader removed From header:\n%s", got)
	}
	if !strings.Contains(got, "body") {
		t.Errorf("rewriteToHeader removed body:\n%s", got)
	}
}

func TestRewriteToHeaderReplacesExisting(t *testing.T) {
	const raw = "From: sender@example.com\r\nTo: old@example.com\r\nSubject: [PATCH] fix\r\n\r\nbody"
	got := string(rewriteToHeader([]byte(raw), "new@example.com"))
	if strings.Contains(got, "To: old@example.com") {
		t.Errorf("rewriteToHeader did not replace old To header:\n%s", got)
	}
	if !strings.Contains(got, "To: new@example.com") {
		t.Errorf("rewriteToHeader did not insert new To header:\n%s", got)
	}
}

func TestRewriteCcHeader(t *testing.T) {
	const raw = "From: sender@example.com\r\nSubject: [PATCH] fix\r\n\r\nbody"
	got := string(rewriteCcHeader([]byte(raw), "cc1@example.com, cc2@example.com"))
	if !strings.Contains(got, "Cc: cc1@example.com, cc2@example.com") {
		t.Errorf("rewriteCcHeader did not insert Cc header:\n%s", got)
	}
}

func TestRewriteHeaderLFLineEndings(t *testing.T) {
	const raw = "From: sender@example.com\nSubject: [PATCH] fix\n\nbody"
	got := string(rewriteToHeader([]byte(raw), "to@example.com"))
	if !strings.Contains(got, "To: to@example.com") {
		t.Errorf("rewriteToHeader did not handle LF line endings:\n%s", got)
	}
	if strings.Contains(got, "\r\n") {
		t.Errorf("rewriteToHeader changed LF to CRLF:\n%s", got)
	}
}

func TestRewriteHeaderNoEmptyTo(t *testing.T) {
	const raw = "From: sender@example.com\r\nSubject: [PATCH] fix\r\n\r\nbody"
	got := string(rewriteToHeader([]byte(raw), ""))
	if strings.Contains(got, "To:") {
		t.Errorf("rewriteToHeader inserted empty To header:\n%s", got)
	}
}
