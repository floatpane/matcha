package view

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/clib"
	"github.com/floatpane/matcha/internal/htmlsanitizer"
	"github.com/floatpane/matcha/internal/loglevel"
	"github.com/floatpane/matcha/theme"
	"golang.org/x/term"
)

var htmlSanitizer htmlsanitizer.Sanitizer = htmlsanitizer.NewLibSanitizer()

const termGhostty = "ghostty"

func linkStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.ActiveTheme.Link)
}

func h3Style() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.ActiveTheme.Accent).
		Bold(true)
}

func h4Style() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.ActiveTheme.Accent).
		Bold(false)
}

func h5Style() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.ActiveTheme.Secondary).
		Bold(true)
}

func h6Style() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.ActiveTheme.Secondary).
		Bold(false).
		Italic(true)
}

func hrStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.ActiveTheme.Secondary)
}

func renderStyledText(s string, style int) string {
	if style == 0 || strings.TrimSpace(s) == "" {
		return s
	}
	st := lipgloss.NewStyle()
	if style&clib.HTMLStyleBold != 0 {
		st = st.Bold(true)
	}
	if style&clib.HTMLStyleItalic != 0 {
		st = st.Italic(true)
	}
	if style&clib.HTMLStyleUnderline != 0 {
		st = st.Underline(true)
	}
	if style&clib.HTMLStyleStrikethrough != 0 {
		st = st.Strikethrough(true)
	}
	return st.Render(s)
}

func renderListItem(itemText, depthStr, indexStr string) string {
	depth := 0
	if d, err := strconv.Atoi(depthStr); err == nil {
		depth = d
	}
	if depth < 0 {
		depth = 0
	}
	indent := strings.Repeat("  ", depth)
	var marker string
	if indexStr != "" {
		marker = indexStr + ". "
	} else {
		marker = "• "
	}
	lines := strings.Split(strings.TrimSpace(itemText), "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = indent + marker + line
		} else {
			lines[i] = indent + strings.Repeat(" ", len(marker)) + line
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderHR() string {
	width := 40
	if cols, _, ok := getTerminalSize(); ok && cols > 0 {
		width = cols
	}
	if width > 60 {
		width = 60
	}
	return "\n" + hrStyle().Render(strings.Repeat("─", width)) + "\n"
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

// markdownToHTML converts a Markdown string to an HTML string using md4c (C).
// Single newlines are kept as line breaks: in email bodies they are part of
// the message (signatures, addresses, short lines), not soft wraps.
func markdownToHTML(md []byte) []byte {
	return clib.MarkdownToHTMLHardBreaks(md)
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
		weztermSupported() || waystSupported() || warpSupported() || konsoleSupported() ||
		(inTmux() && tmuxClientHasPlaceholders())
}

func debugImageProtocol(format string, args ...interface{}) {
	if os.Getenv("DEBUG_IMAGE_PROTOCOL") == "" && os.Getenv("DEBUG_KITTY_IMAGES") == "" {
		return
	}
	msg := fmt.Sprintf("[img-protocol] "+format+"\n", args...)
	loglevel.Infof("%s", strings.TrimSuffix(msg, "\n"))
	if path := os.Getenv("DEBUG_IMAGE_PROTOCOL_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			if _, err := f.WriteString(msg); err != nil {
				loglevel.Debugf("image protocol write error: %v", err)
			}
			if err := f.Close(); err != nil {
				loglevel.Debugf("image protocol close error: %v", err)
			}
		}
	} else if path := os.Getenv("DEBUG_KITTY_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			if _, err := f.WriteString(msg); err != nil {
				loglevel.Debugf("image protocol write error: %v", err)
			}
			if err := f.Close(); err != nil {
				loglevel.Debugf("image protocol close error: %v", err)
			}
		}
	}
}

func dataURIBase64(uri string) string {
	if len(uri) < len("data:") || !strings.EqualFold(uri[:len("data:")], "data:") {
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

// maxImageCellHeight returns the default maximum number of terminal rows an
// inline image may occupy when the caller does not pass explicit bounds: 80%
// of the terminal height, so the image can always be seen in full.
func maxImageCellHeight() int {
	const defaultRows = 25
	_, rows, ok := getTerminalSize()
	if !ok || rows < 1 {
		return defaultRows
	}
	return max(1, rows*8/10)
}

// maxImageCellWidth returns the default maximum number of terminal columns
// an inline image may occupy when the caller does not pass explicit bounds.
func maxImageCellWidth() int {
	const defaultCols = 80
	cols, _, ok := getTerminalSize()
	if !ok || cols < 1 {
		return defaultCols
	}
	if cols > 4 {
		cols -= 4
	}
	return max(1, cols)
}

// terminalSize overrides the detected terminal size when ok is set (tests).
var terminalSize struct {
	cols, rows int
	ok         bool
}

// getTerminalSize returns the current terminal size in columns and rows. The
// size is queried on every call so that it follows window resizes.
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

// ImagePlacement describes an inline image that is painted out-of-band
// (Kitty or Sixel) at a line of the rendered body. Half-block images are part
// of the body text and therefore never produce a placement.
type ImagePlacement struct {
	Line     int           // Line in the rendered body where the image starts
	Rows     int           // Number of terminal rows the image occupies
	Cols     int           // Number of terminal columns the image occupies
	Encoded  string        // Kitty transmit sequence or Sixel sequence
	ID       uint32        // Kitty image id (0 for Sixel)
	Protocol ImageProtocol // Protocol Encoded is written in
	PixelW   int           // Pixel size of the encoded image
	PixelH   int
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
	return ProcessBodyWithInlineRemote(rawBody, mimeType, inline, h1Style, h2Style, bodyStyle, disableImages, false)
}

// ProcessBodyWithInlineRemote renders embedded images and, only when allowed,
// fetches remote images referenced by the email. The permission is scoped to
// the caller's current message view.
func ProcessBodyWithInlineRemote(rawBody, mimeType string, inline []InlineImage, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages, allowRemoteImages bool) (string, []ImagePlacement, error) {
	rendered, err := RenderBody(rawBody, mimeType, inline, h1Style, h2Style, bodyStyle, RenderOptions{
		DisableImages:     disableImages,
		AllowRemoteImages: allowRemoteImages,
	})
	if err != nil {
		return "", nil, err
	}
	text, placements := rendered.Finalize(rendered.Text)
	return text, placements, nil
}

// ProcessBody takes a raw email body, decodes it, and formats it as plain
// text with terminal hyperlinks.
// mimeType is "text/html", "text/plain", or "" (unknown — falls back to legacy markdown→HTML pre-pass).
func ProcessBody(rawBody, mimeType string, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, []ImagePlacement, error) {
	return ProcessBodyWithInlineRemote(rawBody, mimeType, nil, h1Style, h2Style, bodyStyle, disableImages, false)
}

// RenderOptions controls how RenderBody treats images.
type RenderOptions struct {
	DisableImages     bool
	AllowRemoteImages bool
	// SkipRemoteImages lists remote image URLs not to download, such as the
	// ones that failed in an earlier render of the same view. They count as
	// failed.
	SkipRemoteImages map[string]bool
	// Bounds limits inline images, in cells. Zero values derive limits from
	// the terminal size; views should pass their own viewport size.
	Bounds ImageBounds
}

// RenderedBody is a processed email body that still carries image markers.
// Wrap Text to the display width first and then call Finalize, so image
// positions are computed against the final line layout.
type RenderedBody struct {
	Text   string
	images map[int]*preparedImage
	// RemoteImages counts the distinct remote images the body refers to;
	// FailedRemoteImages lists the URLs of those a render with
	// AllowRemoteImages could not show.
	RemoteImages       int
	FailedRemoteImages []string
}

// HasImages reports whether the body contains rendered inline images.
func (r *RenderedBody) HasImages() bool {
	return r != nil && len(r.images) > 0
}

// RenderBody renders rawBody to styled terminal text, keeping image markers
// in place. See RenderedBody.Finalize.
func RenderBody(rawBody, mimeType string, inline []InlineImage, h1Style, h2Style, bodyStyle lipgloss.Style, opts RenderOptions) (*RenderedBody, error) {
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
	return processBody(rawBody, mimeType, inlineMap, h1Style, h2Style, bodyStyle, opts)
}

// Finalize removes the image markers from text, which must be derived from
// r.Text (for example by word-wrapping it). Half-block images are drawn into
// the text; Kitty and Sixel images are returned as placements whose Line is
// relative to the returned text.
func (r *RenderedBody) Finalize(text string) (string, []ImagePlacement) {
	if r == nil || !strings.Contains(text, imageMarkerPrefix) {
		return text, nil
	}
	lines := strings.Split(text, "\n")
	var placements []ImagePlacement
	for i := 0; i < len(lines); i++ {
		match := imageMarkerRegex.FindStringSubmatchIndex(lines[i])
		if match == nil {
			continue
		}
		idx, _ := strconv.Atoi(lines[i][match[2]:match[3]])
		lines[i] = lines[i][:match[0]] + lines[i][match[1]:]
		img := r.images[idx]
		if img == nil {
			continue
		}
		switch img.protocol {
		case ImageProtocolHalfBlock:
			for j, row := range strings.Split(img.data, "\n") {
				if i+j >= len(lines) {
					lines = append(lines, row)
					continue
				}
				lines[i+j] = row
			}
		case ImageProtocolKittyPlaceholder:
			// The cells are text; the placement only carries the upload.
			for j, row := range kittyPlaceholderRows(img.id, img.cols, img.rows) {
				if i+j >= len(lines) {
					lines = append(lines, row)
					continue
				}
				lines[i+j] = row
			}
			fallthrough
		case ImageProtocolKitty, ImageProtocolSixel:
			placements = append(placements, ImagePlacement{
				Line:     i,
				Rows:     img.rows,
				Cols:     img.cols,
				Encoded:  img.data,
				ID:       img.id,
				Protocol: img.protocol,
				PixelW:   img.pixelW,
				PixelH:   img.pixelH,
			})
		case ImageProtocolNone:
		}
	}
	return strings.Join(lines, "\n"), placements
}

func processBody(rawBody, mimeType string, inline map[string]string, h1Style, h2Style, bodyStyle lipgloss.Style, opts RenderOptions) (*RenderedBody, error) {
	// Bodies arrive already transfer-decoded: the IMAP fetcher decodes each
	// part per its BODYSTRUCTURE encoding, go-message decodes Maildir, POP3
	// and raw messages, and JMAP returns decoded text. Decoding
	// quoted-printable again would turn every "=XX" in the content into a
	// raw byte, corrupting text and URLs ("?w=600" becomes "?w`0").
	decodedBody := rawBody

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

	result, images, remote, err := renderHTMLToText(htmlBody, inline, h1Style, h2Style, opts)
	if err != nil {
		return nil, err
	}

	// Some real-world HTML emails (newsletters with table-only layouts and no
	// <th>, AWeber-shape bodies) emit no visible content from htmlconv. Pre-
	// c11de45, every body went through markdownToHTML first, which happened to
	// keep these alive. Retry through the markdown pre-pass when the direct
	// HTML path produces nothing.
	if directHTML && strings.TrimSpace(result) == "" {
		fallbackHTML := htmlSanitizer.SanitizeBytes(markdownToHTML([]byte(decodedBody)))
		result, images, remote, err = renderHTMLToText(fallbackHTML, inline, h1Style, h2Style, opts)
		if err != nil {
			return nil, err
		}
	}

	result = styleQuotedReplies(result)
	return &RenderedBody{
		Text:               bodyStyle.Render(result),
		images:             images,
		RemoteImages:       remote.requested,
		FailedRemoteImages: remote.failedURLs(),
	}, nil
}

// imageMarkerPrefix starts the marker line that records where an inline
// image begins. Markers survive wrapping and are removed by Finalize.
const imageMarkerPrefix = "[[MATCHA_IMG:"

var imageMarkerRegex = regexp.MustCompile(`\[\[MATCHA_IMG:(\d+)\]\]`)

// remoteImageSet holds the remote images of one render: the downloaded
// payloads and the URLs that were wanted but could not be shown.
type remoteImageSet struct {
	payloads  map[string]string
	requested int
	failed    map[string]bool
}

func (r *remoteImageSet) fail(src string) {
	if r.failed == nil {
		r.failed = make(map[string]bool)
	}
	r.failed[src] = true
}

func (r *remoteImageSet) failedURLs() []string {
	urls := make([]string, 0, len(r.failed))
	for u := range r.failed {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	return urls
}

// remoteImageSource returns the remote URL an image element would load, or
// "" when it is not a loadable remote image.
func remoteImageSource(elem clib.HTMLElement) string {
	src := strings.TrimSpace(elem.Attr1)
	if hasTerminalControls(src) || !isRemoteImageURL(src) {
		return ""
	}
	return src
}

// remoteImageURLs returns the distinct remote image URLs elements show.
func remoteImageURLs(elements []clib.HTMLElement) []string {
	seen := make(map[string]bool)
	var urls []string
	for _, elem := range elements {
		if elem.Type != clib.HElemImage {
			continue
		}
		if src := remoteImageSource(elem); src != "" && !seen[src] {
			seen[src] = true
			urls = append(urls, src)
		}
	}
	return urls
}

// loadRemoteImages downloads, in parallel, the remote images elements show.
func loadRemoteImages(elements []clib.HTMLElement, skip map[string]bool) remoteImageSet {
	urls := remoteImageURLs(elements)
	set := remoteImageSet{requested: len(urls)}
	fetch := urls[:0:0]
	for _, src := range urls {
		if skip[src] {
			set.fail(src)
			continue
		}
		fetch = append(fetch, src)
	}
	set.payloads = fetchRemoteImages(fetch)
	return set
}

func renderHTMLToText(htmlBody []byte, inline map[string]string, h1Style, h2Style lipgloss.Style, opts RenderOptions) (string, map[int]*preparedImage, remoteImageSet, error) {
	// Parse HTML into structured elements using C parser.
	elements, ok := clib.HTMLToElements(string(htmlBody))
	if !ok {
		return "", nil, remoteImageSet{}, fmt.Errorf("could not parse email body")
	}

	// Process elements: apply styles and collect image markers.
	var text strings.Builder
	images := make(map[int]*preparedImage)
	protocol := ImageProtocolNone
	if !opts.DisableImages {
		protocol = ActiveImageProtocol()
	}
	var remote remoteImageSet
	if protocol != ImageProtocolNone && opts.AllowRemoteImages {
		remote = loadRemoteImages(elements, opts.SkipRemoteImages)
	} else {
		remote.requested = len(remoteImageURLs(elements))
	}

	onWroteRegex := regexp.MustCompile(`On\s+(.+?),\s+(.+?)\s+wrote:`)

	for _, elem := range elements {
		switch elem.Type {
		case clib.HElemText:
			text.WriteString(renderStyledText(elem.Text, elem.Style))

		case clib.HElemH1:
			text.WriteString(h1Style.Render(elem.Text))
			text.WriteString("\n\n")

		case clib.HElemH2:
			text.WriteString(h2Style.Render(elem.Text))
			text.WriteString("\n\n")

		case clib.HElemH3:
			text.WriteString(h3Style().Render(elem.Text))
			text.WriteString("\n\n")

		case clib.HElemH4:
			text.WriteString(h4Style().Render(elem.Text))
			text.WriteString("\n\n")

		case clib.HElemH5:
			text.WriteString(h5Style().Render(elem.Text))
			text.WriteString("\n\n")

		case clib.HElemH6:
			text.WriteString(h6Style().Render(elem.Text))
			text.WriteString("\n\n")

		case clib.HElemLink:
			if hasTerminalControls(elem.Attr1) {
				text.WriteString(stripTerminalControls(elem.Text))
			} else {
				text.WriteString(hyperlink(elem.Attr1, elem.Text))
			}

		case clib.HElemImage:
			renderImageElement(&text, images, elem, inline, &remote, protocol, opts)

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
			prevText := stripTerminalControls(elem.Attr2)
			cite := stripTerminalControls(elem.Attr1)

			if matches := onWroteRegex.FindStringSubmatch(prevText); matches != nil {
				date = parseDateForDisplay(matches[1])
				from = matches[2]
			} else if matches := onWroteRegex.FindStringSubmatch(cite); matches != nil {
				date = parseDateForDisplay(matches[1])
				from = matches[2]
			}

			text.WriteString(renderQuoteBox(from, date, strings.Split(elem.Text, "\n")))

		case clib.HElemCode:
			text.WriteString(renderCodeBlock(elem.Text, elem.Attr1))

		case clib.HElemListItem:
			text.WriteString(renderListItem(elem.Text, elem.Attr1, elem.Attr2))

		case clib.HElemHR:
			text.WriteString(renderHR())
		}
	}

	result := text.String()

	// Collapse excessive newlines, but not the image row placeholders
	re := regexp.MustCompile(`\n{3,}`)
	result = re.ReplaceAllString(result, "\n\n")

	// Now expand the image row placeholders to actual newlines
	result = expandImageRowPlaceholders(result)

	return result, images, remote, nil
}

func renderImageElement(text *strings.Builder, images map[int]*preparedImage, elem clib.HTMLElement, inline map[string]string, remote *remoteImageSet, protocol ImageProtocol, opts RenderOptions) {
	src := strings.TrimSpace(elem.Attr1)
	alt := stripTerminalControls(elem.Attr2)
	if hasTerminalControls(src) {
		return
	}

	isRemote := isRemoteImageURL(src)
	if protocol != ImageProtocolNone && (opts.AllowRemoteImages || !isRemote) {
		var payload string
		if isRemote {
			payload = remote.payloads[src]
		} else {
			payload = resolveImagePayload(src, inline)
		}

		if payload != "" {
			img, err := prepareImage(payload, protocol, opts.Bounds)
			if err != nil {
				debugImageProtocol("prepare failed for src=%s: %v", src, err)
			} else {
				debugImageProtocol("collected image src=%s protocol=%s cols=%d rows=%d", src, protocol, img.cols, img.rows)

				idx := len(images)
				images[idx] = img

				// The marker line is the image's first row; the placeholder
				// expands to the remaining rows plus one blank spacer line.
				fmt.Fprintf(text, "\n%s%d]]", imageMarkerPrefix, idx)
				fmt.Fprintf(text, "\n%s%d%s\n", imageRowPlaceholderPrefix, img.rows, imageRowPlaceholderSuffix)
				return
			}
		}
		debugImageProtocol("no payload for src=%s", src)
		if isRemote {
			remote.fail(src)
		}
	}
	if isRemote && hyperlinkSupported() {
		fmt.Fprintf(text, "\n %s \n", hyperlink(src, fmt.Sprintf("[Click here to view image: %s]", alt)))
	} else {
		fmt.Fprintf(text, "\n %s \n", linkStyle().Render(fmt.Sprintf("[Image: %s, %s]", alt, imageSourceLabel(src))))
	}
}

// imageSourceLabel describes an image source for the text fallback. Inline
// data: URIs carry the whole image as base64, which must never be printed
// into the body, so only their media type is shown; long URLs are shortened.
func imageSourceLabel(src string) string {
	if strings.HasPrefix(strings.ToLower(src), "data:") {
		rest := src[len("data:"):]
		if end := strings.IndexAny(rest, ";,"); end >= 0 {
			rest = rest[:end]
		}
		return "data:" + ansi.Truncate(rest, 40, "…")
	}
	const maxSourceLen = 80
	if len(src) > maxSourceLen {
		return ansi.Truncate(src, maxSourceLen, "…")
	}
	return src
}

// resolveImagePayload returns the base64 payload of an image embedded in
// the email (data: URI or cid: reference). Remote images are downloaded by
// loadRemoteImages.
func resolveImagePayload(src string, inline map[string]string) string {
	// URI schemes are case-insensitive. Only the prefix is lowered: data:
	// URIs can be megabytes long.
	lower := strings.ToLower(src[:min(len(src), len("data:image/"))])
	switch {
	case strings.HasPrefix(lower, "data:image/"):
		return dataURIBase64(src)
	case strings.HasPrefix(lower, "cid:"):
		cid := src[len("cid:"):]
		cid = strings.Trim(cid, "<>")
		if inline != nil {
			payload := inline[cid]
			debugImageProtocol("cid lookup for %s found=%t len=%d", cid, payload != "", len(payload))
			return payload
		}
		debugImageProtocol("cid lookup skipped inline map nil for %s", cid)
		return ""
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

func codeBoxStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ActiveTheme.Secondary).
		Padding(0, 1)
}

func codeLangStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.ActiveTheme.AccentText).
		Background(theme.ActiveTheme.AccentDark).
		Padding(0, 1)
}

// renderCodeBlock renders a <pre> code block as a styled terminal box with
// optional language label and syntax highlighting. lang is the language hint
// extracted from a class="language-XXX" attribute (may be empty).
func renderCodeBlock(code, lang string) string {
	// Trim leading/trailing blank lines while preserving internal indentation.
	code = strings.TrimRight(code, "\n")
	code = strings.TrimLeft(code, "\n")
	if code == "" {
		return ""
	}

	highlighted := highlightCode(code, lang)

	var label string
	if lang != "" {
		label = codeLangStyle().Render(" " + strings.ToUpper(lang) + " ")
	}

	var content string
	if label != "" {
		content = label + "\n" + highlighted
	} else {
		content = highlighted
	}

	return "\n" + codeBoxStyle().Render(content) + "\n"
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
