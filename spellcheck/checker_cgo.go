//go:build cgo

package spellcheck

/*
#cgo CFLAGS: -I${SRCDIR}/../clib/spelldict
#cgo darwin LDFLAGS: -L${SRCDIR}/../clib/spelldict/target/release -lspelldict
#cgo linux LDFLAGS: -L${SRCDIR}/../clib/spelldict/target/release -lspelldict -ldl -lpthread -lm
#include "spelldict.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unsafe"
)

// Checker holds an opaque handle to a Rust-owned dictionary.
type Checker struct {
	mu       sync.RWMutex
	dict     *C.SpellDict
	loaded   bool
	language string
}

// NewChecker returns an empty checker. Load must be called before Check
// returns useful results.
func NewChecker() *Checker {
	return &Checker{}
}

// Load reads a Hunspell .dic file via the Rust parser and replaces the
// current dictionary. The Rust side owns all word/rune storage.
func (c *Checker) Load(path, language string) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	dict := C.spelldict_load(cpath)
	if dict == nil {
		return fmt.Errorf("spelldict: failed to load %q", path)
	}

	c.mu.Lock()
	if c.dict != nil {
		C.spelldict_free(c.dict)
	}
	c.dict = dict
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

// Check reports whether the word is recognised by the Rust dictionary.
// The same skip rules as the pure-Go implementation apply: non-checkable
// tokens and words whose script isn't covered by the dictionary are accepted.
func (c *Checker) Check(word string) bool {
	if !IsCheckable(word) {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.loaded || c.dict == nil {
		return true
	}
	// Ask Rust whether the dictionary covers this word's script.
	cword := C.CString(word)
	defer C.free(unsafe.Pointer(cword))
	if C.spelldict_covers(c.dict, cword, C.size_t(len(word))) == 0 {
		return true
	}
	// Lookup lowercased; Rust handles apostrophe-suffix stripping internally.
	lower := strings.ToLower(word)
	clower := C.CString(lower)
	defer C.free(unsafe.Pointer(clower))
	return C.spelldict_contains(c.dict, clower, C.size_t(len(lower))) != 0
}

// Suggest returns up to limit candidate corrections, ranked by edit distance.
// The Levenshtein search runs entirely inside Rust.
func (c *Checker) Suggest(word string, limit int) []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.loaded || c.dict == nil {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}
	lower := strings.ToLower(word)
	if len([]rune(lower)) < 2 {
		return nil
	}

	cword := C.CString(lower)
	defer C.free(unsafe.Pointer(cword))

	raw := C.spelldict_suggest(c.dict, cword, C.size_t(len(lower)), C.int(limit))
	if raw == nil {
		return nil
	}
	defer C.spelldict_free_suggestions(raw)

	upper := unicode.IsUpper([]rune(word)[0])
	// Walk the NULL-terminated *char array.
	ptrs := (*[1 << 16]*C.char)(unsafe.Pointer(raw))
	var result []string
	for i := 0; ptrs[i] != nil; i++ {
		result = append(result, matchCase(C.GoString(ptrs[i]), upper))
	}
	return result
}

func matchCase(s string, upperFirst bool) string {
	if !upperFirst || s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
