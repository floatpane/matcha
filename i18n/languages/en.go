package languages

import (
	"github.com/floatpane/matcha/i18n"
	"golang.org/x/text/language"
)

func init() {
	i18n.RegisterLanguage(&i18n.Locale{
		Tag:        language.English,
		Code:       "en",
		Name:       "English",
		NativeName: "English",
		Direction:  "ltr",
		PluralFunc: i18n.EnglishPlural,
	})
}
