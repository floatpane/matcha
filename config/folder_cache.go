package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CachedFolders stores folder names for a single account.
type CachedFolders struct {
	AccountID string    `json:"account_id"`
	Folders   []string  `json:"folders"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FolderCache stores cached folders for all accounts.
type FolderCache struct {
	Accounts  []CachedFolders `json:"accounts"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// folderCacheFile returns the full path to the folder cache file.
func folderCacheFile() (string, error) {
	dir, err := configDir()
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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	cache.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadFolderCache loads the folder cache from disk.
func LoadFolderCache() (*FolderCache, error) {
	path, err := folderCacheFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
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
func GetCachedFolders(accountID string) []string {
	cache, err := LoadFolderCache()
	if err != nil {
		return nil
	}
	for _, acc := range cache.Accounts {
		if acc.AccountID == accountID {
			return acc.Folders
		}
	}
	return nil
}

// SaveAccountFolders saves folder names for a specific account, merging into the existing cache.
func SaveAccountFolders(accountID string, folders []string) error {
	cache, err := LoadFolderCache()
	if err != nil {
		cache = &FolderCache{}
	}

	found := false
	for i, acc := range cache.Accounts {
		if acc.AccountID == accountID {
			cache.Accounts[i].Folders = folders
			cache.Accounts[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		cache.Accounts = append(cache.Accounts, CachedFolders{
			AccountID: accountID,
			Folders:   folders,
			UpdatedAt: time.Now(),
		})
	}

	return SaveFolderCache(cache)
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
	dir, err := configDir()
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
	return os.WriteFile(path, data, 0600)
}

// LoadFolderEmailCache loads cached emails for a folder from disk.
func LoadFolderEmailCache(folderName string) ([]CachedEmail, error) {
	path, err := folderEmailCacheFile(folderName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache FolderEmailCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return cache.Emails, nil
}
