package i18n

import "sync"

var registry = &Registry{
	languages: make(map[string]*Locale),
}

// Registry holds all registered language locales.
type Registry struct {
	languages map[string]*Locale
	mu        sync.RWMutex
}

// RegisterLanguage registers a locale in the global registry.
// This is typically called from init() functions in language files.
func RegisterLanguage(locale *Locale) {
	if locale == nil || locale.Code == "" {
		return
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.languages[locale.Code] = locale
}

// GetLanguage retrieves a registered locale by code.
func GetLanguage(code string) (*Locale, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	locale, ok := registry.languages[code]
	return locale, ok
}

// AvailableLanguages returns all registered locales.
func AvailableLanguages() []*Locale {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	locales := make([]*Locale, 0, len(registry.languages))
	for _, locale := range registry.languages {
		locales = append(locales, locale)
	}
	return locales
}

// LanguageCodes returns all registered language codes.
func LanguageCodes() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	codes := make([]string, 0, len(registry.languages))
	for code := range registry.languages {
		codes = append(codes, code)
	}
	return codes
}

// HasLanguage checks if a language code is registered.
func HasLanguage(code string) bool {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	_, ok := registry.languages[code]
	return ok
}
