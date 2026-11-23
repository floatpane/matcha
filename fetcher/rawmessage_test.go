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

func TestParseDigest(t *testing.T) {
	raw := "From: list-request@example.com\r\n" +
		"Subject: List Digest, Vol 1\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/digest; boundary=\"DIGEST\"\r\n\r\n" +
		"--DIGEST\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"Today=27s Topics:\r\n1. First topic\r\n" +
		"--DIGEST\r\n" +
		"Content-Type: message/rfc822\r\n\r\n" +
		"From: Alice <alice@example.com>\r\n" +
		"Subject: First topic\r\n" +
		"Date: Mon, 20 Nov 2026 10:00:00 +0000\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"INNER\"\r\n\r\n" +
		"--INNER\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Hello from Alice\r\n" +
		"--INNER\r\n" +
		"Content-Type: application/pdf; name=\"doc.pdf\"\r\n" +
		"Content-Disposition: attachment; filename=\"doc.pdf\"\r\n\r\n" +
		"PDFDATA\r\n" +
		"--INNER--\r\n" +
		"--DIGEST\r\n\r\n" +
		"From: Bob <bob@example.com>\r\n" +
		"Subject: Implicit RFC822\r\n" +
		"Date: Mon, 20 Nov 2026 11:00:00 +0000\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Hello from Bob\r\n" +
		"--DIGEST--\r\n"

	body, atts, ok := parseDigest([]byte(raw))
	if !ok {
		t.Fatal("parseDigest returned ok=false")
	}
	for _, want := range []string{
		"Today's Topics:",
		"Alice <alice@example.com>",
		"Hello from Alice",
		"Bob <bob@example.com>",
		"Hello from Bob",
		"doc.pdf",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("digest body missing %q:\n%s", want, body)
		}
	}
	if len(atts) != 1 || atts[0].Filename != "doc.pdf" || string(atts[0].Data) != "PDFDATA" {
		t.Fatalf("unexpected attachments: %+v", atts)
	}
}

func TestParseDigestNoMessages(t *testing.T) {
	raw := "From: a@example.com\r\nSubject: Plain\r\nContent-Type: text/plain\r\n\r\nJust text\r\n"
	if _, _, ok := parseDigest([]byte(raw)); ok {
		t.Fatal("expected ok=false for non-digest message")
	}
}
