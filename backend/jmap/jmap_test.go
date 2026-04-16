package jmap

import (
	"testing"

	"git.sr.ht/~rockorager/go-jmap/mail"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
)

func TestJmapEmailToBackend_ReplyTo(t *testing.T) {
	tests := []struct {
		name        string
		replyTo     []*mail.Address
		wantReplyTo []string
	}{
		{
			name:        "single bare address",
			replyTo:     []*mail.Address{{Email: "alice@example.com"}},
			wantReplyTo: []string{"alice@example.com"},
		},
		{
			name:        "address with display name returns only email",
			replyTo:     []*mail.Address{{Name: "Alice Smith", Email: "alice@example.com"}},
			wantReplyTo: []string{"alice@example.com"},
		},
		{
			name: "display name with comma returns only email",
			replyTo: []*mail.Address{
				{Name: "Doe, John", Email: "john@example.com"},
			},
			wantReplyTo: []string{"john@example.com"},
		},
		{
			name: "multiple addresses with display names",
			replyTo: []*mail.Address{
				{Name: "Doe, John", Email: "john@example.com"},
				{Name: "Smith, Jane", Email: "jane@example.com"},
			},
			wantReplyTo: []string{"john@example.com", "jane@example.com"},
		},
		{
			name:        "empty reply-to",
			replyTo:     nil,
			wantReplyTo: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eml := &email.Email{
				ReplyTo: tt.replyTo,
			}
			result := jmapEmailToBackend(eml, 1, "test-account")

			if len(result.ReplyTo) != len(tt.wantReplyTo) {
				t.Fatalf("ReplyTo length = %d, want %d; got %v", len(result.ReplyTo), len(tt.wantReplyTo), result.ReplyTo)
			}
			for i, want := range tt.wantReplyTo {
				if result.ReplyTo[i] != want {
					t.Errorf("ReplyTo[%d] = %q, want %q", i, result.ReplyTo[i], want)
				}
			}
		})
	}
}

func TestJmapEmailToBackend_To(t *testing.T) {
	tests := []struct {
		name   string
		to     []*mail.Address
		wantTo []string
	}{
		{
			name:   "address with display name returns only email",
			to:     []*mail.Address{{Name: "Alice Smith", Email: "alice@example.com"}},
			wantTo: []string{"alice@example.com"},
		},
		{
			name: "multiple addresses return only emails",
			to: []*mail.Address{
				{Name: "Doe, John", Email: "john@example.com"},
				{Name: "Alice", Email: "alice@example.com"},
			},
			wantTo: []string{"john@example.com", "alice@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eml := &email.Email{
				To: tt.to,
			}
			result := jmapEmailToBackend(eml, 1, "test-account")

			if len(result.To) != len(tt.wantTo) {
				t.Fatalf("To length = %d, want %d; got %v", len(result.To), len(tt.wantTo), result.To)
			}
			for i, want := range tt.wantTo {
				if result.To[i] != want {
					t.Errorf("To[%d] = %q, want %q", i, result.To[i], want)
				}
			}
		})
	}
}

func TestJmapEmailToBackend_From(t *testing.T) {
	tests := []struct {
		name     string
		from     []*mail.Address
		wantFrom string
	}{
		{
			name:     "from with display name includes name",
			from:     []*mail.Address{{Name: "Alice Smith", Email: "alice@example.com"}},
			wantFrom: "Alice Smith <alice@example.com>",
		},
		{
			name:     "from without display name returns bare email",
			from:     []*mail.Address{{Email: "alice@example.com"}},
			wantFrom: "alice@example.com",
		},
		{
			name:     "empty from",
			from:     nil,
			wantFrom: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eml := &email.Email{
				From: tt.from,
			}
			result := jmapEmailToBackend(eml, 1, "test-account")

			if result.From != tt.wantFrom {
				t.Errorf("From = %q, want %q", result.From, tt.wantFrom)
			}
		})
	}
}
