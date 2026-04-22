package languages

import "github.com/floatpane/matcha/i18n"

// LanguageInfo provides metadata about a language.
type LanguageInfo struct {
	Code       string
	Name       string
	NativeName string
	Direction  string
	PluralFunc i18n.PluralFunc
}

// All available languages are registered via init() functions in their respective files.
