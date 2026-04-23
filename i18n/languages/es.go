package languages

import (
	"github.com/floatpane/matcha/i18n"
	"golang.org/x/text/language"
)

func init() {
	i18n.RegisterLanguage(&i18n.Locale{
		Tag:        language.Spanish,
		Code:       "es",
		Name:       "Spanish",
		NativeName: "Español",
		Direction:  "ltr",
		PluralFunc: i18n.SpanishPlural,
	})
}
