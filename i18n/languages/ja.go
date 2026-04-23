package languages

import (
	"github.com/floatpane/matcha/i18n"
	"golang.org/x/text/language"
)

func init() {
	i18n.RegisterLanguage(&i18n.Locale{
		Tag:        language.Japanese,
		Code:       "ja",
		Name:       "Japanese",
		NativeName: "日本語",
		Direction:  "ltr",
		PluralFunc: i18n.JapanesePlural,
	})
}
