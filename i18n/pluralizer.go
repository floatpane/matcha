package i18n

// PluralForm represents the different plural categories.
type PluralForm int

const (
	// Zero is used when count is exactly 0
	Zero PluralForm = iota
	// One is used for singular (typically count == 1)
	One
	// Two is used for dual (count == 2) in some languages
	Two
	// Few is used for small counts in some languages
	Few
	// Many is used for larger counts in some languages
	Many
	// Other is the default/fallback form
	Other
)

// String returns the string representation of the plural form.
func (p PluralForm) String() string {
	switch p {
	case Zero:
		return "zero"
	case One:
		return "one"
	case Two:
		return "two"
	case Few:
		return "few"
	case Many:
		return "many"
	case Other:
		return "other"
	default:
		return "other"
	}
}

// PluralFunc is a function that returns the appropriate plural form for a count.
type PluralFunc func(n int) PluralForm

// Pluralize returns the appropriate text from a message based on count and plural rules.
func (m *Message) Pluralize(count int, pluralFunc PluralFunc) string {
	if pluralFunc == nil {
		pluralFunc = DefaultPlural
	}
	form := pluralFunc(count)
	return m.GetText(form)
}

// DefaultPlural is a simple plural function (English-like: 1 = one, else = other).
func DefaultPlural(n int) PluralForm {
	if n == 1 {
		return One
	}
	return Other
}
