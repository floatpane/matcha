package fetcher

import (
	"strings"
	"testing"
)

func TestParseRawMessageBounce(t *testing.T) {
	raw := "From: MAILER-DAEMON@postfix\r\nTo: test@dovecot\r\nSubject: Undelivered Mail Returned to Sender\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/report; report-type=delivery-status; boundary=\"B\"\r\n\r\n" +
		"This is a MIME-encapsulated message.\r\n\r\n--B\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHost not found\r\n\r\n" +
		"--B\r\nContent-Type: message/global-delivery-status\r\n\r\nAction: failed\r\n\r\n--B--\r\n"
	body, mt := parseRawMessage([]byte(raw))
	if mt != mimeTextPlain || !strings.Contains(body, "Host not found") {
		t.Fatalf("got %q %q", mt, body)
	}
}
