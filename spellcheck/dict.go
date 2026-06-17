// Package spellcheck provides dictionary-backed spell checking for the composer.
//
// Dictionaries follow the Hunspell .dic format (word list, optional /flags
// per line). Affix rules are ignored: each base form is added to a flat
// word set. Dictionaries are downloaded from the wooorm/dictionaries
// GitHub repository on demand.
package spellcheck

import (
	"fmt"
	"os"
	"path/filepath"
)

// DictsDir returns the directory where dictionaries are stored.
func DictsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "matcha", "dicts")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("cannot create dicts directory: %w", err)
	}
	return dir, nil
}

// DictPath returns the on-disk path for a given language code.
func DictPath(lang string) (string, error) {
	dir, err := DictsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, lang+".dic"), nil
}

// DictInstalled reports whether the dictionary for lang exists on disk.
func DictInstalled(lang string) bool {
	path, err := DictPath(lang)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}
