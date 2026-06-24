package view

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/quotedprintable"
	"os"
	"regexp"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/clib"
	"github.com/floatpane/matcha/internal/htmlsanitizer"
	"github.com/floatpane/matcha/internal/httpclient"
	"github.com/floatpane/matcha/internal/loglevel"
	"github.com/floatpane/matcha/theme"
	"github.com/floatpane/termimage"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/term"
)

var htmlSanitizer htmlsanitizer.Sanitizer = htmlsanitizer.NewLibSanitizer()

const termGhostty = "ghostty"

func linkStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.ActiveTheme.Link)
}

// hyperlinkSupported checks if the terminal supports OSC 8 hyperlinks.
func hyperlinkSupported() bool {
	term := strings.ToLower(os.Getenv("TERM"))

	// Terminals known to support OSC 8 hyperlinks
	supportedTerms := []string{
		"kitty",
		termGhostty,
		"wezterm",
		"alacritty",
		"foot",
		"tmux",
		"screen",
	}

	for _, supported := range supportedTerms {
		if strings.Contains(term, supported) {
			return true
		}
	}

	// Check for specific terminal programs
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	supportedPrograms := []string{
		"iterm.app",
		"hyper",
		"vscode",
		termGhostty,
		"wezterm",
	}

	for _, supported := range supportedPrograms {
		if strings.Contains(termProgram, supported) {
			return true
		}
	}

	// Check for VTE-based terminals (GNOME Terminal, etc.)
	if os.Getenv("VTE_VERSION") != "" {
		return true
	}

	// Check for specific environment variables that indicate hyperlink support
	if os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "" ||
		os.Getenv("WEZTERM_EXECUTABLE") != "" ||
		os.Getenv("WT_SESSION") != "" {
		return true
	}

	return false
}

// hyperlink formats a string as either a terminal-clickable hyperlink or plain text with URL.
func hyperlink(url, text string) string {
	url = strings.TrimSpace(url)
	text = stripTerminalControls(text)
	if text == "" {
		text = url
	}

	supported := hyperlinkSupported()

	if supported {
		// Use OSC 8 hyperlink sequence for supported terminals
		return fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, linkStyle().Render(text))
	}
	// Fallback to plain text format for unsupported terminals
	if text == url {
		return fmt.Sprintf("<%s>", linkStyle().Render(url))
	}
	return fmt.Sprintf("%s <%s>", linkStyle().Render(text), linkStyle().Render(url))
}

func stripTerminalControls(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f || r == 0x9c {
			return -1
		}
		return r
	}, s)
}

func hasTerminalControls(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return r < 0x20 || r == 0x7f || r == 0x9c
	}) != -1
}

func decodeQuotedPrintable(s string) (string, error) {
	reader := quotedprintable.NewReader(strings.NewReader(s))
	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// markdownToHTML converts a Markdown string to an HTML string using md4c (C).
func markdownToHTML(md []byte) []byte {
	return clib.MarkdownToHTML(md)
}

func kittySupported() bool {
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "kitty") {
		return true
	}
	return os.Getenv("KITTY_WINDOW_ID") != ""
}

func ghosttySupported() bool {
	// Check for TERM containing ghostty
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, termGhostty) {
		return true
	}

	// Check for Ghostty-specific environment variables
	if os.Getenv("TERM_PROGRAM") == termGhostty {
		return true
	}

	// Check for GHOSTTY_RESOURCES_DIR which Ghostty sets
	return os.Getenv("GHOSTTY_RESOURCES_DIR") != ""
}

func iterm2Supported() bool {
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	if termProgram == "iterm.app" {
		return true
	}

	// Check for iTerm2-specific environment variables
	if os.Getenv("ITERM_SESSION_ID") != "" || os.Getenv("ITERM_PROFILE") != "" {
		return true
	}

	return false
}

func weztermSupported() bool {
	// Check for WezTerm-specific environment variables
	if os.Getenv("WEZTERM_EXECUTABLE") != "" || os.Getenv("WEZTERM_CONFIG_FILE") != "" {
		return true
	}

	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	if termProgram == "wezterm" {
		return true
	}

	term := strings.ToLower(os.Getenv("TERM"))
	return strings.Contains(term, "wezterm")
}

func waystSupported() bool {
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "wayst") {
		return true
	}

	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	return termProgram == "wayst"
}

func warpSupported() bool {
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	if termProgram == "warp" {
		return true
	}

	// Check for Warp-specific environment variables
	if os.Getenv("WARP_IS_LOCAL_SHELL_SESSION") != "" || os.Getenv("WARP_COMBINED_PROMPT_COMMAND_FINISHED") != "" {
		return true
	}

	return false
}

func konsoleSupported() bool {
	// Check for Konsole-specific environment variables
	if os.Getenv("KONSOLE_DBUS_SESSION") != "" || os.Getenv("KONSOLE_VERSION") != "" {
		return true
	}

	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	return termProgram == "konsole"
}

func zellijSupported() bool {
	return os.Getenv("ZELLIJ") != "" || os.Getenv("ZELLIJ_SESSION_NAME") != ""
}

func sixelSupported() bool {
	// Zellij always supports Sixel
	if zellijSupported() {
		return true
	}

	// Native Sixel terminals
	term := strings.ToLower(os.Getenv("TERM"))
	return strings.Contains(term, "mlterm") ||
		strings.Contains(term, "foot") ||
		(strings.Contains(term, "xterm") && os.Getenv("SIXEL") == "1")
}

// ImageProtocolSupported checks if any supported image protocol terminal is detected.
func ImageProtocolSupported() bool {
	return imageProtocolSupported()
}

// SixelSupported returns true if the terminal uses the Sixel graphics protocol.
func SixelSupported() bool {
	return sixelSupported()
}

// imageProtocolSupported checks if any supported image protocol terminal is detected.
func imageProtocolSupported() bool {
	return sixelSupported() || kittySupported() || ghosttySupported() || iterm2Supported() ||
		weztermSupported() || waystSupported() || warpSupported() || konsoleSupported()
}

func debugImageProtocol(format string, args ...interface{}) {
	if os.Getenv("DEBUG_IMAGE_PROTOCOL") == "" && os.Getenv("DEBUG_KITTY_IMAGES") == "" {
		return
	}
	msg := fmt.Sprintf("[img-protocol] "+format+"\n", args...)
	loglevel.Infof("%s", strings.TrimSuffix(msg, "\n"))
	if path := os.Getenv("DEBUG_IMAGE_PROTOCOL_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil { //nolint:gosec
			if _, err := f.WriteString(msg); err != nil {
				loglevel.Debugf("image protocol write error: %v", err)
			}
			if err := f.Close(); err != nil {
				loglevel.Debugf("image protocol close error: %v", err)
			}
		}
	} else if path := os.Getenv("DEBUG_KITTY_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil { //nolint:gosec
			if _, err := f.WriteString(msg); err != nil {
				loglevel.Debugf("image protocol write error: %v", err)
			}
			if err := f.Close(); err != nil {
				loglevel.Debugf("image protocol close error: %v", err)
			}
		}
	}
}

const remoteImageCacheSize = 20

// remoteImageCache caches fetched remote images (URL -> base64 PNG string).
var remoteImageCache *lru.Cache[string, string]

func init() {
	c, err := lru.New[string, string](remoteImageCacheSize)
	if err != nil {
		panic(err) // only fails on size <= 0
	}
	remoteImageCache = c
}

func fetchRemoteBase64(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ""
	}

	// Check cache first
	if cached, ok := remoteImageCache.Get(url); ok {
		debugImageProtocol("remote cache hit url=%s", url)
		return cached
	}

	client := httpclient.New(httpclient.RemoteImageTimeout)
	resp, err := client.Get(url)
	if err != nil {
		debugImageProtocol("remote fetch failed url=%s err=%v", url, err)
		return ""
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		debugImageProtocol("remote fetch non-200 url=%s status=%d", url, resp.StatusCode)
		return ""
	}
	// Limit response body to 10 MB to prevent memory exhaustion from
	// malicious or very large images.
	const maxImageSize = 10 << 20 // 10 MB
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize))
	if err != nil {
		debugImageProtocol("remote fetch read error url=%s err=%v", url, err)
		return ""
	}

	result, ok := clib.DecodeToPNG(data)
	if !ok {
		debugImageProtocol("remote decode failed url=%s", url)
		return ""
	}

	encoded := base64.StdEncoding.EncodeToString(result.PNGData)
	debugImageProtocol("remote fetch ok url=%s len=%d", url, len(encoded))
	remoteImageCache.Add(url, encoded)
	return encoded
}

func dataURIBase64(uri string) string {
	if !strings.HasPrefix(uri, "data:") {
		return ""
	}
	comma := strings.Index(uri, ",")
	if comma == -1 || comma+1 >= len(uri) {
		return ""
	}
	return uri[comma+1:]
}

// imageRowPlaceholderPrefix is used to mark where image row spacing should be inserted.
// This prevents the newline-collapsing regex from removing intentional spacing.
// Uses brackets instead of angle brackets to avoid being interpreted as HTML tags.
const imageRowPlaceholderPrefix = "[[MATCHA_IMG_ROWS:"
const imageRowPlaceholderSuffix = "]]"

// prerenderImage decodes and renders an image via termimage at layout time,
// returning the cached escape sequence and the exact number of terminal rows
// the rendered image will occupy. Both are stored on the ImagePlacement so
// (a) text below the image is offset by the correct row count and (b) the
// paint stage in RenderImageToStdout is a plain stdout write with no decode.
func prerenderImage(payload string) (string, int) {
	src := "data:image/png;base64," + payload
	var buf bytes.Buffer

	// Ask termimage to cap the rendered image so it can never exceed the
	// terminal viewport. This prevents oversized/tall images from covering the
	// whole screen or overlapping other content on scroll. We always pass a
	// maximum in cells and let termimage convert to pixels for the active
	// protocol.
	_, rows, err := termimage.DisplayWithSize(&buf, src, termimage.Options{
		Protocol:  termimage.Auto,
		Sandboxed: true,
		MaxWidth:  maxImageCellWidth(),
		MaxHeight: maxImageCellHeight(),
	})
	if err != nil {
		debugImageProtocol("termimage.DisplayWithSize error: %v", err)
		return "", 1
	}
	if rows < 1 {
		rows = 1
	}
	debugImageProtocol("termimage: prerendered rows=%d bytes=%d", rows, buf.Len())
	return buf.String(), rows
}

// maxImageCellHeight returns the maximum number of terminal rows an inline
// image is allowed to occupy. It is always capped to a fraction of the viewport
// so images cannot monopolize the screen during scrolling.
func maxImageCellHeight() int {
	const defaultRows = 25
	_, rows, ok := getTerminalSize()
	if !ok || rows < 1 {
		return defaultRows
	}
	limit := rows * 8 / 10
	if limit < 1 {
		return 1
	}
	if limit > defaultRows {
		return limit
	}
	return defaultRows
}

// maxImageCellWidth returns the maximum number of terminal columns an inline
// image is allowed to occupy.
func maxImageCellWidth() int {
	const defaultCols = 80
	cols, _, ok := getTerminalSize()
	if !ok || cols < 1 {
		return defaultCols
	}
	if cols > 4 {
		cols -= 4
	}
	if cols < 1 {
		cols = 1
	}
	if cols > defaultCols {
		return cols
	}
	return defaultCols
}

// terminalSize caches the most recent terminal dimensions to avoid repeated
// syscalls. It is refreshed on demand if the dimensions are unknown.
var terminalSize struct {
	cols, rows int
	ok         bool
}

// getTerminalSize returns the current terminal size in columns and rows.
func getTerminalSize() (cols, rows int, ok bool) {
	if terminalSize.ok {
		return terminalSize.cols, terminalSize.rows, true
	}
	size, ok := terminalSizeFrom(os.Stdin)
	if !ok {
		size, ok = terminalSizeFrom(os.Stdout)
	}
	if !ok || size.cols < 1 || size.rows < 1 {
		return 0, 0, false
	}
	terminalSize.cols, terminalSize.rows, terminalSize.ok = size.cols, size.rows, true
	return size.cols, size.rows, true
}

type termSize struct {
	cols, rows int
}

// terminalSizeFrom attempts to read the terminal dimensions using the tty ioctl.
func terminalSizeFrom(f *os.File) (termSize, bool) {
	cols, rows, err := term.GetSize(int(f.Fd()))
	if err != nil || cols < 1 || rows < 1 {
		return termSize{}, false
	}
	return termSize{cols: cols, rows: rows}, true
}

// RenderImageToStdout writes an image directly to stdout at the given screen
// row using cursor positioning. This bypasses bubbletea's cell-based renderer
// which cannot handle graphics protocol escape sequences.
//
// The escape sequence and row count were captured at HTML processing time by
// prerenderImage, so this call is a plain stdout write with no decode.
func RenderImageToStdout(placement *ImagePlacement, screenRow int, screenCol ...int) {
	if placement.Encoded == "" {
		return
	}

	col := 1
	if len(screenCol) > 0 && screenCol[0] > 0 {
		col = screenCol[0]
	}

	debugImageProtocol("termimage: rendering %d bytes at row=%d col=%d", len(placement.Encoded), screenRow+1, col)
	fmt.Fprintf(os.Stdout, "\x1b[s\x1b[%d;%dH%s\x1b[u", //nolint:errcheck
		screenRow+1, col, placement.Encoded)
	os.Stdout.Sync() //nolint:errcheck,gosec
}

// expandImageRowPlaceholders replaces image row placeholders with actual newlines.
func expandImageRowPlaceholders(text string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(imageRowPlaceholderPrefix) + `(\d+)` + regexp.QuoteMeta(imageRowPlaceholderSuffix))
	return re.ReplaceAllStringFunc(text, func(match string) string {
		// Extract the number of rows from the placeholder
		numStr := strings.TrimPrefix(match, imageRowPlaceholderPrefix)
		numStr = strings.TrimSuffix(numStr, imageRowPlaceholderSuffix)
		rows := 1
		if _, err := fmt.Sscanf(numStr, "%d", &rows); err != nil || rows < 1 {
			rows = 1
		}
		// Return the newlines needed to push content below the image
		return strings.Repeat("\n", rows)
	})
}

type InlineImage struct {
	CID    string
	Base64 string
}

// ImagePlacement holds the data needed to render an image at a specific
// line in the email body. Images are rendered directly to stdout (bypassing
// bubbletea's cell-based renderer which cannot handle graphics protocols).
//
// Encoded and Rows are populated at HTML processing time by prerenderImage
// using termimage.DisplayWithSize, so paint-stage rendering is a plain
// stdout write and layout-stage row reservation matches the rendered output
// exactly.
type ImagePlacement struct {
	Line    int    // Line number in the processed body text where the image starts
	Rows    int    // Number of terminal rows the rendered image occupies (from termimage)
	Encoded string // Cached terminal escape sequence from termimage (rendered once at layout time)
}

// BodyMIMEType values understood by ProcessBody/ProcessBodyWithInline. Empty
// string means "unknown" — the renderer falls back to running markdownToHTML
// before HTML parsing, which is correct for plaintext-with-markdown bodies but
// can mangle complex HTML (e.g. tables with attribute-heavy <td style="...">).
const (
	BodyMIMETypeHTML  = "text/html"
	BodyMIMETypePlain = "text/plain"
)

// ProcessBodyWithInline renders the body and resolves CID inline images when provided.
// Returns the rendered body text, image placements for out-of-band rendering, and any error.
// mimeType is "text/html", "text/plain", or "" (unknown — falls back to legacy markdown→HTML pre-pass).
func ProcessBodyWithInline(rawBody, mimeType string, inline []InlineImage, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, []ImagePlacement, error) {
	inlineMap := make(map[string]string, len(inline))
	for _, img := range inline {
		cid := strings.TrimSpace(img.CID)
		cid = strings.TrimPrefix(cid, "<")
		cid = strings.TrimSuffix(cid, ">")
		cid = strings.TrimPrefix(cid, "cid:")
		if cid == "" || img.Base64 == "" {
			continue
		}
		inlineMap[cid] = img.Base64
	}
	return processBody(rawBody, mimeType, inlineMap, h1Style, h2Style, bodyStyle, disableImages)
}

// ProcessBody takes a raw email body, decodes it, and formats it as plain
// text with terminal hyperlinks.
// mimeType is "text/html", "text/plain", or "" (unknown — falls back to legacy markdown→HTML pre-pass).
func ProcessBody(rawBody, mimeType string, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, []ImagePlacement, error) {
	return processBody(rawBody, mimeType, nil, h1Style, h2Style, bodyStyle, disableImages)
}

func processBody(rawBody, mimeType string, inline map[string]string, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, []ImagePlacement, error) {
	decodedBody, err := decodeQuotedPrintable(rawBody)
	if err != nil {
		decodedBody = rawBody
	}

	// HTML bodies skip the markdown pre-pass — md4c can mangle attribute-heavy
	// or indented HTML (#602-style raw-tag bleed-through). Empty mimeType keeps
	// legacy behavior for cached/legacy callers that don't supply one.
	directHTML := mimeType == BodyMIMETypeHTML
	var htmlBody []byte
	if directHTML {
		htmlBody = []byte(decodedBody)
	} else {
		htmlBody = markdownToHTML([]byte(decodedBody))
	}
	htmlBody = htmlSanitizer.SanitizeBytes(htmlBody)

	result, placements, err := renderHTMLToText(htmlBody, inline, h1Style, h2Style, disableImages)
	if err != nil {
		return "", nil, err
	}

	// Some real-world HTML emails (newsletters with table-only layouts and no
	// <th>, AWeber-shape bodies) emit no visible content from htmlconv. Pre-
	// c11de45, every body went through markdownToHTML first, which happened to
	// keep these alive. Retry through the markdown pre-pass when the direct
	// HTML path produces nothing.
	if directHTML && strings.TrimSpace(result) == "" {
		fallbackHTML := htmlSanitizer.SanitizeBytes(markdownToHTML([]byte(decodedBody)))
		result, placements, err = renderHTMLToText(fallbackHTML, inline, h1Style, h2Style, disableImages)
		if err != nil {
			return "", nil, err
		}
	}

	result = styleQuotedReplies(result)
	return bodyStyle.Render(result), placements, nil
}

func renderHTMLToText(htmlBody []byte, inline map[string]string, h1Style, h2Style lipgloss.Style, disableImages bool) (string, []ImagePlacement, error) {
	// Parse HTML into structured elements using C parser.
	elements, ok := clib.HTMLToElements(string(htmlBody))
	if !ok {
		return "", nil, fmt.Errorf("could not parse email body")
	}

	// Process elements: apply styles and collect image placements.
	var text strings.Builder
	var imgIndex int
	var pendingImages []struct {
		index   int
		encoded string
		rows    int
	}

	onWroteRegex := regexp.MustCompile(`On\s+(.+?),\s+(.+?)\s+wrote:`)

	for _, elem := range elements {
		switch elem.Type {
		case clib.HElemText:
			text.WriteString(elem.Text)

		case clib.HElemH1:
			text.WriteString(h1Style.Render(elem.Text))
			text.WriteString("\n\n")

		case clib.HElemH2:
			text.WriteString(h2Style.Render(elem.Text))
			text.WriteString("\n\n")

		case clib.HElemLink:
			if hasTerminalControls(elem.Attr1) {
				text.WriteString(stripTerminalControls(elem.Text))
			} else {
				text.WriteString(hyperlink(elem.Attr1, elem.Text))
			}

		case clib.HElemImage:
			src := strings.TrimSpace(elem.Attr1)
			alt := stripTerminalControls(elem.Attr2)
			if hasTerminalControls(src) {
				continue
			}

			if !disableImages && imageProtocolSupported() {
				payload := resolveImagePayload(src, inline)

				if payload != "" {
					encoded, rows := prerenderImage(payload)
					if encoded == "" {
						debugImageProtocol("prerender failed for src=%s", src)
					} else {
						debugImageProtocol("collected image placement src=%s rows=%d", src, rows)

						idx := imgIndex
						imgIndex++
						pendingImages = append(pendingImages, struct {
							index   int
							encoded string
							rows    int
						}{idx, encoded, rows})

						fmt.Fprintf(&text, "\n[[MATCHA_IMG:%d]]", idx)
						fmt.Fprintf(&text, "\n%s%d%s\n", imageRowPlaceholderPrefix, rows, imageRowPlaceholderSuffix)
						continue
					}
				}
				debugImageProtocol("no payload for src=%s", src)
			}
			if isRemoteImageURL(src) && hyperlinkSupported() {
				fmt.Fprintf(&text, "\n %s \n", hyperlink(src, fmt.Sprintf("[Click here to view image: %s]", alt)))
			} else {
				fmt.Fprintf(&text, "\n %s \n", linkStyle().Render(fmt.Sprintf("[Image: %s, %s]", alt, src)))
			}

		case clib.HElemTable:
			headerRows := 0
			if elem.Attr1 != "" {
				fmt.Sscanf(elem.Attr1, "%d", &headerRows) //nolint:errcheck,gosec
			}
			text.WriteString("\n")
			text.WriteString(renderTable(elem.Text, headerRows))
			text.WriteString("\n")

		case clib.HElemBlockquote:
			var from, date string
			prevText := elem.Attr2
			cite := elem.Attr1

			if matches := onWroteRegex.FindStringSubmatch(prevText); matches != nil {
				date = parseDateForDisplay(matches[1])
				from = matches[2]
			} else if matches := onWroteRegex.FindStringSubmatch(cite); matches != nil {
				date = parseDateForDisplay(matches[1])
				from = matches[2]
			}

			text.WriteString(renderQuoteBox(from, date, strings.Split(elem.Text, "\n")))
		}
	}

	result := text.String()

	// Collapse excessive newlines, but not the image row placeholders
	re := regexp.MustCompile(`\n{3,}`)
	result = re.ReplaceAllString(result, "\n\n")

	// Now expand the image row placeholders to actual newlines
	result = expandImageRowPlaceholders(result)

	// Build image placements by finding the line numbers of image markers.
	var placements []ImagePlacement
	if len(pendingImages) > 0 {
		lines := strings.Split(result, "\n")
		imgMarkerRegex := regexp.MustCompile(`\[\[MATCHA_IMG:(\d+)\]\]`)
		for lineNum, line := range lines {
			if matches := imgMarkerRegex.FindStringSubmatch(line); matches != nil {
				var idx int
				fmt.Sscanf(matches[1], "%d", &idx) //nolint:errcheck,gosec
				for _, pi := range pendingImages {
					if pi.index == idx {
						placements = append(placements, ImagePlacement{
							Line:    lineNum,
							Encoded: pi.encoded,
							Rows:    pi.rows,
						})
						break
					}
				}
			}
		}

		// Remove the image markers from the text (leave the spacing)
		result = imgMarkerRegex.ReplaceAllString(result, "")
	}

	return result, placements, nil
}

func resolveImagePayload(src string, inline map[string]string) string {
	switch {
	case strings.HasPrefix(src, "data:image/"):
		return dataURIBase64(src)
	case strings.HasPrefix(src, "cid:"):
		cid := strings.TrimPrefix(src, "cid:")
		cid = strings.Trim(cid, "<>")
		if inline != nil {
			payload := inline[cid]
			debugImageProtocol("cid lookup for %s found=%t len=%d", cid, payload != "", len(payload))
			return payload
		}
		debugImageProtocol("cid lookup skipped inline map nil for %s", cid)
		return ""
	case strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://"):
		return fetchRemoteBase64(src)
	}
	return ""
}

func isRemoteImageURL(src string) bool {
	src = strings.ToLower(src)
	return strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")
}

func tableHeaderStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Accent)
}

func tableBorderStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.ActiveTheme.Secondary)
}

// renderTable renders table data as a Unicode box-drawing table.
// data is tab-separated cells, newline-separated rows.
// headerRows is the number of header rows.
func renderTable(data string, headerRows int) string {
	rows := strings.Split(data, "\n")
	if len(rows) == 0 {
		return ""
	}

	// Parse into 2D grid and trim cell whitespace
	var grid [][]string
	maxCols := 0
	for _, row := range rows {
		cells := strings.Split(row, "\t")
		trimmed := make([]string, len(cells))
		for i, c := range cells {
			trimmed[i] = strings.TrimSpace(c)
		}
		grid = append(grid, trimmed)
		if len(trimmed) > maxCols {
			maxCols = len(trimmed)
		}
	}

	// Normalize: ensure all rows have the same number of columns
	for i := range grid {
		for len(grid[i]) < maxCols {
			grid[i] = append(grid[i], "")
		}
	}

	// Calculate column widths
	colWidths := make([]int, maxCols)
	for _, row := range grid {
		for j, cell := range row {
			if len(cell) > colWidths[j] {
				colWidths[j] = len(cell)
			}
		}
	}

	// Minimum width per column
	for i := range colWidths {
		if colWidths[i] < 3 {
			colWidths[i] = 3
		}
	}

	bs := tableBorderStyle()
	hs := tableHeaderStyle()

	// Build horizontal borders
	buildBorder := func(left, mid, right, fill string) string {
		var b strings.Builder
		b.WriteString(bs.Render(left))
		for j, w := range colWidths {
			b.WriteString(bs.Render(strings.Repeat(fill, w+2)))
			if j < len(colWidths)-1 {
				b.WriteString(bs.Render(mid))
			}
		}
		b.WriteString(bs.Render(right))
		return b.String()
	}

	topBorder := buildBorder("┌", "┬", "┐", "─")
	midBorder := buildBorder("├", "┼", "┤", "─")
	botBorder := buildBorder("└", "┴", "┘", "─")

	var out strings.Builder
	out.WriteString(topBorder)
	out.WriteString("\n")

	for i, row := range grid {
		out.WriteString(bs.Render("│"))
		for j, cell := range row {
			padded := cell + strings.Repeat(" ", colWidths[j]-len(cell))
			if i < headerRows {
				out.WriteString(" " + hs.Render(padded) + " ")
			} else {
				out.WriteString(" " + padded + " ")
			}
			out.WriteString(bs.Render("│"))
		}
		out.WriteString("\n")

		if i < headerRows && (i+1 == headerRows || i+1 == len(grid)) {
			out.WriteString(midBorder)
			out.WriteString("\n")
		}
	}

	out.WriteString(botBorder)
	return out.String()
}

func quoteBoxStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ActiveTheme.Secondary).
		Padding(0, 1).
		Foreground(theme.ActiveTheme.Secondary)
}

func quoteHeaderStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.ActiveTheme.Secondary)
}

// styleQuotedReplies detects quoted reply sections and styles them in a box
func styleQuotedReplies(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	var quoteBlock []string
	var quoteFrom, quoteDate string
	inQuote := false

	// Regex to match "On DATE, EMAIL wrote:" pattern
	// Matches various date formats
	onWroteRegex := regexp.MustCompile(`^On\s+(.+?),\s+(.+?)\s+wrote:$`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmedLine := strings.TrimSpace(line)

		// Check for "On DATE, EMAIL wrote:" header
		if matches := onWroteRegex.FindStringSubmatch(trimmedLine); matches != nil {
			// If we were already in a quote block, render it first
			if inQuote && len(quoteBlock) > 0 {
				result = append(result, renderQuoteBox(quoteFrom, quoteDate, quoteBlock))
				quoteBlock = nil
			}

			// Parse the date and email from the match
			dateStr := matches[1]
			quoteFrom = matches[2]
			quoteDate = parseDateForDisplay(dateStr)
			inQuote = true
			continue
		}

		// Check if line starts with ">" (quoted text)
		if strings.HasPrefix(trimmedLine, ">") { //nolint:gocritic
			if !inQuote {
				// Start a new quote block without header info
				inQuote = true
				quoteFrom = ""
				quoteDate = ""
			}
			// Remove the leading "> " and add to quote block
			quotedContent := strings.TrimPrefix(trimmedLine, ">")
			quotedContent = strings.TrimPrefix(quotedContent, " ")
			quoteBlock = append(quoteBlock, quotedContent)
		} else if inQuote {
			// End of quote block - check if it's just whitespace
			if trimmedLine == "" && i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), ">") { //nolint:gocritic
				// Empty line within quote block, keep it
				quoteBlock = append(quoteBlock, "")
			} else if trimmedLine == "" && len(quoteBlock) == 0 {
				// Empty line before any quoted content, skip
				continue
			} else {
				// End of quote block
				if len(quoteBlock) > 0 {
					result = append(result, renderQuoteBox(quoteFrom, quoteDate, quoteBlock))
					quoteBlock = nil
				}
				inQuote = false
				quoteFrom = ""
				quoteDate = ""
				result = append(result, line)
			}
		} else {
			result = append(result, line)
		}
	}

	// Handle any remaining quote block
	if inQuote && len(quoteBlock) > 0 {
		result = append(result, renderQuoteBox(quoteFrom, quoteDate, quoteBlock))
	}

	return strings.Join(result, "\n")
}

// parseDateForDisplay converts various date formats to DD:MM:YY HH:MM
func parseDateForDisplay(dateStr string) string {
	// Common date formats to try
	formats := []string{
		"Jan 2, 2006 at 3:04 PM",
		"02:01:06 15:04",
		"2006-01-02 15:04:05",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05",
		"January 2, 2006 at 3:04 PM",
		"Jan 2, 2006 3:04 PM",
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t.Format("02:01:06 15:04")
		}
	}

	// Return original if parsing fails
	return dateStr
}

// renderQuoteBox renders a quoted section in a styled box
func renderQuoteBox(from, date string, lines []string) string {
	// Build header with email on left and date on right
	var header string
	if from != "" || date != "" {
		switch {
		case from != "" && date != "":
			header = quoteHeaderStyle().Render(from + "  " + date)
		case from != "":
			header = quoteHeaderStyle().Render(from)
		default:
			header = quoteHeaderStyle().Render(date)
		}
	}

	// Join the quoted content
	content := strings.Join(lines, "\n")

	// Build the box content
	var boxContent string
	if header != "" {
		boxContent = header + "\n\n" + content
	} else {
		boxContent = content
	}

	return quoteBoxStyle().Render(boxContent)
}
