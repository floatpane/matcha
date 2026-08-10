package htmlsanitizer

import (
	"encoding/base64"
	"net/url"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

type LibSanitizer struct {
	policy *bluemonday.Policy
}

func NewLibSanitizer() LibSanitizer {
	return LibSanitizer{policy: newPolicy()}
}

func (s LibSanitizer) SanitizeBytes(html []byte) []byte {
	return s.policy.SanitizeBytes(html)
}

func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	linkURLPattern := regexp.MustCompile(`(?i)^(https?://|mailto:|tel:)`)
	imageURLPattern := regexp.MustCompile(`(?i)^(https?://|cid:|data:image/)`)
	dataImagePrefixPattern := regexp.MustCompile(`(?i)^image/(gif|jpe?g|png|webp);base64,`)
	langClassPattern := regexp.MustCompile(`(?i)^language-[a-z0-9+#.]+$`)
	p.AllowElements(
		"a", "b", "blockquote", "br", "code", "del", "div", "em", "h1", "h2",
		"h3", "h4", "h5", "h6", "hr", "i", "img", "li", "ol", "p", "pre", "s",
		"span", "strong", "table", "tbody", "td", "th", "thead", "tr", "u", "ul",
	)
	p.AllowAttrs("href").Matching(linkURLPattern).OnElements("a")
	p.AllowAttrs("src").Matching(imageURLPattern).OnElements("img")
	p.AllowAttrs("alt").OnElements("img")
	p.AllowAttrs("cite").OnElements("blockquote")
	p.AllowAttrs("class").Matching(langClassPattern).OnElements("pre", "code")
	p.RequireParseableURLs(true)
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	p.AllowURLSchemeWithCustomPolicy("cid", func(u *url.URL) bool {
		return u.Opaque != "" && u.RawQuery == "" && u.Fragment == ""
	})
	p.AllowURLSchemeWithCustomPolicy("data", func(u *url.URL) bool {
		if u.RawQuery != "" || u.Fragment != "" {
			return false
		}
		prefix := dataImagePrefixPattern.FindString(u.Opaque)
		if prefix == "" {
			return false
		}
		payload := u.Opaque[len(prefix):]
		if _, err := base64.StdEncoding.DecodeString(payload); err == nil {
			return true
		}
		_, err := base64.RawStdEncoding.DecodeString(payload)
		return err == nil
	})
	return p
}
