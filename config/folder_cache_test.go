package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// folderCacheTestSetup redirects HOME to a per-test temp directory so
// cacheDir() resolves under the temp tree and the cache file does not
// collide with the user's real ~/.cache/matcha state. USERPROFILE is set
// for the same reason on Windows, where os.UserHomeDir() reads it instead
// of HOME.
func folderCacheTestSetup(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	return tempDir
}

func TestSaveLoadFolderCache_RoundTrip(t *testing.T) {
	folderCacheTestSetup(t)

	expected := &FolderCache{
		Accounts: []CachedFolders{
			{AccountID: "acct-1", Folders: []string{"INBOX", "Sent", "Drafts"}},
			{AccountID: "acct-2", Folders: []string{"INBOX", "Archive"}},
		},
	}

	if err := SaveFolderCache(expected); err != nil {
		t.Fatalf("SaveFolderCache: %v", err)
	}

	got, err := LoadFolderCache()
	if err != nil {
		t.Fatalf("LoadFolderCache: %v", err)
	}

	if len(got.Accounts) != len(expected.Accounts) {
		t.Fatalf("accounts: got %d, want %d", len(got.Accounts), len(expected.Accounts))
	}
	for i, acc := range got.Accounts {
		if acc.AccountID != expected.Accounts[i].AccountID {
			t.Errorf("account %d ID: got %q, want %q", i, acc.AccountID, expected.Accounts[i].AccountID)
		}
		if !reflect.DeepEqual(acc.Folders, expected.Accounts[i].Folders) {
			t.Errorf("account %d folders: got %v, want %v", i, acc.Folders, expected.Accounts[i].Folders)
		}
	}

	// SaveFolderCache stamps UpdatedAt at write time; round-trip should preserve a non-zero value.
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set after SaveFolderCache")
	}
}

func TestSaveAccountFolders_InvalidatesExistingEntry(t *testing.T) {
	folderCacheTestSetup(t)

	if err := SaveAccountFolders("acct-1", []string{"INBOX", "Sent"}); err != nil {
		t.Fatalf("first SaveAccountFolders: %v", err)
	}

	// Overwriting the same accountID must replace the folder list, not append.
	if err := SaveAccountFolders("acct-1", []string{"INBOX", "Trash"}); err != nil {
		t.Fatalf("second SaveAccountFolders: %v", err)
	}

	got := GetCachedFolders("acct-1")
	want := []string{"INBOX", "Trash"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after overwrite: got %v, want %v", got, want)
	}

	cache, err := LoadFolderCache()
	if err != nil {
		t.Fatalf("LoadFolderCache: %v", err)
	}
	if len(cache.Accounts) != 1 {
		t.Errorf("accounts: got %d, want 1 (overwrite must not duplicate)", len(cache.Accounts))
	}
}

func TestSaveAccountFolders_AddsNewAccount(t *testing.T) {
	folderCacheTestSetup(t)

	if err := SaveAccountFolders("acct-1", []string{"INBOX"}); err != nil {
		t.Fatalf("SaveAccountFolders acct-1: %v", err)
	}
	if err := SaveAccountFolders("acct-2", []string{"INBOX", "Spam"}); err != nil {
		t.Fatalf("SaveAccountFolders acct-2: %v", err)
	}

	cache, err := LoadFolderCache()
	if err != nil {
		t.Fatalf("LoadFolderCache: %v", err)
	}
	if len(cache.Accounts) != 2 {
		t.Errorf("accounts: got %d, want 2", len(cache.Accounts))
	}
	if got := GetCachedFolders("acct-1"); !reflect.DeepEqual(got, []string{"INBOX"}) {
		t.Errorf("acct-1: got %v", got)
	}
	if got := GetCachedFolders("acct-2"); !reflect.DeepEqual(got, []string{"INBOX", "Spam"}) {
		t.Errorf("acct-2: got %v", got)
	}
}

func TestLoadFolderCache_CorruptFileReturnsError(t *testing.T) {
	folderCacheTestSetup(t)

	path, err := folderCacheFile()
	if err != nil {
		t.Fatalf("folderCacheFile: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not valid json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cache, err := LoadFolderCache()
	if err == nil {
		t.Errorf("LoadFolderCache should fail on corrupt JSON; got cache=%+v", cache)
	}
}

func TestSaveAccountFolders_RecoversFromCorruptCache(t *testing.T) {
	folderCacheTestSetup(t)

	path, err := folderCacheFile()
	if err != nil {
		t.Fatalf("folderCacheFile: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("garbage"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// SaveAccountFolders treats a load error as "start fresh" (see the
	// `cache = &FolderCache{}` fall-back) so a corrupt file must be
	// silently replaced with a valid one, not fail the whole save.
	if err := SaveAccountFolders("acct-1", []string{"INBOX"}); err != nil {
		t.Fatalf("SaveAccountFolders should recover from corrupt cache: %v", err)
	}
	if got := GetCachedFolders("acct-1"); !reflect.DeepEqual(got, []string{"INBOX"}) {
		t.Errorf("after recovery: got %v, want [INBOX]", got)
	}
}

func TestSaveFolderCache_EmptyAccounts(t *testing.T) {
	folderCacheTestSetup(t)

	empty := &FolderCache{Accounts: []CachedFolders{}}
	if err := SaveFolderCache(empty); err != nil {
		t.Fatalf("SaveFolderCache(empty): %v", err)
	}
	got, err := LoadFolderCache()
	if err != nil {
		t.Fatalf("LoadFolderCache: %v", err)
	}
	if len(got.Accounts) != 0 {
		t.Errorf("accounts: got %d, want 0", len(got.Accounts))
	}
}

func TestSaveAccountFolders_EmptyFolderList(t *testing.T) {
	folderCacheTestSetup(t)

	if err := SaveAccountFolders("acct-1", nil); err != nil {
		t.Fatalf("SaveAccountFolders nil folders: %v", err)
	}
	if err := SaveAccountFolders("acct-2", []string{}); err != nil {
		t.Fatalf("SaveAccountFolders empty folders: %v", err)
	}

	if got := GetCachedFolders("acct-1"); len(got) != 0 {
		t.Errorf("acct-1: got %v, want empty", got)
	}
	if got := GetCachedFolders("acct-2"); len(got) != 0 {
		t.Errorf("acct-2: got %v, want empty", got)
	}

	// Both accounts should still be tracked even though their folder
	// lists are empty -- the write itself is meaningful (it records
	// "we asked the server and got nothing").
	cache, err := LoadFolderCache()
	if err != nil {
		t.Fatalf("LoadFolderCache: %v", err)
	}
	if len(cache.Accounts) != 2 {
		t.Errorf("accounts: got %d, want 2", len(cache.Accounts))
	}
}

func TestGetCachedFolders_MissingAccount(t *testing.T) {
	folderCacheTestSetup(t)

	if err := SaveAccountFolders("acct-1", []string{"INBOX"}); err != nil {
		t.Fatalf("SaveAccountFolders: %v", err)
	}
	if got := GetCachedFolders("missing"); got != nil {
		t.Errorf("missing account: got %v, want nil", got)
	}
}

func TestGetCachedFolders_NoCacheFile(t *testing.T) {
	folderCacheTestSetup(t)

	// No cache file has been written yet.
	if got := GetCachedFolders("acct-1"); got != nil {
		t.Errorf("no cache file: got %v, want nil", got)
	}
}
