package clib

// ImageConvertResult holds the output of DecodeToPNG.
type ImageConvertResult struct {
	PNGData []byte
	Width   int
	Height  int
}

// HTMLElementType constants mirror the C enum values in htmlconv.h.
const (
	HElemText       = 0
	HElemH1         = 1
	HElemH2         = 2
	HElemLink       = 3
	HElemImage      = 4
	HElemBlockquote = 5
)

// HTMLElement represents a parsed element from an HTML document.
type HTMLElement struct {
	Type  int
	Text  string // Text content
	Attr1 string // href (link), src (image), cite (blockquote)
	Attr2 string // alt (image), prev_text (blockquote)
}
