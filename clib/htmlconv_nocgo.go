//go:build !cgo

package clib

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// HTMLToElements parses HTML and returns structured elements (pure Go fallback).
func HTMLToElements(htmlStr string) ([]HTMLElement, bool) {
	if len(htmlStr) == 0 {
		return nil, true
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(htmlStr)))
	if err != nil {
		return nil, false
	}

	doc.Find("style, script").Remove()

	var elements []HTMLElement

	headingTypes := map[string]int{
		"h1": HElemH1, "h2": HElemH2, "h3": HElemH3,
		"h4": HElemH4, "h5": HElemH5, "h6": HElemH6,
	}
	for tag, etype := range headingTypes {
		t := etype
		doc.Find(tag).Each(func(i int, s *goquery.Selection) {
			elements = append(elements, HTMLElement{Type: t, Text: s.Text()})
			s.ReplaceWithHtml("\n\n")
		})
	}

	doc.Find("hr").Each(func(i int, s *goquery.Selection) {
		elements = append(elements, HTMLElement{Type: HElemHR, Text: ""})
		s.ReplaceWithHtml("\n\n")
	})

	doc.Find("li").Each(func(i int, s *goquery.Selection) {
		depth := s.Parents().Filter("ul,ol").Length() - 1
		if depth < 0 {
			depth = 0
		}
		elem := HTMLElement{
			Type:  HElemListItem,
			Text:  s.Text(),
			Attr1: itoa(depth),
		}
		if parent := s.Parent(); parent.Length() > 0 {
			if goquery.NodeName(parent) == "ol" {
				idx := s.PrevAll().Filter("li").Length() + 1
				elem.Attr2 = itoa(idx)
			}
		}
		elements = append(elements, elem)
		s.ReplaceWithHtml("\n")
	})
	doc.Find("ul,ol").Each(func(i int, s *goquery.Selection) {
		s.ReplaceWithHtml("\n\n")
	})

	doc.Find("p, div").Each(func(i int, s *goquery.Selection) {
		s.After("\n\n")
	})

	doc.Find("br").Each(func(i int, s *goquery.Selection) {
		s.ReplaceWithHtml("\n")
	})

	doc.Find("pre").Each(func(i int, s *goquery.Selection) {
		var lang string
		if code := s.Find("code"); code.Length() > 0 {
			if class, ok := code.Attr("class"); ok {
				lang = extractLanguageClass(class)
			}
		}
		if lang == "" {
			if class, ok := s.Attr("class"); ok {
				lang = extractLanguageClass(class)
			}
		}
		text := s.Text()
		elem := HTMLElement{Type: HElemCode, Text: text}
		if lang != "" {
			elem.Attr1 = lang
		}
		elements = append(elements, elem)
		s.ReplaceWithHtml("\n\n")
	})

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

	walkInline(doc.Find("body"), 0, &elements)

	return elements, true
}

var inlineStyleTags = map[string]int{
	"b":      HTMLStyleBold,
	"strong": HTMLStyleBold,
	"i":      HTMLStyleItalic,
	"em":     HTMLStyleItalic,
	"u":      HTMLStyleUnderline,
	"s":      HTMLStyleStrikethrough,
	"del":    HTMLStyleStrikethrough,
}

func walkInline(sel *goquery.Selection, style int, out *[]HTMLElement) {
	sel.Contents().Each(func(i int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		if node.Type == html.TextNode {
			txt := node.Data
			if strings.TrimSpace(txt) != "" {
				*out = append(*out, HTMLElement{Type: HElemText, Style: style, Text: txt})
			}
			return
		}
		if node.Type == html.ElementNode {
			name := strings.ToLower(goquery.NodeName(s))
			if mask, ok := inlineStyleTags[name]; ok {
				walkInline(s, style|mask, out)
				return
			}
			walkInline(s, style, out)
		}
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// extractLanguageClass parses a class attribute value and returns the
// language token from a "language-XXX" class, or "" if none is present.
func extractLanguageClass(class string) string {
	for _, c := range strings.Fields(class) {
		if strings.HasPrefix(c, "language-") {
			return strings.TrimPrefix(c, "language-")
		}
	}
	return ""
}
