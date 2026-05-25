package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func setup(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetLRU()
}

func TestEmailCache_SaveLoadRoundTrip(t *testing.T) {
	setup(t)

	e1 := CachedEmail{
		UID:     1,
		From:    "t1@e.com",
		To:      []string{"t2@e.com"},
		Subject: "Hello",
	}

	e2 := CachedEmail{
		UID:     2,
		From:    "t2@e.com",
		To:      []string{"t1@e.com"},
		Subject: "Hello",
	}

	input := &EmailCache{Emails: []CachedEmail{e1, e2}}

	if err := SaveEmailCache(input); err != nil {
		t.Fatalf("SaveEmailCache: %v", err)
	}

	output, err := LoadEmailCache()
	if err != nil {
		t.Fatalf("LoadEmailCache: %v", err)
	}

	if len(output.Emails) != len(input.Emails) {
		t.Fatalf("email count: got %d, want %d", len(output.Emails), len(input.Emails))
	}

	for i := range output.Emails {
		IN := input.Emails[i]
		OU := output.Emails[i]
		if IN.UID != OU.UID || IN.From != OU.From || !slices.Equal(IN.To, OU.To) || IN.Subject != OU.Subject {
			t.Errorf("email[%d] mismatch: got %+v, want %+v", i, OU, IN)
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

	e := CachedEmail{
		UID:       1,
		AccountID: "account",
		From:      "t1@e.com",
		To:        []string{"t2@e.com"},
		Subject:   "Hello",
	}

	if err := SaveEmailCache(&EmailCache{Emails: []CachedEmail{e}}); err != nil {
		t.Fatalf("SaveEmailCache: %v", err)
	}

	if err := ClearEmailCache(); err != nil {
		t.Fatalf("ClearEmailCache: %v", err)
	}

	if HasEmailCache() {
		t.Error("HasEmailCache should be false after clear")
	}
}

func TestEmailCache_RemoveAccount(t *testing.T) {
	setup(t)

	e1 := CachedEmail{UID: 1, AccountID: "a1"}
	e2 := CachedEmail{UID: 2, AccountID: "a2"}
	e3 := CachedEmail{UID: 3, AccountID: "a3"}

	if err := SaveEmailCache(&EmailCache{Emails: []CachedEmail{e1, e2, e3}}); err != nil {
		t.Fatalf("SaveEmailCache: %v", err)
	}

	if err := removeAccountFromEmailCache("a2"); err != nil {
		t.Fatalf("removeAccountFromEmailCache: %v", err)
	}

	output, err := LoadEmailCache()
	if err != nil {
		t.Fatalf("LoadEmailCache: %v", err)
	}

	for _, e := range output.Emails {
		if e.AccountID == "a2" {
			t.Errorf("found email belonging to removed account AC2: %+v", e)
		}
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

	if err := os.WriteFile(path, []byte("{corrupted json}"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err = LoadEmailCache(); err == nil {
		t.Error("LoadEmailCache should return an error for corrupt JSON")
	}
}

func TestContacts_SaveLoadRoundTrip(t *testing.T) {
	setup(t)

	c := Contact{
		Name:      "t",
		Email:     "t@e.com",
		Addresses: []string{"address 1, address 2"},
	}

	input := &ContactsCache{Contacts: []Contact{c}}

	if err := SaveContactsCache(input); err != nil {
		t.Fatalf("SaveContactsCache: %v", err)
	}

	output, err := LoadContactsCache()
	if err != nil {
		t.Fatalf("LoadContactsCache: %v", err)
	}

	if len(output.Contacts) != len(input.Contacts) {
		t.Fatalf("contacts count mismatch:\n  got:  %d\n  want: %d", len(output.Contacts), len(input.Contacts))
	}

	for i := range output.Contacts {
		IN := input.Contacts[i]
		OU := output.Contacts[i]
		if IN.Name != OU.Name || IN.Email != OU.Email || !slices.Equal(IN.Addresses, OU.Addresses) {
			t.Errorf("contact[%d] mismatch: got %+v, want %+v", i, OU, IN)
		}
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

	if err := os.WriteFile(path, []byte("{corrupted json}"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err = LoadContactsCache(); err == nil {
		t.Error("LoadContactsCache should error on corrupt JSON")
	}
}

func TestDrafts_SaveLoadRoundTrip(t *testing.T) {
	setup(t)

	d := Draft{
		ID:        "draft 1",
		To:        "d@e.com",
		Subject:   "Hello World",
		AccountID: "a1",
	}

	if err := SaveDraft(d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	output := GetDraft("draft 1")
	if output == nil {
		t.Fatal("GetDraft returned nil")
	}

	if output.ID != d.ID || output.To != d.To || output.Subject != d.Subject || output.AccountID != d.AccountID {
		t.Errorf("draft mismatch: got %+v, want %+v", output, d)
	}
}

func TestDrafts_UpdateExisting(t *testing.T) {
	setup(t)

	d := Draft{
		ID:        "draft 1",
		To:        "d@e.com",
		Subject:   "Hello World",
		AccountID: "a1",
	}

	if err := SaveDraft(d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	d.Subject = "Hello"
	if err := SaveDraft(d); err != nil {
		t.Fatalf("SaveDraft (update): %v", err)
	}

	output := GetAllDrafts()
	if len(output) != 1 {
		t.Fatalf("expected 1 draft after update, got %d", len(output))
	}

	if output[0].Subject != "Hello" {
		t.Errorf("subject: got %q, want %q", output[0].Subject, "Hello")
	}
}

func TestDrafts_Delete(t *testing.T) {
	setup(t)

	d := Draft{
		ID:        "draft 1",
		To:        "d@e.com",
		Subject:   "Hello World",
		AccountID: "a1",
	}

	if err := SaveDraft(d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	if err := DeleteDraft("draft 1"); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}

	if GetDraft("draft 1") != nil {
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

	if err := os.WriteFile(path, []byte("{corrupted json}"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err = LoadDraftsCache(); err == nil {
		t.Error("LoadDraftsCache should error on corrupt JSON")
	}
}

func TestEmailBody_SaveLoadRoundTrip(t *testing.T) {
	setup(t)

	body := CachedEmailBody{
		UID:       1,
		AccountID: "account",
		Body:      "Hello World",
	}

	threshold := 100 * 1024 * 1024

	if err := SaveEmailBody("INBOX", body, threshold); err != nil {
		t.Fatalf("SaveEmailBody: %v", err)
	}

	output := GetCachedEmailBody("INBOX", 1, "account", threshold)
	if output == nil {
		t.Fatal("GetCachedEmailBody returned nil")
	}

	if output.Body != body.Body {
		t.Errorf("body text: got %q, want %q", output.Body, body.Body)
	}
}

func TestEmailBody_FolderIsolation(t *testing.T) {
	setup(t)

	b1 := CachedEmailBody{
		UID:       1,
		AccountID: "account 1",
		Body:      "Hello INBOX",
	}

	b2 := CachedEmailBody{
		UID:       2,
		AccountID: "account 2",
		Body:      "Hello Sent",
	}

	threshold := 100 * 1024 * 1024

	_ = SaveEmailBody("INBOX", b1, threshold)
	_ = SaveEmailBody("Sent", b2, threshold)

	outputInbox := GetCachedEmailBody("INBOX", 1, "account 1", threshold)
	outputSent := GetCachedEmailBody("Sent", 2, "account 2", threshold)

	if outputInbox == nil || outputInbox.Body != "Hello INBOX" {
		t.Errorf("INBOX body: got %v", outputInbox)
	}
	if outputSent == nil || outputSent.Body != "Hello Sent" {
		t.Errorf("Sent body: got %v", outputSent)
	}
}

func TestEmailBody_PruneRemovesStaleUIDs(t *testing.T) {
	setup(t)

	b1 := CachedEmailBody{UID: 1, AccountID: "account 1", Body: "body 1"}
	b2 := CachedEmailBody{UID: 2, AccountID: "account 1", Body: "body 2"}
	b3 := CachedEmailBody{UID: 3, AccountID: "account 1", Body: "body 3"}

	threshold := 100 * 1024 * 1024

	_ = SaveEmailBody("INBOX", b1, threshold)
	_ = SaveEmailBody("INBOX", b2, threshold)
	_ = SaveEmailBody("INBOX", b3, threshold)

	if err := PruneEmailBodyCache("INBOX", map[uint32]string{2: "account 1"}, threshold); err != nil {
		t.Fatalf("PruneEmailBodyCache: %v", err)
	}

	if GetCachedEmailBody("INBOX", 1, "account 1", threshold) != nil {
		t.Error("UID 1 should have been pruned")
	}

	if GetCachedEmailBody("INBOX", 3, "account 1", threshold) != nil {
		t.Error("UID 3 should have been pruned")
	}

	if GetCachedEmailBody("INBOX", 2, "account 1", threshold) == nil {
		t.Error("UID 2 should still be cached")
	}
}

func TestEmailBody_CorruptBodyCacheFile(t *testing.T) {
	setup(t)

	b := CachedEmailBody{UID: 1, AccountID: "account", Body: "Hello World"}

	_ = SaveEmailBody("INBOX", b, 100*1024*1024)

	path, err := bodyCacheFile("INBOX")
	if err != nil {
		t.Fatalf("bodyCacheFile: %v", err)
	}

	if err := os.WriteFile(path, []byte("{corrupted json}"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err = LoadEmailBodyCache("INBOX"); err == nil {
		t.Error("LoadEmailBodyCache should error on corrupt JSON")
	}
}

func TestEmailBodyCache_AttachmentsPreserved(t *testing.T) {
	setup(t)

	a1 := CachedAttachment{
		Filename: "invoice.pdf",
		PartID:   "2",
		MIMEType: "application/pdf",
	}

	a2 := CachedAttachment{
		Filename: "meeting.ics",
		PartID:   "3",
		MIMEType: "text/calendar",
	}

	body := CachedEmailBody{
		UID:         1,
		AccountID:   "account",
		Body:        "attachment",
		Attachments: []CachedAttachment{a1, a2},
	}

	threshold := 100 * 1024 * 1024

	_ = SaveEmailBody("INBOX", body, threshold)

	output := GetCachedEmailBody("INBOX", 1, "account", threshold)
	if output == nil {
		t.Fatal("GetCachedEmailBody returned nil")
	}

	if len(output.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(output.Attachments))
	}

	if output.Attachments[0].Filename != "invoice.pdf" {
		t.Errorf("attachment[0].Filename: got %q", output.Attachments[0].Filename)
	}

	if output.Attachments[1].Filename != "meeting.ics" {
		t.Errorf("attachment[1].Filename: got %q", output.Attachments[1].Filename)
	}
}

func TestLRU_EvictsLeastRecentlyUsed(t *testing.T) {
	setup(t)

	body := strings.Repeat("a", 100)

	threshold := 250

	b1 := CachedEmailBody{UID: 1, AccountID: "account", Body: body, SizeBytes: len(body)}
	b2 := CachedEmailBody{UID: 2, AccountID: "account", Body: body, SizeBytes: len(body)}
	b3 := CachedEmailBody{UID: 3, AccountID: "account", Body: body, SizeBytes: len(body)}

	_ = SaveEmailBody("INBOX", b1, threshold)
	_ = SaveEmailBody("INBOX", b2, threshold)
	_ = SaveEmailBody("INBOX", b3, threshold)

	if GetCachedEmailBody("INBOX", 1, "account", threshold) != nil {
		t.Error("UID 1 should have been evicted (LRU)")
	}

	if GetCachedEmailBody("INBOX", 2, "account", threshold) == nil {
		t.Error("UID 2 should still be cached")
	}

	if GetCachedEmailBody("INBOX", 3, "account", threshold) == nil {
		t.Error("UID 3 should still be cached")
	}
}

func TestLRU_OversizedBodyRejected(t *testing.T) {
	setup(t)

	body := CachedEmailBody{
		UID:       1,
		AccountID: "account",
		Body:      strings.Repeat("a", 100),
	}

	_ = SaveEmailBody("INBOX", body, 50)

	if GetCachedEmailBody("INBOX", 1, "account", 50) != nil {
		t.Error("oversized body should not be stored in LRU")
	}
}

func TestLRU_GetPromotesToFront(t *testing.T) {
	setup(t)

	b1 := CachedEmailBody{UID: 1, AccountID: "account", Body: strings.Repeat("a", 50)}
	b2 := CachedEmailBody{UID: 2, AccountID: "account", Body: strings.Repeat("a", 50)}

	threshold := 100

	_ = SaveEmailBody("INBOX", b1, threshold)
	_ = SaveEmailBody("INBOX", b2, threshold)

	GetCachedEmailBody("INBOX", 1, "account", threshold)

	b3 := CachedEmailBody{UID: 3, AccountID: "account", Body: strings.Repeat("a", 50)}
	_ = SaveEmailBody("INBOX", b3, threshold)

	if GetCachedEmailBody("INBOX", 2, "account", threshold) != nil {
		t.Error("UID 2 should have been evicted (LRU after promotion of UID 1)")
	}

	if GetCachedEmailBody("INBOX", 1, "account", threshold) == nil {
		t.Error("UID 1 should still be cached (was promoted)")
	}
}

func TestLRU_DeleteRemovesEntry(t *testing.T) {
	setup(t)

	b := CachedEmailBody{UID: 1, AccountID: "account", Body: strings.Repeat("a", 50)}

	threshold := 100

	_ = SaveEmailBody("INBOX", b, threshold)

	GetLRUInstance(threshold).Delete("INBOX", 1, "account")

	if GetCachedEmailBody("INBOX", 1, "account", threshold) != nil {
		t.Error("deleted entry should not be retrievable")
	}
}

func TestLRU_ThresholdUpdate(t *testing.T) {
	setup(t)

	lru1 := GetLRUInstance(100)
	if lru1.threshold != 100 {
		t.Errorf("threshold: got %d, want %d", lru1.threshold, 100)
	}

	lru2 := GetLRUInstance(50)
	if lru2.threshold != 50 {
		t.Errorf("updated threshold: got %d, want %d", lru2.threshold, 50)
	}

	if lru1 != lru2 {
		t.Error("GetLRUInstance should always return the same pointer")
	}
}

func TestEmailBody_EvictsLeastRecentlyAccessedAcrossFolders(t *testing.T) {
	setup(t)

	b1 := CachedEmailBody{UID: 1, AccountID: "account", Body: strings.Repeat("a", 50)}
	b2 := CachedEmailBody{UID: 2, AccountID: "account", Body: strings.Repeat("a", 50)}
	b3 := CachedEmailBody{UID: 3, AccountID: "account", Body: strings.Repeat("a", 50)}

	_ = SaveEmailBody("INBOX", b1, 100)
	_ = SaveEmailBody("Sent", b2, 100)
	_ = SaveEmailBody("Trash", b3, 100)

	if got := GetCachedEmailBody("INBOX", 1, "account", 100); got != nil {
		t.Error("oldest INBOX body should be evicted from LRU")
	}

	if got := GetCachedEmailBody("Sent", 2, "account", 100); got == nil {
		t.Error("recent Archive body should still be cached")
	}

	if got := GetCachedEmailBody("Trash", 3, "account", 100); got == nil {
		t.Error("new Sent body should be cached")
	}
}

func TestEmailBody_EvictsMultipleEntriesUntilUnderLimit(t *testing.T) {
	setup(t)

	b1 := CachedEmailBody{UID: 1, AccountID: "account", Body: strings.Repeat("a", 50)}
	b2 := CachedEmailBody{UID: 2, AccountID: "account", Body: strings.Repeat("a", 50)}
	b3 := CachedEmailBody{UID: 3, AccountID: "account", Body: strings.Repeat("a", 50)}
	b4 := CachedEmailBody{UID: 4, AccountID: "account", Body: strings.Repeat("a", 150)}

	_ = SaveEmailBody("INBOX", b1, 150)
	_ = SaveEmailBody("INBOX", b2, 150)
	_ = SaveEmailBody("INBOX", b3, 150)
	_ = SaveEmailBody("INBOX", b4, 150)

	if got := GetCachedEmailBody("INBOX", 1, "account", 150); got != nil {
		t.Error("UID 1 should have been evicted")
	}

	if got := GetCachedEmailBody("INBOX", 2, "account", 150); got != nil {
		t.Error("UID 2 should have been evicted")
	}

	if got := GetCachedEmailBody("INBOX", 3, "account", 150); got != nil {
		t.Error("UID 3 should have been evicted")
	}

	if got := GetCachedEmailBody("INBOX", 4, "account", 150); got == nil {
		t.Error("new Archive body should be cached")
	}
}

func TestLRU_ConcurrentReadWrite(t *testing.T) {
	setup(t)

	var wg sync.WaitGroup
	wg.Add(20)

	for i := range 20 {
		go func(i int) {
			defer wg.Done()
			uid := uint32(i % 5)
			b := CachedEmailBody{
				UID:       uid,
				AccountID: "account",
				Body:      "Hello World",
			}

			_ = SaveEmailBody("INBOX", b, 1000000)
			_ = GetCachedEmailBody("INBOX", uid, "account", 1000000)
		}(i)
	}
	wg.Wait()
}
