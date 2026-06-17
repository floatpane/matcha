//go:build !cgo

package spellcheck

import (
	"bufio"
	"fmt"
	"os"
	"sort"
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
// Words that contain letter runes outside the loaded dictionary's alphabet
// are also treated as correct — we have no signal to judge them.
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
	if idx := strings.IndexByte(lower, '\''); idx > 0 {
		if _, ok := c.words[lower[:idx]]; ok {
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

// Suggest returns up to limit candidate corrections for word, ranked by
// edit distance ascending then alphabetically.
func (c *Checker) Suggest(word string, limit int) []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.loaded || len(c.words) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}

	lower := strings.ToLower(word)
	wRunes := []rune(lower)
	if len(wRunes) < 2 {
		return nil
	}

	maxDist := 2
	if len(wRunes) >= 8 {
		maxDist = 3
	}

	type cand struct {
		word string
		dist int
	}
	var cands []cand

	for w := range c.words {
		ld := len(w) - len(lower)
		if ld < 0 {
			ld = -ld
		}
		if ld > maxDist {
			continue
		}
		if !firstRuneClose(w, lower) {
			continue
		}
		d := levenshtein(wRunes, []rune(w), maxDist)
		if d > maxDist {
			continue
		}
		cands = append(cands, cand{w, d})
	}

	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		return cands[i].word < cands[j].word
	})

	if len(cands) > limit {
		cands = cands[:limit]
	}
	out := make([]string, len(cands))
	upper := unicode.IsUpper([]rune(word)[0])
	for i, c := range cands {
		out[i] = matchCase(c.word, upper)
	}
	return out
}

// ── Dictionary parser ─────────────────────────────────────────────────────────

func parseHunspellDic(path string) (map[string]struct{}, map[rune]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open dict: %w", err)
	}
	defer f.Close() //nolint:errcheck

	words := make(map[string]struct{}, 50000)
	runes := make(map[rune]struct{}, 64)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if first {
			first = false
			if _, err := fmt.Sscanf(line, "%d", new(int)); err == nil && !strings.ContainsAny(line, " \t") {
				continue
			}
		}
		if idx := strings.IndexByte(line, '/'); idx >= 0 {
			line = line[:idx]
		}
		if idx := strings.IndexByte(line, '\t'); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		words[lower] = struct{}{}
		for _, r := range lower {
			if isDictLetter(r) {
				runes[r] = struct{}{}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan dict: %w", err)
	}
	return words, runes, nil
}

func isDictLetter(r rune) bool {
	if r < 0x80 {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.IsLetter(r)
}

// ── Suggest helpers ───────────────────────────────────────────────────────────

func firstRuneClose(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	var ar, br rune
	for _, r := range a {
		ar = r
		break
	}
	for _, r := range b {
		br = r
		break
	}
	if ar == br {
		return true
	}
	return keyboardAdjacent(ar, br)
}

func keyboardAdjacent(a, b rune) bool {
	neighbours := map[rune]string{
		'a': "qwsz", 'b': "vghn", 'c': "xdfv", 'd': "serfcx", 'e': "wsdr",
		'f': "drtgcv", 'g': "ftyhvb", 'h': "gyujnb", 'i': "ujko", 'j': "huikmn",
		'k': "jiolm", 'l': "kop", 'm': "njk", 'n': "bhjm", 'o': "iklp",
		'p': "ol", 'q': "wa", 'r': "edft", 's': "awedxz", 't': "rfgy",
		'u': "yhji", 'v': "cfgb", 'w': "qase", 'x': "zsdc", 'y': "tghu",
		'z': "asx",
	}
	a = unicode.ToLower(a)
	b = unicode.ToLower(b)
	if ns, ok := neighbours[a]; ok {
		return strings.ContainsRune(ns, b)
	}
	return false
}

func levenshtein(a, b []rune, cutoff int) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		minRow := curr[0]
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
			if curr[j] < minRow {
				minRow = curr[j]
			}
		}
		if minRow > cutoff {
			return cutoff + 1
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

func matchCase(s string, upperFirst bool) string {
	if !upperFirst || s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
