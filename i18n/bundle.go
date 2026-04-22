package i18n

import (
	"fmt"
	"sync"
)

// Bundle holds all translation messages for all languages.
type Bundle struct {
	defaultLang string
	messages    map[string]MessageMap // lang -> MessageMap
	locales     map[string]*Locale    // lang -> Locale
	mu          sync.RWMutex
}

// NewBundle creates a new Bundle with a default language.
func NewBundle(defaultLang string) *Bundle {
	return &Bundle{
		defaultLang: defaultLang,
		messages:    make(map[string]MessageMap),
		locales:     make(map[string]*Locale),
	}
}

// AddMessages adds translation messages for a language.
func (b *Bundle) AddMessages(lang string, messages MessageMap) error {
	if lang == "" {
		return ErrInvalidLocale
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.messages[lang] == nil {
		b.messages[lang] = make(MessageMap)
	}

	// Merge messages
	for id, msg := range messages {
		b.messages[lang][id] = msg
	}

	return nil
}

// GetMessage retrieves a message for a specific language and ID.
func (b *Bundle) GetMessage(lang, id string) (*Message, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	langMessages, ok := b.messages[lang]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrLanguageNotFound, lang)
	}

	msg, ok := langMessages[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrMessageNotFound, id)
	}

	return msg, nil
}

// RegisterLocale registers a locale configuration.
func (b *Bundle) RegisterLocale(locale *Locale) {
	if locale == nil || locale.Code == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.locales[locale.Code] = locale
}

// GetLocale retrieves a registered locale.
func (b *Bundle) GetLocale(lang string) (*Locale, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	locale, ok := b.locales[lang]
	return locale, ok
}

// AvailableLanguages returns a list of all languages with loaded messages.
func (b *Bundle) AvailableLanguages() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	langs := make([]string, 0, len(b.messages))
	for lang := range b.messages {
		langs = append(langs, lang)
	}
	return langs
}

// DefaultLanguage returns the default language code.
func (b *Bundle) DefaultLanguage() string {
	return b.defaultLang
}

// MessageCount returns the number of messages for a language.
func (b *Bundle) MessageCount(lang string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if messages, ok := b.messages[lang]; ok {
		return len(messages)
	}
	return 0
}

// HasLanguage checks if a language has been loaded.
func (b *Bundle) HasLanguage(lang string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	_, ok := b.messages[lang]
	return ok
}
