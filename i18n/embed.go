package i18n

import "embed"

// localeFS embeds all translation files from the locales directory.
//
//go:embed locales/*.json
var localeFS embed.FS
