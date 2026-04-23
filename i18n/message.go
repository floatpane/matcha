package i18n

// Message represents a translatable message with support for plural forms.
type Message struct {
	// ID is the unique identifier for this message (e.g., "composer.title")
	ID string `json:"id"`

	// Description provides context for translators
	Description string `json:"description,omitempty"`

	// Hash is an optional content hash for tracking changes
	Hash string `json:"hash,omitempty"`

	// Zero form is used when count is exactly 0 (optional)
	Zero string `json:"zero,omitempty"`

	// One form is used for singular (count == 1)
	One string `json:"one,omitempty"`

	// Two form is used for dual (count == 2) in some languages
	Two string `json:"two,omitempty"`

	// Few form is used for small counts in some languages (e.g., Polish)
	Few string `json:"few,omitempty"`

	// Many form is used for larger counts in some languages (e.g., Russian)
	Many string `json:"many,omitempty"`

	// Other is the default form used when no specific plural form matches
	Other string `json:"other,omitempty"`
}

// MessageMap maps message IDs to Message structs.
type MessageMap map[string]*Message

// GetText returns the appropriate text for the given plural form.
func (m *Message) GetText(form PluralForm) string {
	switch form {
	case Zero:
		if m.Zero != "" {
			return m.Zero
		}
	case One:
		if m.One != "" {
			return m.One
		}
	case Two:
		if m.Two != "" {
			return m.Two
		}
	case Few:
		if m.Few != "" {
			return m.Few
		}
	case Many:
		if m.Many != "" {
			return m.Many
		}
	}
	// Fallback to Other or One
	if m.Other != "" {
		return m.Other
	}
	return m.One
}

// GetDefault returns the most appropriate default text (tries Other, then One).
func (m *Message) GetDefault() string {
	if m.Other != "" {
		return m.Other
	}
	if m.One != "" {
		return m.One
	}
	if m.Zero != "" {
		return m.Zero
	}
	if m.Few != "" {
		return m.Few
	}
	if m.Many != "" {
		return m.Many
	}
	if m.Two != "" {
		return m.Two
	}
	return ""
}
