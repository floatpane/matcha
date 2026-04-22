package i18n

import (
	"strings"

	"golang.org/x/text/language"
)

// Locale represents a language/region configuration.
type Locale struct {
	// Tag is the BCP 47 language tag
	Tag language.Tag

	// Code is the short language code (e.g., "en", "es", "de")
	Code string

	// Name is the English name of the language
	Name string

	// NativeName is the language's name in its own language
	NativeName string

	// Direction is the text direction ("ltr" or "rtl")
	Direction string

	// PluralFunc is the plural rule function for this language
	PluralFunc PluralFunc
}

// ParseLocale parses a language code and returns a Locale.
// Supports formats like "en", "en-US", "en_US".
func ParseLocale(code string) (*Locale, error) {
	if code == "" {
		return nil, ErrInvalidLocale
	}

	// Normalize separators
	code = strings.ReplaceAll(code, "_", "-")

	// Parse language tag
	tag, err := language.Parse(code)
	if err != nil {
		return nil, ErrInvalidLocale
	}

	// Extract base language
	base, _ := tag.Base()
	langCode := base.String()

	// Look up in registry
	if locale, ok := GetLanguage(langCode); ok {
		return locale, nil
	}

	// Return a basic locale if not registered
	return &Locale{
		Tag:        tag,
		Code:       langCode,
		Name:       langCode,
		NativeName: langCode,
		Direction:  "ltr",
		PluralFunc: DefaultPlural,
	}, nil
}

// String returns the string representation of the locale.
func (l *Locale) String() string {
	return l.Code
}

// IsRTL returns true if the locale uses right-to-left text direction.
func (l *Locale) IsRTL() bool {
	return l.Direction == "rtl"
}
