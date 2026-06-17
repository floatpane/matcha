package spellcheck

import (
	"strings"
	"unicode"
)

// IsCheckable returns true when the token looks like a natural-language
// word worth spell-checking. URLs, email-like fragments, numbers, single
// letters, and all-uppercase short tokens (likely acronyms) are skipped.
func IsCheckable(word string) bool {
	runes := []rune(word)
	if len(runes) < 2 {
		return false
	}
	if strings.ContainsAny(word, "@/\\") {
		return false
	}
	hasLetter := false
	hasDigit := false
	allUpper := true
	for _, r := range runes {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
			if !unicode.IsUpper(r) {
				allUpper = false
			}
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter {
		return false
	}
	if hasDigit {
		return false
	}
	if allUpper && len(runes) <= 5 {
		return false
	}
	return true
}

// Token records a word and its byte offsets inside the original text.
type Token struct {
	Word  string
	Start int
	End   int
}

// Tokenize splits s into word tokens. A word is a maximal run of letters
// optionally containing internal apostrophes or hyphens. Leading and
// trailing connector characters are stripped.
func Tokenize(s string) []Token {
	var tokens []Token
	start := -1
	lastLetter := -1
	for i, r := range s {
		switch {
		case unicode.IsLetter(r):
			if start < 0 {
				start = i
			}
			lastLetter = i + utf8RuneLen(r)
		case start >= 0 && (r == '\'' || r == '’' || r == '-'):
			// connector — keep word open
		default:
			if start >= 0 && lastLetter > start {
				tokens = append(tokens, Token{Word: s[start:lastLetter], Start: start, End: lastLetter})
			}
			start = -1
			lastLetter = -1
		}
	}
	if start >= 0 && lastLetter > start {
		tokens = append(tokens, Token{Word: s[start:lastLetter], Start: start, End: lastLetter})
	}
	return tokens
}

func utf8RuneLen(r rune) int {
	switch {
	case r < 0x80:
		return 1
	case r < 0x800:
		return 2
	case r < 0x10000:
		return 3
	default:
		return 4
	}
}
