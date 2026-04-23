package i18n

import (
	"fmt"
	"strings"
)

// Interpolate replaces placeholders in a template string with values from data.
// Supports {key} syntax for variable interpolation.
func Interpolate(template string, data map[string]interface{}) string {
	if data == nil || len(data) == 0 {
		return template
	}

	result := template
	for key, value := range data {
		placeholder := "{" + key + "}"
		replacement := formatValue(value)
		result = strings.ReplaceAll(result, placeholder, replacement)
	}

	return result
}

// formatValue converts a value to its string representation.
func formatValue(v interface{}) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
