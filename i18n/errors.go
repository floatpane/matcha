package i18n

import "errors"

var (
	// ErrLanguageNotFound is returned when a requested language is not available.
	ErrLanguageNotFound = errors.New("language not found")

	// ErrMessageNotFound is returned when a translation key does not exist.
	ErrMessageNotFound = errors.New("message not found")

	// ErrInvalidLocale is returned when a locale code is malformed.
	ErrInvalidLocale = errors.New("invalid locale code")

	// ErrLoadFailed is returned when translation files fail to load.
	ErrLoadFailed = errors.New("failed to load translations")

	// ErrParseFailed is returned when a translation file cannot be parsed.
	ErrParseFailed = errors.New("failed to parse translation file")

	// ErrNoDefaultLanguage is returned when no default language is set.
	ErrNoDefaultLanguage = errors.New("no default language set")
)
