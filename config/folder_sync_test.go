package config

import (
	"path/filepath"
	"testing"
	"time"
)

func isolateCache(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

func TestFolderSyncStateRoundTrip(t *testing.T) {
	isolateCache(t)
	if _, ok := LoadFolderSyncState("INBOX", "a"); ok {
		t.Fatal("state before any save")
	}
	want := FolderSyncState{UIDValidity: 7, HighestModSeq: 42, Skipped: []uint32{3}, FullAt: time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)}
	if err := SaveFolderSyncState("INBOX", "a", want); err != nil {
		t.Fatal(err)
	}
	if err := SaveFolderSyncState("INBOX", "b", FolderSyncState{UIDValidity: 9}); err != nil {
		t.Fatal(err)
	}
	got, ok := LoadFolderSyncState("INBOX", "a")
	if !ok || got.UIDValidity != 7 || got.HighestModSeq != 42 || len(got.Skipped) != 1 || !got.FullAt.Equal(want.FullAt) {
		t.Fatalf("got %+v, %v", got, ok)
	}
	if b, ok := LoadFolderSyncState("INBOX", "b"); !ok || b.UIDValidity != 9 {
		t.Fatalf("account b = %+v, %v", b, ok)
	}
}

func TestReplaceAccountFolderEmails(t *testing.T) {
	isolateCache(t)
	day := func(n int) time.Time { return time.Date(2026, 11, n, 12, 0, 0, 0, time.UTC) }
	if err := SaveFolderEmailCache("INBOX", []CachedEmail{
		{UID: 1, AccountID: "a", Subject: "a gone", Date: day(1)},
		{UID: 1, AccountID: "b", Subject: "b kept", Date: day(2)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceAccountFolderEmails("INBOX", "a", []CachedEmail{
		{UID: 2, AccountID: "a", Subject: "a new", Date: day(3)},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFolderEmailCache("INBOX")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Subject != "a new" || got[1].Subject != "b kept" {
		t.Fatalf("got %+v", got)
	}
}
