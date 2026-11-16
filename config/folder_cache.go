package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	threading "github.com/floatpane/jwz-go"
)

// CachedFolders stores folder names for a single account.
type CachedFolders struct {
	AccountID string         `json:"account_id"`
	Folders   []string       `json:"folders"`
	Unread    map[string]int `json:"unread_counts,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// FolderCache stores cached folders for all accounts.
type FolderCache struct {
	Accounts        []CachedFolders `json:"accounts"`
	ThreadedFolders map[string]bool `json:"threaded_folders,omitempty"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// folderCacheFile returns the full path to the folder cache file.
func folderCacheFile() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "folder_cache.json"), nil
}

// SaveFolderCache saves the folder cache to disk.
func SaveFolderCache(cache *FolderCache) error {
	path, err := folderCacheFile()
	if err != nil {
		return err
	}
	defer lockFile(path)()
	return saveFolderCacheLocked(path, cache)
}

func saveFolderCacheLocked(path string, cache *FolderCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	cache.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

// LoadFolderCache loads the folder cache from disk.
func LoadFolderCache() (*FolderCache, error) {
	path, err := folderCacheFile()
	if err != nil {
		return nil, err
	}
	data, err := SecureReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache FolderCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

// GetCachedFolders returns cached folder names for a specific account.
func GetCachedFolders(accountID string) ([]string, map[string]int) {
	cache, err := LoadFolderCache()
	if err != nil {
		return nil, nil
	}
	for _, acc := range cache.Accounts {
		if acc.AccountID == accountID {
			return acc.Folders, acc.Unread
		}
	}
	return nil, nil
}

// updateFolderCache runs fn on folder_cache.json under the file lock and
// saves the result when fn reports a change. Concurrent callers (one
// goroutine per account after a folder fetch, threading toggles, unread
// adjustments) therefore never lose each other's updates.
func updateFolderCache(fn func(cache *FolderCache) bool) error {
	path, err := folderCacheFile()
	if err != nil {
		return err
	}
	defer lockFile(path)()
	cache := &FolderCache{}
	found, err := loadJSONForUpdate(path, cache, true)
	if err != nil {
		return err
	}
	if !found {
		cache = &FolderCache{}
	}
	if !fn(cache) {
		return nil
	}
	return saveFolderCacheLocked(path, cache)
}

// SaveAccountFolders saves folder names for a specific account, merging into the existing cache.
func SaveAccountFolders(accountID string, folders []string, unread map[string]int) error {
	return updateFolderCache(func(cache *FolderCache) bool {
		saveAccountFoldersLocked(cache, accountID, folders, unread)
		return true
	})
}

// AdjustCachedUnread adds delta to the cached unread count of one folder of
// one account (clamped at zero). Unlike SaveAccountFolders it never touches
// other accounts' entries or the folder list, so it is safe to call with the
// per-account effect of a local action (delete, archive, undo).
func AdjustCachedUnread(accountID, folder string, delta int) error {
	if delta == 0 {
		return nil
	}
	return updateFolderCache(func(cache *FolderCache) bool {
		for i := range cache.Accounts {
			if cache.Accounts[i].AccountID != accountID {
				continue
			}
			if cache.Accounts[i].Unread == nil {
				cache.Accounts[i].Unread = make(map[string]int)
			}
			cache.Accounts[i].Unread[folder] = max(0, cache.Accounts[i].Unread[folder]+delta)
			return true
		}
		return false
	})
}

func saveAccountFoldersLocked(cache *FolderCache, accountID string, folders []string, unread map[string]int) {
	found := false
	for i, acc := range cache.Accounts {
		if acc.AccountID == accountID {
			cache.Accounts[i].Folders = folders
			cache.Accounts[i].Unread = unread
			cache.Accounts[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		cache.Accounts = append(cache.Accounts, CachedFolders{
			AccountID: accountID,
			Folders:   folders,
			Unread:    unread,
			UpdatedAt: time.Now(),
		})
	}
}

func removeAccountFromFolderCache(accountID string) error {
	return updateFolderCache(func(cache *FolderCache) bool {
		filtered := cache.Accounts[:0]
		for _, account := range cache.Accounts {
			if account.AccountID != accountID {
				filtered = append(filtered, account)
			}
		}
		if len(filtered) == len(cache.Accounts) {
			return false
		}
		cache.Accounts = filtered
		return true
	})
}

// --- Per-folder email cache ---

// FolderEmailCache stores cached emails for a specific folder.
type FolderEmailCache struct {
	FolderName string        `json:"folder_name"`
	Emails     []CachedEmail `json:"emails"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

// folderEmailCacheDir returns the directory for folder email cache files.
func folderEmailCacheDir() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "folder_emails"), nil
}

// folderEmailCacheFile returns the file path for a folder's email cache.
// Uses a sanitized folder name to avoid filesystem issues.
func folderEmailCacheFile(folderName string) (string, error) {
	dir, err := folderEmailCacheDir()
	if err != nil {
		return "", err
	}
	// Sanitize folder name for use as filename
	safe := sanitizeFolderName(folderName)
	return filepath.Join(dir, safe+".json"), nil
}

func sanitizeFolderName(name string) string {
	// Replace path separators and other problematic chars
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(name)
}

// SaveFolderEmailCache saves emails for a folder to disk.
func SaveFolderEmailCache(folderName string, emails []CachedEmail) error {
	path, err := folderEmailCacheFile(folderName)
	if err != nil {
		return err
	}
	defer lockFile(path)()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	cache := FolderEmailCache{
		FolderName: folderName,
		Emails:     emails,
		UpdatedAt:  time.Now(),
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

// LoadFolderEmailCache loads cached emails for a folder from disk.
func LoadFolderEmailCache(folderName string) ([]CachedEmail, error) {
	path, err := folderEmailCacheFile(folderName)
	if err != nil {
		return nil, err
	}
	data, err := SecureReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache FolderEmailCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return cache.Emails, nil
}

func removeAccountFromFolderEmailCaches(accountID string) error {
	dir, err := folderEmailCacheDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var errs []error
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue // skip in-flight temp files of atomic writes
		}
		if err := removeAccountFromFolderEmailFile(filepath.Join(dir, entry.Name()), accountID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func removeAccountFromFolderEmailFile(path, accountID string) error {
	defer lockFile(path)()
	var cache FolderEmailCache
	found, err := loadJSONForUpdate(path, &cache, false)
	if err != nil || !found {
		return err
	}

	filtered := cache.Emails[:0]
	for _, email := range cache.Emails {
		if email.AccountID != accountID {
			filtered = append(filtered, email)
		}
	}
	if len(filtered) == len(cache.Emails) {
		return nil
	}
	if len(filtered) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	cache.Emails = filtered
	cache.UpdatedAt = time.Now()
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

func LoadFolderEmailHeaders(folderName string) ([]threading.EmailHeader, error) {
	emails, err := LoadFolderEmailCache(folderName)
	if err != nil {
		return nil, err
	}
	headers := make([]threading.EmailHeader, 0, len(emails))
	for _, email := range emails {
		headers = append(headers, threading.EmailHeader{
			ID:         email.MessageID,
			InReplyTo:  email.InReplyTo,
			References: email.References,
			Subject:    email.Subject,
			Date:       email.Date,
			EmailID:    cachedEmailID(email),
			Sender:     email.From,
		})
	}
	return headers, nil
}

// IsFolderThreaded returns the threading state for a folder. If the user has
// explicitly toggled threading for this folder, that override is returned.
// Otherwise defaultEnabled (from Config.EnableThreaded) is used.
func IsFolderThreaded(folderName string, defaultEnabled bool) bool {
	cache, err := LoadFolderCache()
	if err != nil || cache.ThreadedFolders == nil {
		return defaultEnabled
	}
	v, ok := cache.ThreadedFolders[folderName]
	if !ok {
		return defaultEnabled
	}
	return v
}

// SetFolderThreaded stores an explicit per-folder threading override.
func SetFolderThreaded(folderName string, threaded bool) error {
	return updateFolderCache(func(cache *FolderCache) bool {
		if cache.ThreadedFolders == nil {
			cache.ThreadedFolders = make(map[string]bool)
		}
		cache.ThreadedFolders[folderName] = threaded
		return true
	})
}

func cachedEmailID(email CachedEmail) string {
	return email.AccountID + ":" + formatUID(email.UID)
}

func formatUID(uid uint32) string {
	return strconv.FormatUint(uint64(uid), 10)
}
