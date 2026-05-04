package jmap

import (
	"testing"
	"time"

	jmapclient "git.sr.ht/~rockorager/go-jmap"
	"github.com/floatpane/matcha/backend"
)

func TestBuildSearchFilter(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	f := buildSearchFilter(jmapclient.ID("mailbox-id"), backend.SearchQuery{
		From: "alice@example.com", To: "bob@example.com", Subject: "invoice",
		Body: "paid", Since: since, Before: before, LargerThan: 4096,
	})

	if f.InMailbox != "mailbox-id" || f.From != "alice@example.com" || f.To != "bob@example.com" ||
		f.Subject != "invoice" || f.Body != "paid" || f.MinSize != 4096 {
		t.Fatalf("filter = %+v", f)
	}
	if f.After == nil || !f.After.Equal(since) || f.Before == nil || !f.Before.Equal(before) {
		t.Fatalf("date filters = after:%v before:%v", f.After, f.Before)
	}
}
