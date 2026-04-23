package i18n

import "strings"

// FallbackChain defines a sequence of languages to try when looking up translations.
type FallbackChain struct {
	langs []string
}

// NewFallbackChain creates a new fallback chain with a preferred language and defaults.
// Example: NewFallbackChain("pt-BR", "pt", "en") creates chain: pt-BR → pt → en
func NewFallbackChain(preferred string, defaults ...string) *FallbackChain {
	chain := &FallbackChain{
		langs: make([]string, 0, len(defaults)+2),
	}

	// Add preferred language
	if preferred != "" {
		chain.langs = append(chain.langs, preferred)

		// If preferred has region code (e.g., "en-US"), also add base (e.g., "en")
		if parts := strings.Split(preferred, "-"); len(parts) > 1 {
			base := parts[0]
			if !contains(chain.langs, base) {
				chain.langs = append(chain.langs, base)
			}
		}
	}

	// Add fallback languages
	for _, lang := range defaults {
		if lang != "" && !contains(chain.langs, lang) {
			chain.langs = append(chain.langs, lang)
		}
	}

	return chain
}

// Resolve attempts to find a message in the fallback chain.
// Returns the message, the language it was found in, and any error.
func (f *FallbackChain) Resolve(bundle *Bundle, key string) (*Message, string, error) {
	for _, lang := range f.langs {
		msg, err := bundle.GetMessage(lang, key)
		if err == nil {
			return msg, lang, nil
		}
	}

	return nil, "", ErrMessageNotFound
}

// Languages returns the ordered list of languages in the fallback chain.
func (f *FallbackChain) Languages() []string {
	return f.langs
}

// contains checks if a slice contains a string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
