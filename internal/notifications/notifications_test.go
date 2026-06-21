package notifications

import (
	"testing"

	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/emailstore"
)

func TestCountUnreadDeduplicatesOverlappingStores(t *testing.T) {
	email := fetcher.Email{UID: 42, AccountID: "acct-a"}
	got := CountUnread(
		map[string][]fetcher.Email{
			"acct-a": {email},
		},
		map[string][]fetcher.Email{
			emailstore.FolderInbox: {email},
		},
	)

	if got != 1 {
		t.Fatalf("CountUnread() = %d, want 1", got)
	}
}
