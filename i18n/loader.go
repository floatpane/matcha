package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadTranslations loads all translation files into a bundle.
// First attempts to load from embedded files, then checks for external files.
func LoadTranslations(bundle *Bundle) error {
	// Load from embedded files
	if err := loadFromEmbedded(bundle); err != nil {
		return fmt.Errorf("%w: embedded load failed: %v", ErrLoadFailed, err)
	}

	return nil
}

// loadFromEmbedded loads translation files from the embedded filesystem.
func loadFromEmbedded(bundle *Bundle) error {
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !strings.HasSuffix(filename, ".json") {
			continue
		}

		// Read file
		data, err := localeFS.ReadFile(filepath.Join("locales", filename))
		if err != nil {
			continue
		}

		// Extract language code from filename (e.g., "en.json" -> "en")
		lang := strings.TrimSuffix(filename, ".json")

		// Load into bundle
		if err := loadLanguageFile(bundle, lang, data); err != nil {
			return err
		}
	}

	return nil
}

// LoadFromDirectory loads translation files from a directory on disk.
// This allows overriding embedded translations with external files.
func LoadFromDirectory(bundle *Bundle, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrLoadFailed, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !strings.HasSuffix(filename, ".json") {
			continue
		}

		// Read file
		path := filepath.Join(dir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Extract language code
		lang := strings.TrimSuffix(filename, ".json")

		// Load into bundle
		if err := loadLanguageFile(bundle, lang, data); err != nil {
			return err
		}
	}

	return nil
}

// loadLanguageFile parses and loads a single language file into the bundle.
func loadLanguageFile(bundle *Bundle, lang string, data []byte) error {
	messages, err := ParseJSON(data)
	if err != nil {
		return fmt.Errorf("%w: language %s: %v", ErrParseFailed, lang, err)
	}

	return bundle.AddMessages(lang, messages)
}
