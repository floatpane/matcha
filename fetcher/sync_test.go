package fetcher

import (
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/floatpane/matcha/config"
)

func TestMergeSync(t *testing.T) {
	day := func(n int) time.Time { return time.Date(2026, 11, n, 12, 0, 0, 0, time.UTC) }
	known := []Email{
		{UID: 1, Subject: "deleted", Date: day(1)},
		{UID: 2, Subject: "read elsewhere", Date: day(2)},
		{UID: 3, Subject: "unchanged", Date: day(3), IsRead: true},
	}
	present := map[uint32]struct{}{2: {}, 3: {}, 4: {}}
	flags := map[uint32]bool{2: true, 99: true} // 99: flag change of a mail we never listed
	fresh := []Email{{UID: 4, Subject: "new", Date: day(4)}}

	got := mergeSync(known, present, flags, fresh)

	want := []struct {
		uid  uint32
		read bool
	}{{4, false}, {3, true}, {2, true}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, w := range want {
		if got[i].UID != w.uid || got[i].IsRead != w.read {
			t.Fatalf("got[%d] = UID %d read %v, want UID %d read %v", i, got[i].UID, got[i].IsRead, w.uid, w.read)
		}
	}
}

func TestMergeSyncFreshReplacesKnownCopy(t *testing.T) {
	known := []Email{{UID: 5, Subject: "old copy"}}
	present := map[uint32]struct{}{5: {}}
	got := mergeSync(known, present, nil, []Email{{UID: 5, Subject: "new copy"}})
	if len(got) != 1 || got[0].Subject != "new copy" {
		t.Fatalf("got %+v", got)
	}
}

func TestNeedsFullSync(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	prev := config.FolderSyncState{UIDValidity: 7, HighestModSeq: 100, FullAt: now.Add(-time.Hour)}
	sel := &imap.SelectData{UIDValidity: 7, HighestModSeq: 120}

	if needsFullSync(prev, true, sel, true, now) {
		t.Fatal("fresh state with CONDSTORE should sync incrementally")
	}
	cases := map[string]func() bool{
		"no state":         func() bool { return needsFullSync(prev, false, sel, true, now) },
		"no CONDSTORE":     func() bool { return needsFullSync(prev, true, sel, false, now) },
		"no HIGHESTMODSEQ": func() bool { return needsFullSync(prev, true, &imap.SelectData{UIDValidity: 7}, true, now) },
		"UIDVALIDITY change": func() bool {
			return needsFullSync(prev, true, &imap.SelectData{UIDValidity: 8, HighestModSeq: 120}, true, now)
		},
		"day since full": func() bool { return needsFullSync(prev, true, sel, true, now.Add(fullSyncInterval)) },
	}
	for name, full := range cases {
		if !full() {
			t.Errorf("%s: want full sync", name)
		}
	}
}

func TestCachedEmailRoundTripKeepsReplyTo(t *testing.T) {
	in := []Email{{UID: 1, ReplyTo: []string{"list@example.com"}, AccountID: "a"}}
	out := FromCachedEmails(ToCachedEmails(in))
	if len(out) != 1 || len(out[0].ReplyTo) != 1 || out[0].ReplyTo[0] != "list@example.com" {
		t.Fatalf("round trip = %+v", out)
	}
}
