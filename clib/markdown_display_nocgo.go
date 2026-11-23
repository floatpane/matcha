//go:build !cgo

package clib

import (
	"bytes"
	"log"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

// MarkdownToHTMLHardBreaks converts Markdown to HTML like MarkdownToHTML but
// keeps every newline as a line break. Use it to display plain-text email,
// whose line breaks are meaningful (signatures, addresses, lists).
func MarkdownToHTMLHardBreaks(md []byte) []byte {
	var buf bytes.Buffer
	p := goldmark.New(
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			html.WithHardWraps(),
		),
	)
	if err := p.Convert(md, &buf); err != nil {
		log.Printf("markdown: goldmark conversion failed, falling back to escaped plain-text HTML: %v", err)
		return markdownPlainTextHTML(md)
	}
	return buf.Bytes()
}
