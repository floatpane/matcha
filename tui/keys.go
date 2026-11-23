package tui

import "unicode/utf8"

// IsTypeableKey reports whether k is a key that types text: a single
// character or "space". Names like "esc" and modified or function keys
// ("ctrl+q", "f10") are not typeable.
func IsTypeableKey(k string) bool {
	return k == keySpaceBar || utf8.RuneCountInString(k) == 1
}
