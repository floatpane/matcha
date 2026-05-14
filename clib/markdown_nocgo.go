//go:build !cgo

package clib

import (
	"bytes"
	"log"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

// MarkdownToHTML converts Markdown bytes to HTML using goldmark (pure Go fallback).
func MarkdownToHTML(md []byte) []byte {
	var buf bytes.Buffer
	p := goldmark.New(
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	if err := p.Convert(md, &buf); err != nil {
		log.Printf("markdown: goldmark conversion failed, falling back to escaped plain-text HTML: %v", err)
		return markdownPlainTextHTML(md)
	}
	return buf.Bytes()
}
