package i18n

import (
	"encoding/json"
	"fmt"
)

// TranslationFile represents the structure of a JSON translation file.
type TranslationFile struct {
	Language string                 `json:"language"`
	Messages map[string]interface{} `json:"messages"`
}

// ParseJSON parses a JSON translation file and returns a MessageMap.
func ParseJSON(data []byte) (MessageMap, error) {
	var file TranslationFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseFailed, err)
	}

	messages := make(MessageMap)
	parseNestedMessages("", file.Messages, messages)

	return messages, nil
}

// parseNestedMessages recursively parses nested message structures.
// Builds dot-notation keys like "composer.title" from nested objects.
func parseNestedMessages(prefix string, data map[string]interface{}, messages MessageMap) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case string:
			// Simple string message
			messages[fullKey] = &Message{
				ID:    fullKey,
				Other: v,
			}

		case map[string]interface{}:
			// Check if this is a plural form object or nested structure
			if isPluralForm(v) {
				messages[fullKey] = parsePluralMessage(fullKey, v)
			} else {
				// Nested structure - recurse
				parseNestedMessages(fullKey, v, messages)
			}

		default:
			// Unexpected type - treat as string
			messages[fullKey] = &Message{
				ID:    fullKey,
				Other: fmt.Sprintf("%v", v),
			}
		}
	}
}

// isPluralForm checks if a map contains plural form keys.
func isPluralForm(data map[string]interface{}) bool {
	pluralKeys := []string{"zero", "one", "two", "few", "many", "other"}
	for _, key := range pluralKeys {
		if _, ok := data[key]; ok {
			return true
		}
	}
	return false
}

// parsePluralMessage creates a Message from plural form data.
func parsePluralMessage(id string, data map[string]interface{}) *Message {
	msg := &Message{ID: id}

	if v, ok := data["zero"].(string); ok {
		msg.Zero = v
	}
	if v, ok := data["one"].(string); ok {
		msg.One = v
	}
	if v, ok := data["two"].(string); ok {
		msg.Two = v
	}
	if v, ok := data["few"].(string); ok {
		msg.Few = v
	}
	if v, ok := data["many"].(string); ok {
		msg.Many = v
	}
	if v, ok := data["other"].(string); ok {
		msg.Other = v
	}
	if v, ok := data["description"].(string); ok {
		msg.Description = v
	}

	return msg
}
