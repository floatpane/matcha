package htmlsanitizer

import (
	"strings"
	"testing"
)

func TestLibSanitizerRemovesUnsafeHTML(t *testing.T) {
	sanitizer := NewLibSanitizer()
	input := []byte(`
		<p onclick="alert(1)">Hello</p>
		<script>alert(1)</script>
		<style>body { background-image: url("javascript:alert(1)") }</style>
		<a href="javascript:alert(1)">bad link</a>
		<a href="https://example.com">good link</a>
		<img src="file:///tmp/bad.png" alt="bad image">
		<img src="cid:test@example.com" alt="cid image">
		<img src="data:text/html,<script>alert(1)</script>" alt="bad data">
		<img src="data:image/png;base64,iVBORw0KGgo=" alt="data image">
	`)

	got := string(sanitizer.SanitizeBytes(input))

	for _, forbidden := range []string{
		"onclick",
		"<script",
		"<style",
		"javascript:",
		"file:///tmp/bad.png",
		"data:text/html",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized HTML contains %q:\n%s", forbidden, got)
		}
	}

	for _, want := range []string{
		`href="https://example.com"`,
		`src="cid:test@example.com"`,
		`src="data:image/png;base64,iVBORw0KGgo="`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitized HTML does not contain %q:\n%s", want, got)
		}
	}
}

func TestLibSanitizerDoesNotAllowDataOrCIDLinks(t *testing.T) {
	sanitizer := NewLibSanitizer()
	input := []byte(`
		<a href="data:image/png;base64,iVBORw0KGgo=">data link</a>
		<a href="cid:test@example.com">cid link</a>
		<a href="ftp://example.com/file.txt">ftp link</a>
		<a href="file:///tmp/bad.txt">file link</a>
		<a href="vbscript:msgbox(1)">vbscript link</a>
		<a href="//example.com/protocol-relative">protocol relative link</a>
		<a href="/relative/path">relative link</a>
		<a href=":not-a-url">broken link</a>
	`)

	got := string(sanitizer.SanitizeBytes(input))

	for _, forbidden := range []string{
		"href=\"data:image",
		"href=\"cid:",
		"href=\"ftp:",
		"href=\"file:",
		"href=\"vbscript:",
		"href=\"//example.com",
		"href=\"/relative",
		"href=\":not-a-url",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized HTML contains %q:\n%s", forbidden, got)
		}
	}

	for _, wantText := range []string{
		"data link",
		"cid link",
		"ftp link",
		"file link",
		"vbscript link",
		"protocol relative link",
		"relative link",
		"broken link",
	} {
		if !strings.Contains(got, wantText) {
			t.Fatalf("sanitized HTML should keep link text %q:\n%s", wantText, got)
		}
	}
}

func TestLibSanitizerAllowsSafeLinks(t *testing.T) {
	sanitizer := NewLibSanitizer()
	input := []byte(`
		<a href="http://example.com/path?x=1">http link</a>
		<a href="https://example.com/path?x=1">https link</a>
		<a href="HTTPS://example.com/path?x=1">uppercase https link</a>
		<a href="mailto:security@example.com">mailto link</a>
		<a href="MAILTO:security@example.com">uppercase mailto link</a>
		<a href="tel:+15551234567">tel link</a>
	`)

	got := string(sanitizer.SanitizeBytes(input))

	for _, want := range []string{
		`href="http://example.com/path?x=1"`,
		`href="https://example.com/path?x=1"`,
		`href="https://example.com/path?x=1"`,
		`href="mailto:security@example.com"`,
		`href="mailto:security@example.com"`,
		`href="tel:+15551234567"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitized HTML does not contain %q:\n%s", want, got)
		}
	}
}

func TestLibSanitizerFiltersImageSources(t *testing.T) {
	sanitizer := NewLibSanitizer()
	input := []byte(`
		<img src="http://example.com/image.png" alt="http image">
		<img src="https://example.com/image.png" alt="https image">
		<img src="cid:test@example.com" alt="cid image">
		<img src="data:image/png;base64,iVBORw0KGgo=" alt="data image">
		<img src="javascript:alert(1)" alt="javascript image">
		<img src="file:///tmp/bad.png" alt="file image">
		<img src="data:text/html,<script>alert(1)</script>" alt="html data image">
		<img src="/relative.png" alt="relative image">
	`)

	got := string(sanitizer.SanitizeBytes(input))

	for _, want := range []string{
		`src="http://example.com/image.png"`,
		`src="https://example.com/image.png"`,
		`src="cid:test@example.com"`,
		`src="data:image/png;base64,iVBORw0KGgo="`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitized HTML does not contain %q:\n%s", want, got)
		}
	}

	for _, forbidden := range []string{
		"src=\"javascript:",
		"src=\"file:",
		"src=\"data:text/html",
		"src=\"/relative.png",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized HTML contains %q:\n%s", forbidden, got)
		}
	}
}

func TestLibSanitizerRemovesUnknownElementsButKeepsText(t *testing.T) {
	sanitizer := NewLibSanitizer()
	input := []byte(`
		<form action="https://example.com"><input name="token" value="secret">form text</form>
		<iframe src="https://example.com">iframe text</iframe>
		<object data="https://example.com">object text</object>
		<p>safe text</p>
	`)

	got := string(sanitizer.SanitizeBytes(input))

	for _, forbidden := range []string{
		"<form",
		"<input",
		"<iframe",
		"<object",
		"action=",
		"value=\"secret\"",
		"src=\"https://example.com\"",
		"data=\"https://example.com\"",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized HTML contains %q:\n%s", forbidden, got)
		}
	}

	for _, wantText := range []string{
		"form text",
		"safe text",
	} {
		if !strings.Contains(got, wantText) {
			t.Fatalf("sanitized HTML should keep text %q:\n%s", wantText, got)
		}
	}
}

func TestLibSanitizerRemovesUnsafeGlobalAttributes(t *testing.T) {
	sanitizer := NewLibSanitizer()
	input := []byte(`
		<p style="color: red" class="promo" data-secret="token" id="message">styled text</p>
		<blockquote cite="https://example.com" onclick="alert(1)">quote text</blockquote>
	`)

	got := string(sanitizer.SanitizeBytes(input))

	for _, forbidden := range []string{
		"style=",
		"class=",
		"data-secret",
		"id=",
		"onclick=",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized HTML contains %q:\n%s", forbidden, got)
		}
	}

	for _, want := range []string{
		"styled text",
		`cite="https://example.com"`,
		"quote text",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitized HTML does not contain %q:\n%s", want, got)
		}
	}
}

func TestLibSanitizerRejectsCIDWithQueryOrFragment(t *testing.T) {
	sanitizer := NewLibSanitizer()
	input := []byte(`
		<img src="cid:test@example.com?x=1" alt="cid query">
		<img src="cid:test@example.com#frag" alt="cid fragment">
		<img src="cid:test@example.com" alt="cid ok">
	`)

	got := string(sanitizer.SanitizeBytes(input))

	for _, forbidden := range []string{
		`src="cid:test@example.com?x=1"`,
		`src="cid:test@example.com#frag"`,
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized HTML contains %q:\n%s", forbidden, got)
		}
	}

	if !strings.Contains(got, `src="cid:test@example.com"`) {
		t.Fatalf("sanitized HTML should keep clean cid source:\n%s", got)
	}
}

func TestLibSanitizerRejectsInvalidDataImages(t *testing.T) {
	sanitizer := NewLibSanitizer()
	input := []byte(`
		<img src="data:image/png;base64,not base64!" alt="invalid base64">
		<img src="data:image/svg+xml;base64,PHN2Zy8+" alt="svg data">
		<img src="data:image/png;base64,iVBORw0KGgo=" alt="png data">
		<img src="data:image/png;base64,iVBORw0KGgo" alt="raw png data">
	`)

	got := string(sanitizer.SanitizeBytes(input))

	for _, forbidden := range []string{
		"not base64",
		"data:image/svg+xml",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized HTML contains %q:\n%s", forbidden, got)
		}
	}

	if !strings.Contains(got, `src="data:image/png;base64,iVBORw0KGgo="`) {
		t.Fatalf("sanitized HTML should keep valid png data URI:\n%s", got)
	}
	if !strings.Contains(got, `src="data:image/png;base64,iVBORw0KGgo"`) {
		t.Fatalf("sanitized HTML should keep valid unpadded png data URI:\n%s", got)
	}
}
