package emailstore

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

func mkEmails(n int, accounts ...string) []fetcher.Email {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]fetcher.Email, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fetcher.Email{
			UID:       uint32(i),
			AccountID: accounts[i%len(accounts)],
			Subject:   fmt.Sprint(i),
			Date:      base.Add(time.Duration(i) * time.Minute),
		})
	}
	return out
}

// SetFolder runs on the UI goroutine for every folder load; the old O(n²)
// sort took ~0.8s for 20k emails. Compare growth rather than wall time so the
// check holds under -race and on slow machines: 8x the input must cost far
// less than the 64x a quadratic sort needs.
func TestSetFolderSortsLargeFoldersQuickly(t *testing.T) {
	measure := func(n int) time.Duration {
		best := time.Duration(1<<63 - 1)
		for r := 0; r < 3; r++ {
			emails := mkEmails(n, "a", "b")
			s := NewStore()
			start := time.Now()
			s.SetFolder("INBOX", emails)
			best = min(best, time.Since(start))
			if len(s.Emails) != n {
				t.Fatalf("len = %d, want %d", len(s.Emails), n)
			}
			for i := 1; i < len(s.Emails); i++ {
				if s.Emails[i].Date.After(s.Emails[i-1].Date) {
					t.Fatalf("not sorted newest first at %d", i)
				}
			}
		}
		return best
	}
	small, large := measure(4000), measure(32000)
	if ratio := float64(large) / float64(max(small, time.Microsecond)); ratio > 32 {
		t.Fatalf("SetFolder scales quadratically: 4k=%v 32k=%v (x%.0f)", small, large, ratio)
	}
}

func TestNewEmailsMatchesAccountAndUID(t *testing.T) {
	prev := []fetcher.Email{{UID: 1, AccountID: "a"}, {UID: 2, AccountID: "a"}}
	fetched := []fetcher.Email{{UID: 1, AccountID: "a"}, {UID: 1, AccountID: "b"}, {UID: 3, AccountID: "a"}}
	got := NewEmails(prev, fetched)
	if len(got) != 2 || got[0].AccountID != "b" || got[1].UID != 3 {
		t.Fatalf("NewEmails = %+v", got)
	}
}

// A failed account keeps its previously known emails instead of vanishing.
func TestMergeFolderFetchKeepsFailedAccounts(t *testing.T) {
	now := time.Now()
	prev := []fetcher.Email{
		{UID: 1, AccountID: "up", Date: now.Add(-3 * time.Hour)},
		{UID: 9, AccountID: "down", Date: now.Add(-time.Hour)},
	}
	fetched := []fetcher.Email{{UID: 2, AccountID: "up", Date: now}}
	got := MergeFolderFetch(prev, fetched, map[string]error{"down": errors.New("offline")})
	if len(got) != 2 || got[0].UID != 2 || got[1].UID != 9 {
		t.Fatalf("MergeFolderFetch = %+v", got)
	}
	if same := MergeFolderFetch(prev, fetched, nil); len(same) != 1 {
		t.Fatalf("no failures should return fetched as is, got %+v", same)
	}
}

// Bodies of an account whose fetch failed must survive the prune, and two
// accounts sharing a UID must both keep theirs.
func TestCacheFolderSnapshotSkipsFailedAccounts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	const th = 100 * 1024 * 1024
	for _, acct := range []string{"a", "b", "down"} {
		if err := config.SaveEmailBody("INBOX", config.CachedEmailBody{UID: 1, AccountID: acct, Body: "x"}, th); err != nil {
			t.Fatal(err)
		}
	}
	_ = config.SaveEmailBody("INBOX", config.CachedEmailBody{UID: 2, AccountID: "a", Body: "deleted on server"}, th)

	emails := []fetcher.Email{{UID: 1, AccountID: "a"}, {UID: 1, AccountID: "b"}}
	CacheFolderSnapshot("INBOX", emails, map[string]error{"down": errors.New("offline")}, []string{"a", "b", "down"}, th)

	deadline := time.Now().Add(5 * time.Second)
	for config.GetCachedEmailBody("INBOX", 2, "a", th) != nil {
		if time.Now().After(deadline) {
			t.Fatal("stale body for a/2 was not pruned")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, acct := range []string{"a", "b", "down"} {
		if config.GetCachedEmailBody("INBOX", 1, acct, th) == nil {
			t.Errorf("body for %s/1 was pruned", acct)
		}
	}
	waitForFolderCache(t, "INBOX", 2)
}

func waitForFolderCache(t *testing.T, folder string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := LoadFolderEmailsFromCache(folder)
		if len(got) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("folder cache has %d emails, want %d", len(got), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Background saves of one folder must end with the newest snapshot.
func TestSaveFolderEmailsAsyncNewestWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	for n := 1; n <= 30; n++ {
		SaveFolderEmailsAsync("Work", mkEmails(n, "a"))
	}
	waitForFolderCache(t, "Work", 30)
	time.Sleep(50 * time.Millisecond)
	if got := len(LoadFolderEmailsFromCache("Work")); got != 30 {
		t.Fatalf("stale snapshot overwrote newest: %d emails", got)
	}
}
