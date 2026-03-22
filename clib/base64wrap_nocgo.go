//go:build !cgo

package clib

import "strings"

// WrapBase64 wraps base64-encoded data at 76 characters per line with \r\n
// separators, as required by MIME (RFC 2045).
// This is the pure Go fallback used when cgo is not available.
func WrapBase64(data string) string {
	const lineLength = 76
	if len(data) == 0 {
		return ""
	}
	var result strings.Builder
	for i := 0; i < len(data); i += lineLength {
		end := i + lineLength
		if end > len(data) {
			end = len(data)
		}
		result.WriteString(data[i:end])
		if end < len(data) {
			result.WriteString("\r\n")
		}
	}
	return result.String()
}
