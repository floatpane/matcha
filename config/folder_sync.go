package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// FolderSyncState is what an incremental IMAP sync of one account's folder
// remembers between runs. It lives beside the folder's email cache, in its
// own small file, so the many writers of the email list never touch it.
type FolderSyncState struct {
	UIDValidity   uint32 `json:"uid_validity"`
	HighestModSeq uint64 `json:"highest_modseq"`
	// Skipped are UIDs fetched but left out of the list (mail for another
	// address of a shared mailbox), so they are not fetched again.
	Skipped []uint32 `json:"skipped,omitempty"`
	// FullAt is when the folder was last fetched in full.
	FullAt time.Time `json:"full_at"`
}

func folderSyncFile(folderName string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "folder_sync", sanitizeFolderName(folderName)+".json"), nil
}

// loadFolderSyncStates reads a folder's sync states. A state that cannot be
// decoded is dropped (it only costs a full fetch), unless it is encrypted
// and the vault is locked; see loadJSONForUpdate.
func loadFolderSyncStates(path string) (map[string]FolderSyncState, error) {
	states := map[string]FolderSyncState{}
	found, err := loadJSONForUpdate(path, &states, true)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]FolderSyncState{}, nil
	}
	return states, nil
}

// LoadFolderSyncState returns the sync state of one account's folder, or
// false when there is none.
func LoadFolderSyncState(folderName, accountID string) (FolderSyncState, bool) {
	path, err := folderSyncFile(folderName)
	if err != nil {
		return FolderSyncState{}, false
	}
	defer lockFile(path)()
	states, err := loadFolderSyncStates(path)
	if err != nil {
		return FolderSyncState{}, false
	}
	st, ok := states[accountID]
	return st, ok
}

// SaveFolderSyncState stores the sync state of one account's folder.
func SaveFolderSyncState(folderName, accountID string, state FolderSyncState) error {
	path, err := folderSyncFile(folderName)
	if err != nil {
		return err
	}
	defer lockFile(path)()
	states, err := loadFolderSyncStates(path)
	if err != nil {
		return err
	}
	states[accountID] = state
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(states)
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

// ReplaceAccountFolderEmails replaces one account's emails in a folder's
// cache, keeping the other accounts' emails, sorted newest first.
func ReplaceAccountFolderEmails(folderName, accountID string, emails []CachedEmail) error {
	path, err := folderEmailCacheFile(folderName)
	if err != nil {
		return err
	}
	defer lockFile(path)()
	var cache FolderEmailCache
	if _, err := loadJSONForUpdate(path, &cache, true); err != nil {
		return err
	}
	merged := make([]CachedEmail, 0, len(cache.Emails)+len(emails))
	for _, e := range cache.Emails {
		if e.AccountID != accountID {
			merged = append(merged, e)
		}
	}
	merged = append(merged, emails...)
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Date.After(merged[j].Date)
	})
	return saveFolderEmailCacheLocked(path, folderName, merged)
}
