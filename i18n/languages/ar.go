package languages

import (
	"github.com/floatpane/matcha/i18n"
	"golang.org/x/text/language"
)

func init() {
	i18n.RegisterLanguage(&i18n.Locale{
		Tag:        language.Arabic,
		Code:       "ar",
		Name:       "Arabic",
		NativeName: "العربية",
		Direction:  "rtl",
		PluralFunc: i18n.ArabicPlural,
	})
}
