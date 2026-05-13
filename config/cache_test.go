package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSaveEmailBodyEvictsLeastRecentlyAccessedAcrossFolders(t *testing.T) {
	folderCacheTestSetup(t)

	oldTime := time.Now().Add(-2 * time.Hour)
	recentTime := time.Now().Add(-1 * time.Hour)

	if err := saveEmailBodyCache(&EmailBodyCache{
		FolderName: "INBOX",
		Bodies: []CachedEmailBody{
			{
				UID:            1,
				AccountID:      "acct",
				Body:           strings.Repeat("a", 10),
				SizeBytes:      10,
				CachedAt:       oldTime,
				LastAccessedAt: oldTime,
			},
		},
	}); err != nil {
		t.Fatalf("save old cache: %v", err)
	}

	if err := saveEmailBodyCache(&EmailBodyCache{
		FolderName: "Archive",
		Bodies: []CachedEmailBody{
			{
				UID:            2,
				AccountID:      "acct",
				Body:           strings.Repeat("b", 10),
				SizeBytes:      10,
				CachedAt:       recentTime,
				LastAccessedAt: recentTime,
			},
		},
	}); err != nil {
		t.Fatalf("save recent cache: %v", err)
	}

	if err := SaveEmailBody("Sent", CachedEmailBody{
		UID:       3,
		AccountID: "acct",
		Body:      strings.Repeat("c", 10),
	}, 20); err != nil {
		t.Fatalf("SaveEmailBody: %v", err)
	}

	inbox, err := LoadEmailBodyCache("INBOX")
	if err != nil {
		t.Fatalf("LoadEmailBodyCache(INBOX): %v", err)
	}
	if len(inbox.Bodies) != 0 {
		t.Fatalf("oldest INBOX body should be evicted, got %d bodies", len(inbox.Bodies))
	}

	archive, err := LoadEmailBodyCache("Archive")
	if err != nil {
		t.Fatalf("LoadEmailBodyCache(Archive): %v", err)
	}
	if len(archive.Bodies) != 1 || archive.Bodies[0].UID != 2 {
		t.Fatalf("recent Archive body should remain, got %+v", archive.Bodies)
	}

	sent, err := LoadEmailBodyCache("Sent")
	if err != nil {
		t.Fatalf("LoadEmailBodyCache(Sent): %v", err)
	}
	if len(sent.Bodies) != 1 || sent.Bodies[0].UID != 3 {
		t.Fatalf("new Sent body should remain, got %+v", sent.Bodies)
	}
}

func TestSaveEmailBodyEvictsMultipleEntriesUntilUnderLimit(t *testing.T) {
	folderCacheTestSetup(t)

	now := time.Now()
	bodies := make([]CachedEmailBody, 0, 4)
	for i := 1; i <= 4; i++ {
		accessedAt := now.Add(-time.Duration(5-i) * time.Minute)
		bodies = append(bodies, CachedEmailBody{
			UID:            uint32(i),
			AccountID:      "acct",
			Body:           strings.Repeat(string(rune('a'+i-1)), 10),
			SizeBytes:      10,
			CachedAt:       accessedAt,
			LastAccessedAt: accessedAt,
		})
	}

	if err := saveEmailBodyCache(&EmailBodyCache{
		FolderName: "INBOX",
		Bodies:     bodies,
	}); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	if err := SaveEmailBody("Archive", CachedEmailBody{
		UID:       5,
		AccountID: "acct",
		Body:      strings.Repeat("e", 30),
	}, 50); err != nil {
		t.Fatalf("SaveEmailBody: %v", err)
	}

	inbox, err := LoadEmailBodyCache("INBOX")
	if err != nil {
		t.Fatalf("LoadEmailBodyCache(INBOX): %v", err)
	}

	gotUIDs := make([]uint32, 0, len(inbox.Bodies))
	for _, body := range inbox.Bodies {
		gotUIDs = append(gotUIDs, body.UID)
	}
	wantUIDs := []uint32{3, 4}
	if !reflect.DeepEqual(gotUIDs, wantUIDs) {
		t.Fatalf("remaining INBOX UIDs = %v, want %v", gotUIDs, wantUIDs)
	}

	archive, err := LoadEmailBodyCache("Archive")
	if err != nil {
		t.Fatalf("LoadEmailBodyCache(Archive): %v", err)
	}
	if len(archive.Bodies) != 1 || archive.Bodies[0].UID != 5 {
		t.Fatalf("new Archive body should remain, got %+v", archive.Bodies)
	}
}

func TestSaveEmailBodyDropsOversizedReplacement(t *testing.T) {
	folderCacheTestSetup(t)

	if err := SaveEmailBody("INBOX", CachedEmailBody{
		UID:       1,
		AccountID: "acct",
		Body:      strings.Repeat("a", 10),
	}, 20); err != nil {
		t.Fatalf("initial SaveEmailBody: %v", err)
	}

	if err := SaveEmailBody("INBOX", CachedEmailBody{
		UID:       1,
		AccountID: "acct",
		Body:      strings.Repeat("b", 25),
	}, 20); err != nil {
		t.Fatalf("oversized SaveEmailBody: %v", err)
	}

	cache, err := LoadEmailBodyCache("INBOX")
	if err != nil {
		t.Fatalf("LoadEmailBodyCache: %v", err)
	}
	if len(cache.Bodies) != 0 {
		t.Fatalf("oversized replacement should not remain cached, got %+v", cache.Bodies)
	}
}
