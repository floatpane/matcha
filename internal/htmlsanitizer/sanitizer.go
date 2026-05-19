package htmlsanitizer

type Sanitizer interface {
	SanitizeBytes(html []byte) []byte
}
