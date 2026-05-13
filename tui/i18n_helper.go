package tui

import "github.com/floatpane/matcha/i18n"

// t translates a message key to the current language.
// Example: t("composer.title") -> "Compose New Email"
func t(key string) string {
	return i18n.GetManager().T(key)
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
