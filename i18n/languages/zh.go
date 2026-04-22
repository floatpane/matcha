package languages

import (
	"github.com/floatpane/matcha/i18n"
	"golang.org/x/text/language"
)

func init() {
	i18n.RegisterLanguage(&i18n.Locale{
		Tag:        language.Chinese,
		Code:       "zh",
		Name:       "Chinese",
		NativeName: "中文",
		Direction:  "ltr",
		PluralFunc: i18n.ChinesePlural,
	})
}
