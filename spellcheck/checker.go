package spellcheck

import (
	"strings"
	"sync"
	"unicode"
)

// Checker holds a loaded word set and reports whether tokens are known.
type Checker struct {
	mu       sync.RWMutex
	words    map[string]struct{}
	runes    map[rune]struct{}
	loaded   bool
	language string
}

// NewChecker returns an empty checker. Load must be called before Check
// returns useful results.
func NewChecker() *Checker {
	return &Checker{words: make(map[string]struct{}), runes: make(map[rune]struct{})}
}

// Load reads a dictionary file from disk and replaces the current word set.
func (c *Checker) Load(path, language string) error {
	w, runes, err := parseHunspellDic(path)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.words = w
	c.runes = runes
	c.loaded = true
	c.language = language
	c.mu.Unlock()
	return nil
}

// LoadLang loads the dictionary for the given language code from the
// configured dicts directory.
func (c *Checker) LoadLang(lang string) error {
	path, err := DictPath(lang)
	if err != nil {
		return err
	}
	return c.Load(path, lang)
}

// Loaded reports whether the checker has a dictionary ready.
func (c *Checker) Loaded() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loaded
}

// Language returns the language code of the loaded dictionary.
func (c *Checker) Language() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.language
}

// Check reports whether the word is recognised. Words shorter than 2 runes,
// numeric, or containing only punctuation are always treated as correct.
// Words that contain letter runes outside the loaded dictionary's
// alphabet (e.g. Cyrillic text against an English dictionary, or accented
// characters not present in the dictionary's base forms) are also treated
// as correct — we have no signal to judge them.
func (c *Checker) Check(word string) bool {
	if !IsCheckable(word) {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.loaded {
		return true
	}
	if !c.coversWord(word) {
		return true
	}
	lower := strings.ToLower(word)
	if _, ok := c.words[lower]; ok {
		return true
	}
	// Strip a trailing apostrophe-suffix ('s, 'd, 'll, 're, 've, 't, 'm)
	// so possessives and common contractions don't get flagged when the
	// dictionary lists only the base form.
	if idx := strings.IndexByte(lower, '\''); idx > 0 {
		base := lower[:idx]
		if _, ok := c.words[base]; ok {
			return true
		}
	}
	return false
}

// coversWord returns true when every letter rune in word is present in
// the loaded dictionary's rune set. Caller must hold c.mu.
func (c *Checker) coversWord(word string) bool {
	if len(c.runes) == 0 {
		return true
	}
	for _, r := range word {
		if !unicode.IsLetter(r) {
			continue
		}
		lr := unicode.ToLower(r)
		if _, ok := c.runes[lr]; !ok {
			return false
		}
	}
	return true
}

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
