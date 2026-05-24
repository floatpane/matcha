package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const threshold = 10000

func setup(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetLRU()
}

func createBody(uid uint32, accountID, text string) CachedEmailBody {
	return CachedEmailBody{
		UID:       uid,
		AccountID: accountID,
		Body:      text,
		CachedAt:  time.Now(),
		SizeBytes: len(text),
	}
}

func createDraft(id, accountID, subject string) Draft {
	return Draft{
		ID:        id,
		AccountID: accountID,
		To:        "t@e.com",
		Subject:   subject,
		Body:      "This is a draft.",
	}
}

func TestEmailCache_SaveLoadRoundTrip(t *testing.T) {
	setup(t)

	email1 := CachedEmail{
		UID:       1,
		From:      "t1@e.com",
		To:        []string{"t2@e.com"},
		Subject:   "Hello",
		AccountID: "AC1",
		IsRead:    false,
		Date:      time.Now().Truncate(time.Second),
	}

	email2 := CachedEmail{
		UID:       2,
		From:      "t2@e.com",
		To:        []string{"t1@e.com"},
		Subject:   "Hello",
		AccountID: "AC2",
		IsRead:    true,
		Date:      time.Now().Truncate(time.Second),
	}

	want := &EmailCache{Emails: []CachedEmail{email1, email2}}

	if err := SaveEmailCache(want); err != nil {
		t.Fatalf("SaveEmailCache: %v", err)
	}

	emailCache, err := LoadEmailCache()

	if err != nil {
		t.Fatalf("LoadEmailCache: %v", err)
	}

	if len(emailCache.Emails) != len(want.Emails) {
		t.Fatalf("email count: got %d, want %d", len(emailCache.Emails), len(want.Emails))
	}

	for i, e := range emailCache.Emails {
		w := want.Emails[i]
		if e.UID != w.UID || e.From != w.From || e.Subject != w.Subject || e.IsRead != w.IsRead {
			t.Errorf("email[%d] mismatch: got %+v, want %+v", i, e, w)
		}
	}
}

func TestEmailCache_HasEmailCache_FalseWhenMissing(t *testing.T) {
	setup(t)
	if HasEmailCache() {
		t.Error("HasEmailCache should be false before any save")
	}
}

func TestEmailCache_HasEmailCache_TrueAfterSave(t *testing.T) {
	setup(t)
	if err := SaveEmailCache(&EmailCache{}); err != nil {
		t.Fatalf("SaveEmailCache: %v", err)
	}
	if !HasEmailCache() {
		t.Error("HasEmailCache should be true after save")
	}
}

func TestEmailCache_ClearEmailCache(t *testing.T) {
	setup(t)
	if err := SaveEmailCache(&EmailCache{Emails: []CachedEmail{{UID: 1, AccountID: "AC1"}}}); err != nil {
		t.Fatalf("SaveEmailCache: %v", err)
	}
	if err := ClearEmailCache(); err != nil {
		t.Fatalf("ClearEmailCache: %v", err)
	}
	if HasEmailCache() {
		t.Error("HasEmailCache should be false after clear")
	}
}

func TestEmailCache_LoadCorruptFile(t *testing.T) {
	setup(t)
	if err := SaveEmailCache(&EmailCache{}); err != nil {
		t.Fatalf("SaveEmailCache: %v", err)
	}
	path, err := cacheFile()
	if err != nil {
		t.Fatalf("cacheFile: %v", err)
	}
	if err := os.WriteFile(path, []byte("{invalid json}"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err = LoadEmailCache(); err == nil {
		t.Error("LoadEmailCache should return an error for corrupt JSON")
	}
}

func TestEmailCache_RemoveAccount(t *testing.T) {
	setup(t)

	email1 := CachedEmail{UID: 1, AccountID: "AC1"}
	email2 := CachedEmail{UID: 2, AccountID: "AC2"}
	email3 := CachedEmail{UID: 3, AccountID: "AC3"}

	if err := SaveEmailCache(&EmailCache{Emails: []CachedEmail{email1, email2, email3}}); err != nil {
		t.Fatalf("SaveEmailCache: %v", err)
	}

	if err := removeAccountFromEmailCache("AC2"); err != nil {
		t.Fatalf("removeAccountFromEmailCache: %v", err)
	}

	emailCache, err := LoadEmailCache()

	if err != nil {
		t.Fatalf("LoadEmailCache: %v", err)
	}

	for _, e := range emailCache.Emails {
		if e.AccountID == "AC2" {
			t.Errorf("found email belonging to removed account AC2: %+v", e)
		}
	}
	if len(emailCache.Emails) != 2 {
		t.Errorf("expected 1 email remaining, got %d", len(emailCache.Emails))
	}
}

func TestContacts_SearchEmpty(t *testing.T) {
	setup(t)
	if results := SearchContacts(""); len(results) != 0 {
		t.Errorf("SearchContacts(\"\") should return nil, got %d results", len(results))
	}
}

func TestContacts_LoadCorruptFile(t *testing.T) {
	setup(t)
	path, err := GetContactsCachePath()
	if err != nil {
		t.Fatalf("GetContactsCachePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{invalid json}"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err = LoadContactsCache(); err == nil {
		t.Error("LoadContactsCache should error on corrupt JSON")
	}
}

func TestDrafts_SaveLoadRoundTrip(t *testing.T) {
	setup(t)
	d := createDraft("D1", "AC1", "Subject")
	if err := SaveDraft(d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	draft := GetDraft("D1")
	if draft == nil {
		t.Fatal("GetDraft returned nil")
	}
	if draft.Subject != d.Subject || draft.AccountID != d.AccountID {
		t.Errorf("draft mismatch: got %+v, want %+v", draft, d)
	}
}

func TestDrafts_UpdateExisting(t *testing.T) {
	setup(t)
	d := createDraft("D1", "AC1", "Subject")
	if err := SaveDraft(d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	d.Subject = "Updated"
	if err := SaveDraft(d); err != nil {
		t.Fatalf("SaveDraft (update): %v", err)
	}
	all := GetAllDrafts()
	if len(all) != 1 {
		t.Fatalf("expected 1 draft after update, got %d", len(all))
	}
	if all[0].Subject != "Updated" {
		t.Errorf("subject: got %q, want %q", all[0].Subject, "Updated")
	}
}

func TestDrafts_Delete(t *testing.T) {
	setup(t)
	if err := SaveDraft(createDraft("D1", "AC1", "Subject")); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if err := DeleteDraft("D1"); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	if GetDraft("D1") != nil {
		t.Error("deleted draft should return nil")
	}
	if HasDrafts() {
		t.Error("HasDrafts should be false after all drafts deleted")
	}
}

func TestDrafts_LoadCorruptFile(t *testing.T) {
	setup(t)
	path, err := draftsFile()
	if err != nil {
		t.Fatalf("draftsFile: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{invalid json}"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err = LoadDraftsCache(); err == nil {
		t.Error("LoadDraftsCache should error on corrupt JSON")
	}
}

func TestEmailBody_SaveLoadRoundTrip(t *testing.T) {
	setup(t)
	body := createBody(1, "AC1", "Hello, world!")
	if err := SaveEmailBody("INBOX", body, threshold); err != nil {
		t.Fatalf("SaveEmailBody: %v", err)
	}
	got := GetCachedEmailBody("INBOX", 1, "AC1", threshold)
	if got == nil {
		t.Fatal("GetCachedEmailBody returned nil")
	}
	if got.Body != body.Body {
		t.Errorf("body text: got %q, want %q", got.Body, body.Body)
	}
}

func TestEmailBody_FolderIsolation(t *testing.T) {
	setup(t)

	_ = SaveEmailBody("INBOX", createBody(1, "AC1", "inbox body"), threshold)
	_ = SaveEmailBody("Sent", createBody(1, "AC1", "sent body"), threshold)

	gotInbox := GetCachedEmailBody("INBOX", 1, "AC1", threshold)
	gotSent := GetCachedEmailBody("Sent", 1, "AC1", threshold)

	if gotInbox == nil || gotInbox.Body != "inbox body" {
		t.Errorf("INBOX body: got %v", gotInbox)
	}
	if gotSent == nil || gotSent.Body != "sent body" {
		t.Errorf("Sent body: got %v", gotSent)
	}
}

func TestEmailBody_PruneRemovesStaleUIDs(t *testing.T) {
	setup(t)

	_ = SaveEmailBody("INBOX", createBody(1, "AC1", fmt.Sprintf("body %d", 1)), threshold)
	_ = SaveEmailBody("INBOX", createBody(2, "AC1", fmt.Sprintf("body %d", 2)), threshold)
	_ = SaveEmailBody("INBOX", createBody(3, "AC1", fmt.Sprintf("body %d", 3)), threshold)

	if err := PruneEmailBodyCache("INBOX", map[uint32]string{2: "AC1"}, threshold); err != nil {
		t.Fatalf("PruneEmailBodyCache: %v", err)
	}
	if GetCachedEmailBody("INBOX", 1, "AC1", threshold) != nil {
		t.Error("UID 1 should have been pruned")
	}
	if GetCachedEmailBody("INBOX", 3, "AC1", threshold) != nil {
		t.Error("UID 3 should have been pruned")
	}
	if GetCachedEmailBody("INBOX", 2, "AC1", threshold) == nil {
		t.Error("UID 2 should still be cached")
	}
}

func TestEmailBody_CorruptBodyCacheFile(t *testing.T) {
	setup(t)
	_ = SaveEmailBody("INBOX", createBody(1, "AC1", "valid"), threshold)
	path, err := bodyCacheFile("INBOX")
	if err != nil {
		t.Fatalf("bodyCacheFile: %v", err)
	}
	if err := os.WriteFile(path, []byte("{invalid json}"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err = LoadEmailBodyCache("INBOX"); err == nil {
		t.Error("LoadEmailBodyCache should error on corrupt JSON")
	}
}

func TestEmailBodyCache_AttachmentsPreserved(t *testing.T) {
	setup(t)
	body := CachedEmailBody{
		UID:       1,
		AccountID: "AC1",
		Body:      "attachment",
		Attachments: []CachedAttachment{
			{Filename: "invoice.pdf", PartID: "2", MIMEType: "application/pdf"},
			{
				Filename:         "meeting.ics",
				PartID:           "3",
				MIMEType:         "text/calendar",
				IsCalendarInvite: true,
				CalendarData:     []byte("BEGIN:VCALENDAR\nEND:VCALENDAR"),
			},
		},
	}

	body.SizeBytes = calculateEmailBodySize(&body)

	_ = SaveEmailBody("INBOX", body, threshold)

	got := GetCachedEmailBody("INBOX", 1, "AC1", threshold)

	if got == nil {
		t.Fatal("GetCachedEmailBody returned nil")
	}
	if len(got.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(got.Attachments))
	}
	if got.Attachments[0].Filename != "invoice.pdf" {
		t.Errorf("attachment[0].Filename: got %q", got.Attachments[0].Filename)
	}
	if !got.Attachments[1].IsCalendarInvite {
		t.Error("attachment[1].IsCalendarInvite should be true")
	}
	if string(got.Attachments[1].CalendarData) != "BEGIN:VCALENDAR\nEND:VCALENDAR" {
		t.Errorf("calendar data mismatch: %s", got.Attachments[1].CalendarData)
	}
}

func TestLRU_EvictsLeastRecentlyUsed(t *testing.T) {
	setup(t)

	const body = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	for _, uid := range []uint32{1, 2, 3} {
		b := CachedEmailBody{UID: uid, AccountID: "AC1", Body: body, SizeBytes: len(body)}
		if err := SaveEmailBody("INBOX", b, 250); err != nil {
			t.Fatalf("SaveEmailBody uid=%d: %v", uid, err)
		}
	}

	if GetCachedEmailBody("INBOX", 1, "AC1", threshold) != nil {
		t.Error("UID 1 should have been evicted (LRU)")
	}
	if GetCachedEmailBody("INBOX", 2, "AC1", threshold) == nil {
		t.Error("UID 2 should still be cached")
	}
	if GetCachedEmailBody("INBOX", 3, "AC1", threshold) == nil {
		t.Error("UID 3 should still be cached")
	}
}

func TestLRU_OversizedBodyRejected(t *testing.T) {
	setup(t)

	body := CachedEmailBody{
		UID:       99,
		AccountID: "AC1",
		Body:      "This is body",
		SizeBytes: 80,
	}
	_ = SaveEmailBody("INBOX", body, 50)

	if GetCachedEmailBody("INBOX", 99, "acc1", 50) != nil {
		t.Error("oversized body should not be stored in LRU")
	}
}

func TestLRU_GetPromotesToFront(t *testing.T) {
	setup(t)

	const body = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

	for _, uid := range []uint32{1, 2} {
		b := CachedEmailBody{UID: uid, AccountID: "AC1", Body: body, SizeBytes: len(body)}
		_ = SaveEmailBody("INBOX", b, 250)
	}

	GetCachedEmailBody("INBOX", 1, "AC1", 250)

	b := CachedEmailBody{UID: 3, AccountID: "AC1", Body: body, SizeBytes: len(body)}
	_ = SaveEmailBody("INBOX", b, 250)

	if GetCachedEmailBody("INBOX", 2, "AC1", threshold) != nil {
		t.Error("UID 2 should have been evicted (LRU after promotion of UID 1)")
	}
	if GetCachedEmailBody("INBOX", 1, "AC1", threshold) == nil {
		t.Error("UID 1 should still be cached (was promoted)")
	}
}

func TestLRU_DeleteRemovesEntry(t *testing.T) {
	setup(t)
	b := createBody(1, "AC1", "to be deleted")
	_ = SaveEmailBody("INBOX", b, threshold)
	GetLRUInstance(threshold).Delete("INBOX", 1, "AC1")
	if GetCachedEmailBody("INBOX", 1, "AC1", threshold) != nil {
		t.Error("deleted entry should not be retrievable")
	}
}

func TestLRU_ThresholdUpdate(t *testing.T) {
	setup(t)
	lru1 := GetLRUInstance(threshold)
	if lru1.threshold != threshold {
		t.Errorf("threshold: got %d, want %d", lru1.threshold, threshold)
	}
	lru2 := GetLRUInstance(threshold / 2)
	if lru2.threshold != threshold/2 {
		t.Errorf("updated threshold: got %d, want %d", lru2.threshold, threshold/2)
	}
	if lru1 != lru2 {
		t.Error("GetLRUInstance should always return the same pointer")
	}
}

func TestLRU_ConcurrentReadWrite(t *testing.T) {
	setup(t)

	var wg sync.WaitGroup
	wg.Add(20)
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			uid := uint32(i % 5)
			b := CachedEmailBody{
				UID: uid, AccountID: "AC1",
				Body:      fmt.Sprintf("body uid=%d goroutine=%d", uid, i),
				SizeBytes: 40,
			}
			_ = SaveEmailBody("INBOX", b, 1000000)
			_ = GetCachedEmailBody("INBOX", uid, "AC1", 1000000)
		}()
	}
	wg.Wait()
}

func TestEmailBody_EvictsLeastRecentlyAccessedAcrossFolders(t *testing.T) {
	setup(t)

	if err := SaveEmailBody("INBOX", createBody(1, "AC1", "1234567890"), 20); err != nil {
		t.Fatalf("SaveEmailBody: %v", err)
	}
	if err := SaveEmailBody("Archive", createBody(2, "AC1", "1234567890"), 20); err != nil {
		t.Fatalf("SaveEmailBody: %v", err)
	}

	if err := SaveEmailBody("Sent", createBody(3, "AC1", "1234567890"), 20); err != nil {
		t.Fatalf("SaveEmailBody: %v", err)
	}

	if got := GetCachedEmailBody("INBOX", 1, "AC1", 20); got != nil {
		t.Error("oldest INBOX body should be evicted from LRU")
	}

	if got := GetCachedEmailBody("Archive", 2, "AC1", 20); got == nil {
		t.Error("recent Archive body should still be cached")
	}

	if got := GetCachedEmailBody("Sent", 3, "AC1", 20); got == nil {
		t.Error("new Sent body should be cached")
	}
}

func TestEmailBody_EvictsMultipleEntriesUntilUnderLimit(t *testing.T) {
	setup(t)

	for i := uint32(1); i <= 4; i++ {
		if err := SaveEmailBody("INBOX", createBody(i, "AC1", "1234567890"), 50); err != nil {
			t.Fatalf("SaveEmailBody: %v", err)
		}
	}

	if err := SaveEmailBody("Archive", createBody(5, "AC1", "123456789012345678901234567890"), 50); err != nil {
		t.Fatalf("SaveEmailBody: %v", err)
	}

	if got := GetCachedEmailBody("INBOX", 1, "AC1", 50); got != nil {
		t.Error("UID 1 should have been evicted")
	}
	if got := GetCachedEmailBody("INBOX", 2, "AC1", 50); got != nil {
		t.Error("UID 2 should have been evicted")
	}

	if got := GetCachedEmailBody("INBOX", 3, "AC1", 50); got == nil {
		t.Error("UID 3 should still be cached")
	}
	if got := GetCachedEmailBody("INBOX", 4, "AC1", 50); got == nil {
		t.Error("UID 4 should still be cached")
	}

	if got := GetCachedEmailBody("Archive", 5, "AC1", 50); got == nil {
		t.Error("new Archive body should be cached")
	}
}
