package languages

import (
	"github.com/floatpane/matcha/i18n"
	"golang.org/x/text/language"
)

func init() {
	i18n.RegisterLanguage(&i18n.Locale{
		Tag:        language.German,
		Code:       "de",
		Name:       "German",
		NativeName: "Deutsch",
		Direction:  "ltr",
		PluralFunc: i18n.GermanPlural,
	})
}
