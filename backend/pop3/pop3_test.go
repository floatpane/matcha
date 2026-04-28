package pop3

import (
	"context"
	"errors"
	"testing"

	"github.com/emersion/go-message"
	"github.com/floatpane/matcha/backend"
	pop3client "github.com/knadh/go-pop3"
)

func TestEntityToEmail_ReplyTo(t *testing.T) {
	tests := []struct {
		name          string
		replyToHeader string
		wantReplyTo   []string
	}{
		{
			name:          "single bare address",
			replyToHeader: "alice@example.com",
			wantReplyTo:   []string{"alice@example.com"},
		},
		{
			name:          "single address with display name",
			replyToHeader: "Alice Smith <alice@example.com>",
			wantReplyTo:   []string{"alice@example.com"},
		},
		{
			name:          "display name with comma",
			replyToHeader: `"Doe, John" <john@example.com>`,
			wantReplyTo:   []string{"john@example.com"},
		},
		{
			name:          "multiple addresses",
			replyToHeader: "alice@example.com, bob@example.com",
			wantReplyTo:   []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:          "multiple addresses with display names containing commas",
			replyToHeader: `"Doe, John" <john@example.com>, "Smith, Jane" <jane@example.com>`,
			wantReplyTo:   []string{"john@example.com", "jane@example.com"},
		},
		{
			name:          "empty reply-to",
			replyToHeader: "",
			wantReplyTo:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var header message.Header
			header.Set("From", "sender@example.com")
			header.Set("Subject", "Test")
			if tt.replyToHeader != "" {
				header.Set("Reply-To", tt.replyToHeader)
			}

			msgInfo := pop3client.MessageID{ID: 1, UID: "test-uid"}
			email := entityToEmail(&header, msgInfo, "test-account")

			if len(email.ReplyTo) != len(tt.wantReplyTo) {
				t.Fatalf("ReplyTo length = %d, want %d; got %v", len(email.ReplyTo), len(tt.wantReplyTo), email.ReplyTo)
			}
			for i, want := range tt.wantReplyTo {
				if email.ReplyTo[i] != want {
					t.Errorf("ReplyTo[%d] = %q, want %q", i, email.ReplyTo[i], want)
				}
			}
		})
	}
}

func TestEntityToEmail_To(t *testing.T) {
	tests := []struct {
		name     string
		toHeader string
		wantTo   []string
	}{
		{
			name:     "display name with comma",
			toHeader: `"Doe, John" <john@example.com>`,
			wantTo:   []string{"john@example.com"},
		},
		{
			name:     "multiple addresses with display names",
			toHeader: `"Doe, John" <john@example.com>, Alice <alice@example.com>`,
			wantTo:   []string{"john@example.com", "alice@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var header message.Header
			header.Set("From", "sender@example.com")
			header.Set("To", tt.toHeader)

			msgInfo := pop3client.MessageID{ID: 1, UID: "test-uid"}
			email := entityToEmail(&header, msgInfo, "test-account")

			if len(email.To) != len(tt.wantTo) {
				t.Fatalf("To length = %d, want %d; got %v", len(email.To), len(tt.wantTo), email.To)
			}
			for i, want := range tt.wantTo {
				if email.To[i] != want {
					t.Errorf("To[%d] = %q, want %q", i, email.To[i], want)
				}
			}
		})
	}
}

func TestSearchNotSupported(t *testing.T) {
	p := &Provider{}
	_, err := p.Search(context.Background(), "INBOX", backend.SearchQuery{Raw: "subject:test"})
	if !errors.Is(err, backend.ErrNotSupported) {
		t.Fatalf("Search error = %v, want ErrNotSupported", err)
	}
}
