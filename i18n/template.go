package i18n

import "strings"

// Template represents a parsed template string with placeholders.
type Template struct {
	raw   string
	parts []templatePart
}

type templatePart struct {
	isVar bool
	value string
}

// NewTemplate parses a template string and returns a Template.
func NewTemplate(s string) *Template {
	t := &Template{
		raw:   s,
		parts: parseTemplate(s),
	}
	return t
}

// Execute applies data to the template and returns the result.
func (t *Template) Execute(data map[string]interface{}) string {
	if len(t.parts) == 0 {
		return t.raw
	}

	var result strings.Builder
	for _, part := range t.parts {
		if part.isVar {
			if val, ok := data[part.value]; ok {
				result.WriteString(formatValue(val))
			} else {
				// Keep placeholder if no value provided
				result.WriteString("{")
				result.WriteString(part.value)
				result.WriteString("}")
			}
		} else {
			result.WriteString(part.value)
		}
	}
	return result.String()
}

// parseTemplate breaks a template string into parts (literal text and variables).
func parseTemplate(s string) []templatePart {
	var parts []templatePart
	var current strings.Builder
	inVar := false
	var varName strings.Builder

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '{' && !inVar {
			// Start of variable
			if current.Len() > 0 {
				parts = append(parts, templatePart{isVar: false, value: current.String()})
				current.Reset()
			}
			inVar = true
			varName.Reset()
		} else if ch == '}' && inVar {
			// End of variable
			if varName.Len() > 0 {
				parts = append(parts, templatePart{isVar: true, value: varName.String()})
			}
			inVar = false
		} else if inVar {
			varName.WriteByte(ch)
		} else {
			current.WriteByte(ch)
		}
	}

	// Add remaining text
	if current.Len() > 0 {
		parts = append(parts, templatePart{isVar: false, value: current.String()})
	}

	return parts
}

// String returns the raw template string.
func (t *Template) String() string {
	return t.raw
}
