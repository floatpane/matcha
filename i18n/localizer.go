package i18n

// Localizer handles translation lookups for a specific language.
type Localizer struct {
	lang     string
	bundle   *Bundle
	locale   *Locale
	cache    *Cache
	fallback *FallbackChain
}

// NewLocalizer creates a new Localizer for a language.
func NewLocalizer(lang string, bundle *Bundle) *Localizer {
	locale, _ := bundle.GetLocale(lang)
	if locale == nil {
		// Fallback to parsing locale
		locale, _ = ParseLocale(lang)
	}

	return &Localizer{
		lang:     lang,
		bundle:   bundle,
		locale:   locale,
		cache:    NewCache(),
		fallback: NewFallbackChain(lang, bundle.DefaultLanguage()),
	}
}

// Localize translates a message ID to text.
func (l *Localizer) Localize(messageID string) string {
	// Check cache first
	if cached, ok := l.cache.Get(messageID); ok {
		return cached
	}

	// Try fallback chain
	msg, _, err := l.fallback.Resolve(l.bundle, messageID)
	if err != nil {
		// Return the key itself if translation not found
		return messageID
	}

	text := msg.GetDefault()
	l.cache.Set(messageID, text)
	return text
}

// LocalizePlural translates a message with plural support.
func (l *Localizer) LocalizePlural(messageID string, count int, data map[string]interface{}) string {
	// Try fallback chain
	msg, _, err := l.fallback.Resolve(l.bundle, messageID)
	if err != nil {
		return messageID
	}

	// Get plural function
	pluralFunc := l.locale.PluralFunc
	if pluralFunc == nil {
		pluralFunc = DefaultPlural
	}

	// Get appropriate plural form
	text := msg.Pluralize(count, pluralFunc)

	// Interpolate variables
	if data != nil {
		text = Interpolate(text, data)
	}

	return text
}

// LocalizeTemplate translates a message and applies template data.
func (l *Localizer) LocalizeTemplate(messageID string, data map[string]interface{}) string {
	// Try fallback chain
	msg, _, err := l.fallback.Resolve(l.bundle, messageID)
	if err != nil {
		return messageID
	}

	text := msg.GetDefault()

	// Interpolate variables
	if data != nil {
		text = Interpolate(text, data)
	}

	return text
}

// Language returns the localizer's language code.
func (l *Localizer) Language() string {
	return l.lang
}

// Locale returns the localizer's locale.
func (l *Localizer) Locale() *Locale {
	return l.locale
}

// ClearCache clears the localizer's cache.
func (l *Localizer) ClearCache() {
	l.cache.Clear()
}
