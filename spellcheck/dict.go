// Package spellcheck provides dictionary-backed spell checking for the composer.
//
// Dictionaries follow the Hunspell .dic format (word list, optional /flags
// per line). Affix rules are ignored: each base form is added to a flat
// word set. Dictionaries are downloaded from the wooorm/dictionaries
// GitHub repository on demand.
package spellcheck

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
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

// parseHunspellDic reads a Hunspell .dic file and returns the set of base
// words plus the set of letter runes that appear in those words. The
// first line (when numeric) is treated as a count and skipped. Each entry
// may carry "/FLAGS" affix metadata which we strip — we don't expand
// affix rules, so the checker recognises base forms only.
func parseHunspellDic(path string) (map[string]struct{}, map[rune]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open dict: %w", err)
	}
	defer f.Close() //nolint:errcheck

	words := make(map[string]struct{}, 50000)
	runes := make(map[rune]struct{}, 64)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if first {
			first = false
			if _, err := fmt.Sscanf(line, "%d", new(int)); err == nil && !strings.ContainsAny(line, " \t") {
				continue
			}
		}
		if idx := strings.IndexByte(line, '/'); idx >= 0 {
			line = line[:idx]
		}
		if idx := strings.IndexByte(line, '\t'); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		words[lower] = struct{}{}
		for _, r := range lower {
			if isDictLetter(r) {
				runes[r] = struct{}{}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan dict: %w", err)
	}
	return words, runes, nil
}

func isDictLetter(r rune) bool {
	if r < 0x80 {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.IsLetter(r)
}
