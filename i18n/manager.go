package i18n

import (
	"fmt"
	"sync"
)

var (
	globalManager *Manager
	managerOnce   sync.Once
)

// Manager is the global translation manager.
type Manager struct {
	bundle      *Bundle
	currentLang string
	localizers  map[string]*Localizer
	cache       *Cache
	mu          sync.RWMutex
}

// Init initializes the global translation manager with a default language.
func Init(defaultLang string) error {
	var initErr error

	managerOnce.Do(func() {
		bundle := NewBundle(defaultLang)

		// Load all embedded translations
		if err := LoadTranslations(bundle); err != nil {
			initErr = err
			return
		}

		// Register locales from registry into bundle
		for _, locale := range AvailableLanguages() {
			bundle.RegisterLocale(locale)
		}

		globalManager = &Manager{
			bundle:      bundle,
			currentLang: defaultLang,
			localizers:  make(map[string]*Localizer),
			cache:       NewCache(),
		}

		// Create default localizer
		globalManager.localizers[defaultLang] = NewLocalizer(defaultLang, bundle)
	})

	return initErr
}

// GetManager returns the global manager instance.
func GetManager() *Manager {
	if globalManager == nil {
		// Auto-initialize with English if not yet initialized
		_ = Init("en")
	}
	return globalManager
}

// SetLanguage changes the current language.
func (m *Manager) SetLanguage(lang string) error {
	if lang == "" {
		return ErrInvalidLocale
	}

	lang = normalizeLanguageCode(lang)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if language is available
	if !m.bundle.HasLanguage(lang) {
		return fmt.Errorf("%w: %s", ErrLanguageNotFound, lang)
	}

	// Create localizer if not exists
	if _, ok := m.localizers[lang]; !ok {
		m.localizers[lang] = NewLocalizer(lang, m.bundle)
	}

	m.currentLang = lang
	m.cache.Clear() // Clear cache when switching languages

	return nil
}

// GetLanguage returns the current language code.
func (m *Manager) GetLanguage() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.currentLang
}

// T translates a message key using the current language.
func (m *Manager) T(key string) string {
	m.mu.RLock()
	localizer := m.localizers[m.currentLang]
	m.mu.RUnlock()

	if localizer == nil {
		return key
	}

	return localizer.Localize(key)
}

// Tn translates a message with plural support.
func (m *Manager) Tn(key string, count int, data map[string]interface{}) string {
	m.mu.RLock()
	localizer := m.localizers[m.currentLang]
	m.mu.RUnlock()

	if localizer == nil {
		return key
	}

	// Ensure count is in data
	if data == nil {
		data = make(map[string]interface{})
	}
	if _, ok := data["count"]; !ok {
		data["count"] = count
	}

	return localizer.LocalizePlural(key, count, data)
}

// Tpl translates a message and applies template variables.
func (m *Manager) Tpl(key string, data map[string]interface{}) string {
	m.mu.RLock()
	localizer := m.localizers[m.currentLang]
	m.mu.RUnlock()

	if localizer == nil {
		return key
	}

	return localizer.LocalizeTemplate(key, data)
}

// AvailableLanguages returns all loaded languages.
func (m *Manager) AvailableLanguages() []string {
	return m.bundle.AvailableLanguages()
}

// GetLocale returns the current locale.
func (m *Manager) GetLocale() *Locale {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if localizer, ok := m.localizers[m.currentLang]; ok {
		return localizer.Locale()
	}

	locale, _ := ParseLocale(m.currentLang)
	return locale
}

// ClearCache clears all translation caches.
func (m *Manager) ClearCache() {
	m.cache.Clear()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, localizer := range m.localizers {
		localizer.ClearCache()
	}
}
