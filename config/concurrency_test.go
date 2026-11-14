package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	resetLRU()
	return dir
}

// The macOS contacts sync, saveContactsAfterSend and the settings screen all
// add contacts concurrently. Before writes were atomic and locked, a reader
// saw a truncated file, treated it as empty, and wiped every contact.
func TestConcurrentContactWritesKeepEverything(t *testing.T) {
	isolateHome(t)

	cache := &ContactsCache{}
	for i := 0; i < 200; i++ {
		cache.Contacts = append(cache.Contacts, Contact{
			Name:  fmt.Sprintf("Person %d", i),
			Email: fmt.Sprintf("p%d@example.com", i),
			Usage: map[string]ContactUsage{"a": {UseCount: 1}},
		})
	}
	if err := SaveContactsCache(cache); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				if err := AddContactForAccount("", fmt.Sprintf("g%d-%d@example.com", g, i), "a"); err != nil {
					t.Errorf("AddContactForAccount: %v", err)
				}
				_ = SearchContacts("example") // concurrent reader
			}
		}(g)
	}
	wg.Wait()

	final, err := LoadContactsCache()
	if err != nil {
		t.Fatalf("contacts file unreadable after concurrent writes: %v", err)
	}
	if got, want := len(final.Contacts), 300; got != want {
		t.Fatalf("contacts = %d, want %d (updates lost)", got, want)
	}
}

// One goroutine per account saves its folder list after every folder fetch
// while the UI toggles per-folder threading.
func TestConcurrentFolderCacheWritesKeepEverything(t *testing.T) {
	isolateHome(t)

	var wg sync.WaitGroup
	const accounts = 6
	for i := 0; i < accounts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("acct-%d", i)
			for r := 0; r < 10; r++ {
				if err := SaveAccountFolders(id, []string{"INBOX", "Sent"}, map[string]int{"INBOX": i}); err != nil {
					t.Errorf("SaveAccountFolders: %v", err)
				}
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := 0; r < 10; r++ {
			if err := SetFolderThreaded("Work", true); err != nil {
				t.Errorf("SetFolderThreaded: %v", err)
			}
		}
	}()
	wg.Wait()

	cache, err := LoadFolderCache()
	if err != nil {
		t.Fatalf("folder cache unreadable: %v", err)
	}
	if len(cache.Accounts) != accounts {
		t.Fatalf("cached accounts = %d, want %d", len(cache.Accounts), accounts)
	}
	if !cache.ThreadedFolders["Work"] {
		t.Fatal("threading override lost")
	}
	for i := 0; i < accounts; i++ {
		folders, _ := GetCachedFolders(fmt.Sprintf("acct-%d", i))
		if len(folders) != 2 {
			t.Fatalf("acct-%d folders = %v", i, folders)
		}
	}
}

// A contacts file that cannot be decoded must be reported, not replaced by a
// cache holding only the new contact.
func TestCorruptContactsFileIsNotOverwritten(t *testing.T) {
	isolateHome(t)
	path, err := GetContactsCachePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte(`{"contacts":[{"name":"Keep Me","email":"keep@example.com"}` + "}}")
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AddContactForAccount("New", "new@example.com", "a"); err == nil {
		t.Fatal("expected an error for an undecodable contacts file")
	}
	if err := SaveDraft(Draft{ID: "d1"}); err != nil {
		t.Fatalf("drafts are a separate file: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(corrupt) {
		t.Fatal("corrupt contacts file was overwritten")
	}
}

// A folder cache corrupted by an older version heals on the next write
// (it is only a cache and is rebuilt from the server).
func TestCorruptFolderCacheHeals(t *testing.T) {
	isolateHome(t)
	path, err := folderCacheFile()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"accounts":[]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveAccountFolders("a", []string{"INBOX"}, nil); err != nil {
		t.Fatalf("SaveAccountFolders on corrupt cache: %v", err)
	}
	if folders, _ := GetCachedFolders("a"); len(folders) != 1 {
		t.Fatalf("folders = %v", folders)
	}
}

func TestAtomicWriteKeepsModeAndSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	if err := os.WriteFile(target, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := writeFileAtomic(link, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced by a regular file (err=%v)", err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "new" {
		t.Fatalf("target = %q", data)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

// Two accounts sharing a UID in the same folder (normal for IMAP) must both
// keep their cached bodies; accounts not covered by the prune are untouched.
func TestPruneEmailBodyCacheByAccount(t *testing.T) {
	isolateHome(t)
	const th = 100 * 1024 * 1024
	for _, acct := range []string{"a1", "a2", "down"} {
		_ = SaveEmailBody("INBOX", CachedEmailBody{UID: 1, AccountID: acct, Body: acct + " body"}, th)
		_ = SaveEmailBody("INBOX", CachedEmailBody{UID: 2, AccountID: acct, Body: acct + " old"}, th)
	}
	err := PruneEmailBodyCacheByAccount("INBOX", map[string]map[uint32]bool{
		"a1": {1: true},
		"a2": {1: true},
	}, th)
	if err != nil {
		t.Fatal(err)
	}
	check := func(acct string, uid uint32, want bool) {
		t.Helper()
		if got := GetCachedEmailBody("INBOX", uid, acct, th) != nil; got != want {
			t.Errorf("%s/%d cached = %v, want %v", acct, uid, got, want)
		}
	}
	check("a1", 1, true)
	check("a2", 1, true)
	check("a1", 2, false)
	check("a2", 2, false)
	check("down", 1, true) // account not covered (e.g. its fetch failed)
	check("down", 2, true)

	// The disk copy agrees after a reload.
	resetLRU()
	check("a1", 2, false)
	check("down", 2, true)
}

// A cache hit runs on the UI goroutine and must not rewrite the folder file.
func TestGetCachedEmailBodyDoesNotWrite(t *testing.T) {
	isolateHome(t)
	const th = 100 * 1024 * 1024
	_ = SaveEmailBody("INBOX", CachedEmailBody{UID: 1, AccountID: "a", Body: "hello"}, th)
	path, err := bodyCacheFile("INBOX")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	for i := 0; i < 5; i++ {
		if GetCachedEmailBody("INBOX", 1, "a", th) == nil {
			t.Fatal("miss")
		}
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("cache hit rewrote the body cache file")
	}

	FlushEmailBodyCache()
	cache, err := LoadEmailBodyCache("INBOX")
	if err != nil || len(cache.Bodies) != 1 {
		t.Fatalf("after flush: %v %+v", err, cache)
	}
	if !cache.Bodies[0].LastAccessedAt.After(cache.Bodies[0].CachedAt) {
		t.Fatal("flush did not persist the access time")
	}
}

func TestConcurrentBodyCacheUse(t *testing.T) {
	isolateHome(t)
	const th = 100 * 1024 * 1024
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				uid := uint32(g*100 + i)
				_ = SaveEmailBody("INBOX", CachedEmailBody{UID: uid, AccountID: "a", Body: "b"}, th)
				_ = GetCachedEmailBody("INBOX", uid, "a", th)
				if i%5 == 0 {
					_ = PruneEmailBodyCacheByAccount("INBOX", map[string]map[uint32]bool{"other": {}}, th)
				}
			}
		}(g)
	}
	wg.Wait()
	cache, err := LoadEmailBodyCache("INBOX")
	if err != nil {
		t.Fatal(err)
	}
	if len(cache.Bodies) != 80 {
		t.Fatalf("bodies on disk = %d, want 80", len(cache.Bodies))
	}
}

func TestAdjustCachedUnread(t *testing.T) {
	isolateHome(t)
	if err := SaveAccountFolders("a", []string{"INBOX"}, map[string]int{"INBOX": 2}); err != nil {
		t.Fatal(err)
	}
	if err := SaveAccountFolders("b", []string{"INBOX", "Work"}, map[string]int{"INBOX": 5}); err != nil {
		t.Fatal(err)
	}
	if err := AdjustCachedUnread("a", "INBOX", -3); err != nil {
		t.Fatal(err)
	}
	_, a := GetCachedFolders("a")
	folders, b := GetCachedFolders("b")
	if a["INBOX"] != 0 || b["INBOX"] != 5 || len(folders) != 2 {
		t.Fatalf("a=%v b=%v folders(b)=%v", a, b, folders)
	}
}
