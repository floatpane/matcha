//go:build !cgo

package clib

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// HTMLToElements parses HTML and returns structured elements (pure Go fallback).
func HTMLToElements(html string) ([]HTMLElement, bool) {
	if len(html) == 0 {
		return nil, true
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil, false
	}

	doc.Find("style, script").Remove()

	var elements []HTMLElement

	// Process h1 elements
	doc.Find("h1").Each(func(i int, s *goquery.Selection) {
		elements = append(elements, HTMLElement{Type: HElemH1, Text: s.Text()})
		s.ReplaceWithHtml("\n\n")
	})

	// Process h2 elements
	doc.Find("h2").Each(func(i int, s *goquery.Selection) {
		elements = append(elements, HTMLElement{Type: HElemH2, Text: s.Text()})
		s.ReplaceWithHtml("\n\n")
	})

	// Add newlines after block elements
	doc.Find("p, div").Each(func(i int, s *goquery.Selection) {
		s.After("\n\n")
	})

	// Replace <br> with newlines
	doc.Find("br").Each(func(i int, s *goquery.Selection) {
		s.ReplaceWithHtml("\n")
	})

	// Process blockquotes
	onWroteRegex := regexp.MustCompile(`On\s+(.+?),\s+(.+?)\s+wrote:`)
	doc.Find("blockquote").Each(func(i int, s *goquery.Selection) {
		cite, _ := s.Attr("cite")
		quoteText := strings.TrimSpace(s.Text())

		var prevText string
		if prev := s.Prev(); prev.Length() > 0 {
			prevText = strings.TrimSpace(prev.Text())
			if onWroteRegex.MatchString(prevText) {
				s.Prev().Remove()
			}
		}

		elem := HTMLElement{
			Type: HElemBlockquote,
			Text: quoteText,
		}
		if cite != "" {
			elem.Attr1 = cite
		}
		if prevText != "" {
			elem.Attr2 = prevText
		}
		elements = append(elements, elem)
		s.ReplaceWithHtml("\n")
	})

	// Process links
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		elements = append(elements, HTMLElement{
			Type:  HElemLink,
			Text:  s.Text(),
			Attr1: href,
		})
		s.ReplaceWithHtml("")
	})

	// Process images
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists {
			return
		}
		alt, _ := s.Attr("alt")
		if alt == "" {
			alt = "Does not contain alt text"
		}
		elements = append(elements, HTMLElement{
			Type:  HElemImage,
			Attr1: src,
			Attr2: alt,
		})
		s.ReplaceWithHtml("")
	})

	// Get remaining text
	text := doc.Text()
	if strings.TrimSpace(text) != "" {
		elements = append(elements, HTMLElement{Type: HElemText, Text: text})
	}

	return elements, true
}
