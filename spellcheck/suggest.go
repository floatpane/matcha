package spellcheck

import (
	"sort"
	"strings"
	"unicode"
)

// Suggest returns up to limit candidate corrections for word, ranked by
// edit distance ascending then alphabetically. Returns nil when the
// checker has no dictionary loaded or when word is too short.
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

	// Allow up to 2 edits for short-to-medium words, 3 for longer ones.
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
		// Length filter: prune impossible candidates without an alloc.
		ld := len(w) - len(lower)
		if ld < 0 {
			ld = -ld
		}
		if ld > maxDist {
			continue
		}
		// First-rune similarity prunes most mismatched candidates cheaply.
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

// firstRuneClose returns true when the first runes of a and b are equal,
// adjacent on a QWERTY keyboard, or one of them is missing.
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

// keyboardAdjacent returns true when a and b are neighbours on a QWERTY
// keyboard. Used purely to widen the candidate pool around typos like
// "guzzy"→"fuzzy" (g↔f) without exploding the cost of suggestion.
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

// levenshtein computes the edit distance between a and b, returning early
// once the running minimum exceeds cutoff. Two-row dynamic programming
// keeps the allocation small.
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

// matchCase capitalises the first rune of s when the original word
// started with an uppercase letter, so suggestions blend back into the
// user's writing.
func matchCase(s string, upperFirst bool) string {
	if !upperFirst || s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
