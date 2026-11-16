package view

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/clib"
)

// onePixelPNG is a 1×1 white PNG.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="

func useImageTerminal(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range []string{"TERM", "TERM_PROGRAM", "KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR", "ITERM_SESSION_ID", "ITERM_PROFILE",
		"WEZTERM_EXECUTABLE", "WEZTERM_CONFIG_FILE", "KONSOLE_DBUS_SESSION", "KONSOLE_VERSION", "ZELLIJ", "ZELLIJ_SESSION_NAME",
		"WARP_IS_LOCAL_SHELL_SESSION", "WARP_COMBINED_PROMPT_COMMAND_FINISHED", "SIXEL", "TMUX", "TMUX_PANE"} {
		t.Setenv(key, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	old := cellPixelOverride
	cellPixelOverride.w, cellPixelOverride.h = 8, 16
	t.Cleanup(func() { cellPixelOverride = old })
}

func TestImagePositionsFollowWrapping(t *testing.T) {
	useImageTerminal(t, map[string]string{"TERM": "xterm-kitty"})

	// A long paragraph before the image wraps onto several lines at the
	// display width; the image must be placed after all of them.
	body := `<p>` + strings.Repeat("word ", 40) + `</p><img src="data:image/png;base64,` + onePixelPNG + `" alt="pic"><p>after</p>`
	style := lipgloss.NewStyle()
	rendered, err := RenderBody(body, BodyMIMETypeHTML, nil, style, style, style, RenderOptions{Bounds: ImageBounds{MaxCols: 20, MaxRows: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if !rendered.HasImages() {
		t.Fatal("expected an inline image")
	}
	wrapped := lipgloss.NewStyle().Width(20).Render(rendered.Text)
	text, placements := rendered.Finalize(wrapped)
	if strings.Contains(text, imageMarkerPrefix) {
		t.Fatal("image marker left in the text")
	}
	if len(placements) != 1 {
		t.Fatalf("got %d placements, want 1", len(placements))
	}
	lines := strings.Split(text, "\n")
	lastWord := -1
	for i, line := range lines {
		if strings.Contains(line, "word") {
			lastWord = i
		}
	}
	p := placements[0]
	if p.Line <= lastWord {
		t.Fatalf("image placed on line %d, overlapping the wrapped paragraph that ends on line %d", p.Line, lastWord)
	}
	for i := p.Line; i < p.Line+p.Rows && i < len(lines); i++ {
		if strings.TrimSpace(ansi.Strip(lines[i])) != "" {
			t.Fatalf("image row %d is not blank: %q", i, ansi.Strip(lines[i]))
		}
	}
	if p.Protocol != ImageProtocolKitty || p.ID == 0 || p.Cols < 1 || p.Cols > 20 || p.Rows < 1 || p.Rows > 10 {
		t.Fatalf("unexpected placement %+v", p)
	}
	if !strings.Contains(p.Encoded, "a=t,") || !strings.Contains(p.Encoded, "q=2") {
		t.Fatalf("kitty upload should transmit without displaying and suppress replies: %q", p.Encoded[:min(60, len(p.Encoded))])
	}
}

func TestHalfBlockImagesAreInlineText(t *testing.T) {
	useImageTerminal(t, map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iterm.app"})
	if got := ActiveImageProtocol(); got != ImageProtocolHalfBlock {
		t.Fatalf("protocol = %v, want halfblock", got)
	}
	body := `<p>before</p><img src="data:image/png;base64,` + onePixelPNG + `" alt="pic"><p>after</p>`
	style := lipgloss.NewStyle()
	rendered, err := RenderBody(body, BodyMIMETypeHTML, nil, style, style, style, RenderOptions{Bounds: ImageBounds{MaxCols: 30, MaxRows: 10}})
	if err != nil {
		t.Fatal(err)
	}
	text, placements := rendered.Finalize(rendered.Text)
	if len(placements) != 0 {
		t.Fatalf("half-block images must not be painted out-of-band, got %d placements", len(placements))
	}
	if !strings.Contains(text, "▀") || strings.Contains(text, imageMarkerPrefix) {
		t.Fatalf("expected inline half-block image, got %q", ansi.Strip(text))
	}
	for _, line := range strings.Split(text, "\n") {
		if ansi.StringWidth(line) > 30 {
			t.Fatalf("image line wider than the bounds: %d", ansi.StringWidth(line))
		}
	}
}

func TestImageCellBox(t *testing.T) {
	cases := []struct {
		name               string
		pw, ph             int
		maxCols, maxRows   int
		wantCols, wantRows int
	}{
		{"small image keeps native size", 80, 32, 100, 50, 10, 2},
		{"wide image shrinks to width", 1600, 400, 50, 50, 50, 7},
		{"tall image shrinks to height", 400, 3200, 100, 20, 5, 20},
		{"degenerate", 0, 0, 10, 10, 1, 1},
	}
	for _, tc := range cases {
		cols, rows := imageCellBox(tc.pw, tc.ph, 8, 16, tc.maxCols, tc.maxRows)
		if cols != tc.wantCols || rows != tc.wantRows {
			t.Errorf("%s: got %dx%d cells, want %dx%d", tc.name, cols, rows, tc.wantCols, tc.wantRows)
		}
		if cols > max(1, tc.maxCols) || rows > max(1, tc.maxRows) {
			t.Errorf("%s: %dx%d exceeds bounds %dx%d", tc.name, cols, rows, tc.maxCols, tc.maxRows)
		}
	}
}

func TestDefaultImageBoundsFitSmallTerminals(t *testing.T) {
	old := terminalSize
	defer func() { terminalSize = old }()
	terminalSize = struct {
		cols, rows int
		ok         bool
	}{cols: 60, rows: 24, ok: true}

	// These used to be raised to 25 rows / 80 columns, i.e. taller and wider
	// than the terminal, which made images scroll the whole screen.
	if got := maxImageCellHeight(); got > 24 {
		t.Fatalf("maxImageCellHeight = %d on a 24-row terminal", got)
	}
	if got := maxImageCellWidth(); got > 60 {
		t.Fatalf("maxImageCellWidth = %d on a 60-column terminal", got)
	}
}

func TestKittySequencesNeverMoveTheCursor(t *testing.T) {
	place := KittyPlaceSequence(3, 2, 9, 4, 20, 6, 10, 160, 80)
	for _, want := range []string{"\x1b7", "\x1b[10;5H", "a=p,i=3,p=2", "C=1", "c=20,r=6", "y=10", "w=160", "h=80", "\x1b8"} {
		if !strings.Contains(place, want) {
			t.Errorf("placement %q missing %q", place, want)
		}
	}
	upload := kittyTransmitSequence(4, make([]byte, 9000))
	if strings.Contains(upload, "\n") || strings.Contains(place, "\n") {
		t.Fatal("kitty sequences must not contain newlines; they scroll the screen")
	}
	if strings.Count(upload, "\x1b_G") < 3 || !strings.Contains(upload, "m=0") {
		t.Fatalf("large uploads must be chunked and terminated: %d chunks", strings.Count(upload, "\x1b_G"))
	}
}

func TestPlainTextKeepsLineBreaks(t *testing.T) {
	style := lipgloss.NewStyle()
	out, _, err := ProcessBody("Cheers,\nAlice\nExample Corp", BodyMIMETypePlain, style, style, style, true)
	if err != nil {
		t.Fatal(err)
	}
	clean := ansi.Strip(out)
	if strings.Contains(clean, "Cheers, Alice") || !strings.Contains(clean, "Alice\n") {
		t.Fatalf("plain-text line breaks were merged: %q", clean)
	}
}

// bigPNG returns the base64 of a valid PNG large enough that its data: URI
// is several kilobytes long.
func bigPNG(t *testing.T) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 120, 80))
	seed := uint32(1)
	for y := range 80 {
		for x := range 120 {
			// Noise does not compress, so the PNG stays large.
			seed = seed*1664525 + 1013904223
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(seed >> 24), G: uint8(seed >> 16), B: uint8(seed >> 8), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestLargeDataURIImagesRender(t *testing.T) {
	useImageTerminal(t, map[string]string{"TERM": "xterm-kitty"})
	payload := bigPNG(t)
	if len(payload) < 4096 {
		t.Fatalf("test image too small (%d bytes) to cover long attributes", len(payload))
	}
	body := `<p>before</p><img src="data:image/png;base64,` + payload + `" alt="chart"><p>after</p>`
	style := lipgloss.NewStyle()
	text, placements, err := ProcessBodyWithInline(body, BodyMIMETypeHTML, nil, style, style, style, false)
	if err != nil {
		t.Fatal(err)
	}
	// The src attribute used to be cut at 2047 bytes, so the image never
	// decoded and the label printed kilobytes of base64 into the body.
	if len(placements) != 1 {
		t.Fatalf("got %d placements for a large data: image, want 1", len(placements))
	}
	if strings.Contains(text, payload[:64]) {
		t.Fatal("image payload leaked into the body text")
	}
}

func TestImageFallbackNeverPrintsDataURIs(t *testing.T) {
	useImageTerminal(t, map[string]string{"TERM": "dumb"})
	payload := bigPNG(t)
	body := `<img src="data:image/png;base64,` + payload + `" alt="chart">`
	style := lipgloss.NewStyle()
	text, _, err := ProcessBodyWithInline(body, BodyMIMETypeHTML, nil, style, style, style, false)
	if err != nil {
		t.Fatal(err)
	}
	clean := ansi.Strip(text)
	if strings.Contains(clean, payload[:32]) {
		t.Fatal("fallback label printed the base64 image data")
	}
	if !strings.Contains(clean, "[Image: chart, data:image/png]") {
		t.Fatalf("fallback label = %q", strings.TrimSpace(clean))
	}
}

func TestLongLinksAreNotTruncated(t *testing.T) {
	href := "https://example.com/track?" + strings.Repeat("abcdefghij", 400)
	elements, ok := clib.HTMLToElements(`<a href="` + href + `">link</a>`)
	if !ok {
		t.Fatal("parse failed")
	}
	for _, e := range elements {
		if e.Type == clib.HElemLink {
			if e.Attr1 != href {
				t.Fatalf("href truncated to %d bytes, want %d", len(e.Attr1), len(href))
			}
			return
		}
	}
	t.Fatal("no link element")
}
