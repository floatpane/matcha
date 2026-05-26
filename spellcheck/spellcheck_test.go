package spellcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestChecker(t *testing.T, words ...string) *Checker {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.dic")
	content := []byte("# header\n" + strings.Join(words, "\n") + "\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write dic: %v", err)
	}
	c := NewChecker()
	if err := c.Load(path, "test"); err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

func TestCheckerCheck(t *testing.T) {
	c := newTestChecker(t, "hello", "world", "go")
	if !c.Check("hello") {
		t.Error("hello should be known")
	}
	if !c.Check("Hello") {
		t.Error("Hello should match case-insensitively")
	}
	if c.Check("helo") {
		t.Error("helo should be unknown")
	}
	// Short / numeric / uppercase tokens are skipped.
	if !c.Check("Z") {
		t.Error("single rune skipped")
	}
	if !c.Check("ABC") {
		t.Error("short uppercase acronym skipped")
	}
	if !c.Check("42") {
		t.Error("numeric skipped")
	}
}

func TestTokenize(t *testing.T) {
	got := Tokenize("hello, world! it's nice")
	want := []struct {
		w          string
		start, end int
	}{
		{"hello", 0, 5},
		{"world", 7, 12},
		{"it's", 14, 18},
		{"nice", 19, 23},
	}
	if len(got) != len(want) {
		t.Fatalf("tokens = %d, want %d (%+v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Word != w.w || got[i].Start != w.start || got[i].End != w.end {
			t.Errorf("token %d = %+v, want %s [%d:%d]", i, got[i], w.w, w.start, w.end)
		}
	}
}

func TestParseHunspellDicSkipsCountLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.dic")
	// First line is a count, words follow, with hunspell-style flags.
	body := "3\nfoo/AB\nbar\nbaz\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	w, _, err := parseHunspellDic(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"foo", "bar", "baz"} {
		if _, ok := w[k]; !ok {
			t.Errorf("missing %q", k)
		}
	}
}

func TestHighlightWrapsMisspelled(t *testing.T) {
	c := newTestChecker(t, "hello", "world", "abcdefghijklmnopqrstuvwxyz")
	out := Highlight("hello wurld", c, -1)
	if !strings.Contains(out, "wurld") {
		t.Fatalf("output missing word: %q", out)
	}
	if !strings.Contains(out, openSGR) || !strings.Contains(out, closeSGR) {
		t.Errorf("expected SGR markers, got %q", out)
	}
	// "hello" is correct and must not be wrapped.
	idxHello := strings.Index(out, "hello")
	idxOpen := strings.Index(out, openSGR)
	if idxOpen < idxHello {
		t.Errorf("opener appeared before hello: open=%d hello=%d", idxOpen, idxHello)
	}
}

func TestHighlightPreservesANSI(t *testing.T) {
	c := newTestChecker(t, "good", "abcdefghijklmnopqrstuvwxyz")
	// Pretend the line was rendered with a colour style around the whole
	// content: ESC[31m...ESC[0m. Misspelled token "bd" inside.
	in := "\x1b[31mgood bd\x1b[0m"
	out := Highlight(in, c, -1)
	if !strings.Contains(out, "\x1b[31m") {
		t.Errorf("original colour ANSI lost: %q", out)
	}
	if !strings.Contains(out, openSGR) {
		t.Errorf("missing underline open: %q", out)
	}
}

func TestHighlightNoCheckerIsNoop(t *testing.T) {
	in := "anything goes"
	if got := Highlight(in, nil, -1); got != in {
		t.Errorf("nil checker should be no-op, got %q", got)
	}
}

func TestSuggest(t *testing.T) {
	c := newTestChecker(t, "hello", "help", "world", "word", "ward", "wild")
	got := c.Suggest("wurld", 5)
	if len(got) == 0 {
		t.Fatal("expected at least one suggestion")
	}
	// "world" should outrank "ward" / "wild" by edit distance.
	if got[0] != "world" {
		t.Errorf("top suggestion = %q, want world (all: %v)", got[0], got)
	}
}

func TestSuggestCaseMatch(t *testing.T) {
	c := newTestChecker(t, "hello", "world")
	got := c.Suggest("Wurld", 3)
	if len(got) == 0 || got[0] != "World" {
		t.Errorf("expected capitalised World, got %v", got)
	}
}

func TestCheckSkipsForeignScript(t *testing.T) {
	c := newTestChecker(t, "hello", "world")
	// Cyrillic — dict has no cyrillic runes, so we must NOT flag it.
	if !c.Check("привет") {
		t.Error("cyrillic word should be skipped against latin dict")
	}
	// Accented French not in dict ('é' absent) — must not flag.
	if !c.Check("café") {
		t.Error("accented word with foreign rune should be skipped")
	}
	// Plain ASCII typo still flagged.
	if c.Check("helo") {
		t.Error("ASCII typo should still be flagged")
	}
}

func TestCheckRecognisesAccentsWhenDictHasThem(t *testing.T) {
	// Dictionary that legitimately contains an accented word — its rune
	// set covers 'é' so accented words can be evaluated normally.
	c := newTestChecker(t, "café", "hello")
	if !c.Check("café") {
		t.Error("café should be recognised when present in dict")
	}
	if c.Check("cofé") {
		t.Error("misspelled accented word should still be flagged")
	}
}

func TestIsCheckable(t *testing.T) {
	cases := map[string]bool{
		"hello":         true,
		"a":             false,
		"42":            false,
		"hello42":       false,
		"NASA":          false,
		"hi@there":      false,
		"path/to":       false,
		"don't":         true,
		"HelloWorld":    true, // mixed case, not an acronym
		"INTERNATIONAL": true, // > 5 upper letters, treated as a word
	}
	for in, want := range cases {
		if got := IsCheckable(in); got != want {
			t.Errorf("IsCheckable(%q) = %v, want %v", in, got, want)
		}
	}
}
