package i18n

// This file contains plural rule implementations for different languages.
// Plural rules are based on Unicode CLDR plural rules.
// Reference: https://cldr.unicode.org/index/cldr-spec/plural-rules

// EnglishPlural implements plural rules for English.
// Rule: one (n == 1), other (everything else)
func EnglishPlural(n int) PluralForm {
	if n == 1 {
		return One
	}
	return Other
}

// SpanishPlural implements plural rules for Spanish.
// Rule: one (n == 1), other (everything else)
func SpanishPlural(n int) PluralForm {
	if n == 1 {
		return One
	}
	return Other
}

// GermanPlural implements plural rules for German.
// Rule: one (n == 1), other (everything else)
func GermanPlural(n int) PluralForm {
	if n == 1 {
		return One
	}
	return Other
}

// FrenchPlural implements plural rules for French.
// Rule: one (n == 0 or n == 1), other (everything else)
func FrenchPlural(n int) PluralForm {
	if n == 0 || n == 1 {
		return One
	}
	return Other
}

// PortuguesePlural implements plural rules for Portuguese.
// Rule: one (n == 0 or n == 1), other (everything else)
func PortuguesePlural(n int) PluralForm {
	if n == 0 || n == 1 {
		return One
	}
	return Other
}

// RussianPlural implements plural rules for Russian.
// Rule: one (n mod 10 == 1 and n mod 100 != 11)
//
//	few (n mod 10 in 2..4 and n mod 100 not in 12..14)
//	many (everything else)
func RussianPlural(n int) PluralForm {
	mod10 := n % 10
	mod100 := n % 100

	if mod10 == 1 && mod100 != 11 {
		return One
	}
	if mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) {
		return Few
	}
	return Many
}

// ArabicPlural implements plural rules for Arabic.
// Rule: zero (n == 0)
//
//	one (n == 1)
//	two (n == 2)
//	few (n mod 100 in 3..10)
//	many (n mod 100 in 11..99)
//	other (everything else)
func ArabicPlural(n int) PluralForm {
	if n == 0 {
		return Zero
	}
	if n == 1 {
		return One
	}
	if n == 2 {
		return Two
	}

	mod100 := n % 100
	if mod100 >= 3 && mod100 <= 10 {
		return Few
	}
	if mod100 >= 11 && mod100 <= 99 {
		return Many
	}
	return Other
}

// JapanesePlural implements plural rules for Japanese.
// Rule: other (always - no plural distinction)
func JapanesePlural(n int) PluralForm {
	return Other
}

// ChinesePlural implements plural rules for Chinese.
// Rule: other (always - no plural distinction)
func ChinesePlural(n int) PluralForm {
	return Other
}

// PolishPlural implements plural rules for Polish.
// Rule: one (n == 1)
//
//	few (n mod 10 in 2..4 and n mod 100 not in 12..14)
//	many (everything else)
func PolishPlural(n int) PluralForm {
	if n == 1 {
		return One
	}

	mod10 := n % 10
	mod100 := n % 100

	if mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) {
		return Few
	}
	return Many
}

// CzechPlural implements plural rules for Czech.
// Rule: one (n == 1)
//
//	few (n in 2..4)
//	many (everything else)
func CzechPlural(n int) PluralForm {
	if n == 1 {
		return One
	}
	if n >= 2 && n <= 4 {
		return Few
	}
	return Many
}

// ItalianPlural implements plural rules for Italian.
// Rule: one (n == 1), other (everything else)
func ItalianPlural(n int) PluralForm {
	if n == 1 {
		return One
	}
	return Other
}

// UkrainianPlural implements plural rules for Ukrainian.
// Rule: one (n mod 10 == 1 and n mod 100 != 11)
//
//	few (n mod 10 in 2..4 and n mod 100 not in 12..14)
//	many (everything else)
//
// Same as Russian
func UkrainianPlural(n int) PluralForm {
	mod10 := n % 10
	mod100 := n % 100

	if mod10 == 1 && mod100 != 11 {
		return One
	}
	if mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) {
		return Few
	}
	return Many
}
