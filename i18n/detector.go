package i18n

import (
	"os"
	"strings"

	"github.com/floatpane/matcha/config"
)

// DetectLanguage determines the language to use based on config and environment.
func DetectLanguage(cfg *config.Config) string {
	// 1. Check config first
	if lang := detectFromConfig(cfg); lang != "" {
		return normalizeLanguageCode(lang)
	}

	// 2. Check environment variables
	if lang := detectFromEnv(); lang != "" {
		return normalizeLanguageCode(lang)
	}

	// 3. Default to English
	return "en"
}

// detectFromConfig gets language from configuration.
func detectFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.GetLanguage()
}

// detectFromEnv gets language from environment variables.
func detectFromEnv() string {
	// Check standard language environment variables
	for _, envVar := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if lang := os.Getenv(envVar); lang != "" {
			return lang
		}
	}
	return ""
}

// normalizeLanguageCode converts various language code formats to a standard form.
// Examples:
//   - "en_US.UTF-8" -> "en"
//   - "en-US" -> "en"
//   - "pt_BR" -> "pt"
func normalizeLanguageCode(code string) string {
	if code == "" {
		return ""
	}

	// Remove encoding (e.g., ".UTF-8")
	if idx := strings.Index(code, "."); idx != -1 {
		code = code[:idx]
	}

	// Replace underscore with hyphen
	code = strings.ReplaceAll(code, "_", "-")

	// Split on hyphen and take base language
	parts := strings.Split(code, "-")
	if len(parts) > 0 {
		base := strings.ToLower(parts[0])

		// Validate it's a known language
		if HasLanguage(base) {
			return base
		}
	}

	return code
}

// isValidLanguage checks if a language code is registered.
func isValidLanguage(code string) bool {
	return HasLanguage(code)
}
