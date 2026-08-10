package tui

import "github.com/floatpane/matcha/i18n"

// i18nT translates a message key to the current language using the i18n
// locale bundle directly, without consulting plugin text overrides. It is the
// fallback used by t() when no override is registered.
func i18nT(key string) string {
	return i18n.GetManager().T(key)
}

// t translates a message key to the current language. It first checks the
// plugin text override registry (populated via matcha.ui.set_text); if a
// plugin has overridden the key, the plugin value wins. Otherwise it falls
// back to the standard i18n locale translation.
//
// Example: t("composer.title") -> "Compose New Email" (or plugin override)
func t(key string) string {
	return overriddenT(key)
}

// tn translates a message with plural support.
// Example: tn("inbox.emails", 5, nil) -> "5 emails"
func tn(key string, count int, data map[string]interface{}) string {
	return i18n.GetManager().Tn(key, count, data)
}

// tpl translates a message and applies template variables.
// Example: tpl("welcome.message", map[string]interface{}{"name": "John"}) -> "Welcome, John!"
func tpl(key string, data map[string]interface{}) string {
	return i18n.GetManager().Tpl(key, data)
}

// tfs formats a file size using the active UI locale.
// Example: tfs(1258291) -> "1.2 MB" in English.
func tfs(bytes int64) string {
	return i18n.GetManager().GetNumberFormatter().FormatFileSize(bytes)
}
