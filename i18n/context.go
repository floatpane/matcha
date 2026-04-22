package i18n

import "context"

type contextKey int

const (
	// localeContextKey is the key for storing locale in context.
	localeContextKey contextKey = iota
)

// WithLocale returns a new context with the given locale.
func WithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeContextKey, locale)
}

// LocaleFromContext extracts the locale from context.
// Returns empty string if no locale is set.
func LocaleFromContext(ctx context.Context) string {
	if locale, ok := ctx.Value(localeContextKey).(string); ok {
		return locale
	}
	return ""
}
