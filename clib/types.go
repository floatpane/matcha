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
	HElemTable      = 6
	HElemCode       = 7
	HElemH3         = 8
	HElemH4         = 9
	HElemH5         = 10
	HElemH6         = 11
	HElemListItem   = 12
	HElemHR         = 13
)

const (
	HTMLStyleBold          = 0x01
	HTMLStyleItalic        = 0x02
	HTMLStyleUnderline     = 0x04
	HTMLStyleStrikethrough = 0x08
)

// HTMLElement represents a parsed element from an HTML document.
type HTMLElement struct {
	Type  int
	Style int
	Text  string
	Attr1 string
	Attr2 string
}
