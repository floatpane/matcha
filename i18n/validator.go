package i18n

import (
	"fmt"
	"sort"
)

// ValidationResult contains the results of validating translation files.
type ValidationResult struct {
	Valid   bool
	Errors  []ValidationError
	Missing map[string][]string // lang -> missing keys
	Extra   map[string][]string // lang -> extra keys
}

// ValidationError represents a validation issue.
type ValidationError struct {
	Language string
	Key      string
	Message  string
}

// ValidateTranslations validates all translations against a base language.
// Checks for missing keys, extra keys, and consistency.
func ValidateTranslations(bundle *Bundle, baseLang string) *ValidationResult {
	result := &ValidationResult{
		Valid:   true,
		Errors:  []ValidationError{},
		Missing: make(map[string][]string),
		Extra:   make(map[string][]string),
	}

	// Get base language messages
	baseMessages, err := getMessages(bundle, baseLang)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Language: baseLang,
			Message:  fmt.Sprintf("Failed to load base language: %v", err),
		})
		return result
	}

	// Get all available languages
	languages := bundle.AvailableLanguages()

	// Validate each language against base
	for _, lang := range languages {
		if lang == baseLang {
			continue
		}

		langMessages, err := getMessages(bundle, lang)
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Language: lang,
				Message:  fmt.Sprintf("Failed to load language: %v", err),
			})
			continue
		}

		// Find missing and extra keys
		missing, extra := compareKeys(baseMessages, langMessages)

		if len(missing) > 0 {
			result.Valid = false
			result.Missing[lang] = missing
		}

		if len(extra) > 0 {
			result.Extra[lang] = extra
		}
	}

	return result
}

// getMessages retrieves all message keys for a language.
func getMessages(bundle *Bundle, lang string) (MessageMap, error) {
	bundle.mu.RLock()
	defer bundle.mu.RUnlock()

	messages, ok := bundle.messages[lang]
	if !ok {
		return nil, fmt.Errorf("language not found: %s", lang)
	}

	return messages, nil
}

// compareKeys compares two message maps and returns missing and extra keys.
func compareKeys(base, target MessageMap) (missing, extra []string) {
	// Find missing keys (in base but not in target)
	for key := range base {
		if _, ok := target[key]; !ok {
			missing = append(missing, key)
		}
	}

	// Find extra keys (in target but not in base)
	for key := range target {
		if _, ok := base[key]; !ok {
			extra = append(extra, key)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)

	return missing, extra
}

// String returns a human-readable validation report.
func (v *ValidationResult) String() string {
	if v.Valid {
		return "✓ All translations are valid"
	}

	var report string

	// Report errors
	if len(v.Errors) > 0 {
		report += "Errors:\n"
		for _, err := range v.Errors {
			if err.Key != "" {
				report += fmt.Sprintf("  [%s] %s: %s\n", err.Language, err.Key, err.Message)
			} else {
				report += fmt.Sprintf("  [%s] %s\n", err.Language, err.Message)
			}
		}
		report += "\n"
	}

	// Report missing keys
	if len(v.Missing) > 0 {
		report += "Missing translations:\n"
		for lang, keys := range v.Missing {
			report += fmt.Sprintf("  [%s] %d missing keys:\n", lang, len(keys))
			for _, key := range keys {
				report += fmt.Sprintf("    - %s\n", key)
			}
		}
		report += "\n"
	}

	// Report extra keys
	if len(v.Extra) > 0 {
		report += "Extra translations (not in base):\n"
		for lang, keys := range v.Extra {
			report += fmt.Sprintf("  [%s] %d extra keys:\n", lang, len(keys))
			for _, key := range keys {
				report += fmt.Sprintf("    - %s\n", key)
			}
		}
	}

	return report
}
