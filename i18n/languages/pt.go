package languages

import (
	"github.com/floatpane/matcha/i18n"
	"golang.org/x/text/language"
)

func init() {
	i18n.RegisterLanguage(&i18n.Locale{
		Tag:        language.Portuguese,
		Code:       "pt",
		Name:       "Portuguese",
		NativeName: "Português",
		Direction:  "ltr",
		PluralFunc: i18n.PortuguesePlural,
	})
}
