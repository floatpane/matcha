package view

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime/quotedprintable"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/clib"
	"github.com/floatpane/matcha/internal/httpclient"
	"github.com/floatpane/matcha/theme"
	lru "github.com/hashicorp/golang-lru/v2"
)

func linkStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.ActiveTheme.Link)
}

// getTerminalCellSize returns the height of a terminal cell in pixels.
// It queries the terminal using TIOCGWINSZ to get both character and pixel dimensions.
// Falls back to a default of 18 pixels if the query fails.
func getTerminalCellSize() int {
	const defaultCellHeight = 18

	// Try stdout, stdin, stderr, then /dev/tty as last resort
	fds := []int{int(os.Stdout.Fd()), int(os.Stdin.Fd()), int(os.Stderr.Fd())}

	for _, fd := range fds {
		if cellHeight := getCellHeightFromFd(fd); cellHeight > 0 {
			return cellHeight
		}
	}

	// Try /dev/tty directly - this works even when stdio is redirected (e.g., in Bubble Tea)
	if tty, err := os.Open("/dev/tty"); err == nil {
		defer tty.Close()
		if cellHeight := getCellHeightFromFd(int(tty.Fd())); cellHeight > 0 {
			return cellHeight
		}
	}

	debugImageProtocol("using default cell height: %d pixels", defaultCellHeight)
	return defaultCellHeight
}

// hyperlinkSupported checks if the terminal supports OSC 8 hyperlinks.
func hyperlinkSupported() bool {
	term := strings.ToLower(os.Getenv("TERM"))

	// Terminals known to support OSC 8 hyperlinks
	supportedTerms := []string{
		"kitty",
		"ghostty",
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
		"ghostty",
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
	if text == "" {
		text = url
	}

	supported := hyperlinkSupported()

	if supported {
		// Use OSC 8 hyperlink sequence for supported terminals
		return fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, linkStyle().Render(text))
	} else {
		// Fallback to plain text format for unsupported terminals
		if text == url {
			return fmt.Sprintf("<%s>", linkStyle().Render(url))
		}
		return fmt.Sprintf("%s <%s>", linkStyle().Render(text), linkStyle().Render(url))
	}
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
	if strings.Contains(term, "ghostty") {
		return true
	}

	// Check for Ghostty-specific environment variables
	if os.Getenv("TERM_PROGRAM") == "ghostty" {
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
	if strings.Contains(term, "wezterm") {
		return true
	}

	return false
}

func waystSupported() bool {
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "wayst") {
		return true
	}

	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	if termProgram == "wayst" {
		return true
	}

	return false
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
	if termProgram == "konsole" {
		return true
	}

	return false
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
	fmt.Print(msg)
	if path := os.Getenv("DEBUG_IMAGE_PROTOCOL_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			_, _ = f.WriteString(msg)
			_ = f.Close()
		}
	} else if path := os.Getenv("DEBUG_KITTY_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			_, _ = f.WriteString(msg)
			_ = f.Close()
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

// nextImageID is an auto-incrementing counter for Kitty image IDs.
var nextImageID uint32 = 1000

// allocImageID returns a unique Kitty image ID.
func allocImageID() uint32 {
	return atomic.AddUint32(&nextImageID, 1)
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
	defer resp.Body.Close()
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

func kittyInlineImage(payload string) string {
	if payload == "" {
		return ""
	}

	const chunkSize = 4096
	var b strings.Builder

	// Calculate how many terminal rows the image occupies to advance text after it.
	rows := 1
	if data, err := base64.StdEncoding.DecodeString(payload); err == nil {
		if _, h, ok := clib.ImageDimensions(data); ok {
			cellHeight := getTerminalCellSize()
			rows = (h + cellHeight - 1) / cellHeight
			if rows < 1 {
				rows = 1
			}
			debugImageProtocol("image height: %d pixels, cell height: %d pixels, rows needed: %d", h, cellHeight, rows)
		}
	}

	for offset := 0; offset < len(payload); offset += chunkSize {
		end := offset + chunkSize
		if end > len(payload) {
			end = len(payload)
		}
		more := "0"
		if end < len(payload) {
			more = "1"
		}

		chunk := payload[offset:end]
		if offset == 0 {
			// C=1 means cursor does NOT move after image render (stays at top-left of image position)
			// This is needed for proper TUI rendering, but we must add newlines to push text below
			b.WriteString(fmt.Sprintf("\x1b_Gf=100,a=T,q=2,C=1,m=%s;%s\x1b\\", more, chunk))
		} else {
			b.WriteString(fmt.Sprintf("\x1b_Gm=%s;%s\x1b\\", more, chunk))
		}
	}

	// Add newlines to push cursor below the image.
	// Use a placeholder that won't be collapsed by the newline regex.
	b.WriteString(fmt.Sprintf("\n%s%d%s\n", imageRowPlaceholderPrefix, rows, imageRowPlaceholderSuffix))

	return b.String()
}

// iterm2InlineImage renders an image using iTerm2's image protocol
func iterm2InlineImage(payload string) string {
	if payload == "" {
		return ""
	}

	// Calculate rows for cursor positioning
	rows := 1
	if data, err := base64.StdEncoding.DecodeString(payload); err == nil {
		if _, h, ok := clib.ImageDimensions(data); ok {
			cellHeight := getTerminalCellSize()
			rows = (h + cellHeight - 1) / cellHeight
			if rows < 1 {
				rows = 1
			}
			debugImageProtocol("image height: %d pixels, cell height: %d pixels, rows needed: %d", h, cellHeight, rows)
		}
	}

	// iTerm2 image protocol: ESC]1337;File=inline=1:<base64_data>BEL
	result := fmt.Sprintf("\x1b]1337;File=inline=1:%s\x07\n", payload)

	// Add placeholder for row spacing
	result += fmt.Sprintf("%s%d%s\n", imageRowPlaceholderPrefix, rows, imageRowPlaceholderSuffix)

	return result
}

// sixelInlineImage returns Sixel escape sequence + newline placeholders
func sixelInlineImage(base64PNG string) string {
	data, err := base64.StdEncoding.DecodeString(base64PNG)
	if err != nil {
		return ""
	}

	cellHeight := getTerminalCellSize()
	sixel, rows, err := clib.EncodePNGToSixel(data, cellHeight)
	if err != nil {
		debugImageProtocol("Sixel encoding failed: %v", err)
		return ""
	}

	debugImageProtocol("Sixel: encoded %d bytes, %d rows", len(sixel), rows)

	// Sixel sequences don't auto-advance cursor
	// Add newlines to preserve layout
	return sixel + strings.Repeat("\n", rows)
}

// sixelImageEscapeOnly returns raw Sixel for out-of-band rendering
func sixelImageEscapeOnly(base64PNG string) string {
	data, err := base64.StdEncoding.DecodeString(base64PNG)
	if err != nil {
		return ""
	}

	cellHeight := getTerminalCellSize()
	sixel, _, err := clib.EncodePNGToSixel(data, cellHeight)
	if err != nil {
		return ""
	}

	return sixel
}

// renderInlineImage renders an image using the appropriate protocol for the detected terminal
func renderInlineImage(payload string) string {
	if payload == "" {
		return ""
	}

	// Priority: Sixel in multiplexers overrides native protocols
	if sixelSupported() {
		return sixelInlineImage(payload)
	}

	if kittySupported() || ghosttySupported() || weztermSupported() || waystSupported() || konsoleSupported() {
		// These terminals use the Kitty graphics protocol
		return kittyInlineImage(payload)
	} else if iterm2Supported() || warpSupported() {
		// iTerm2 and Warp use the iTerm2 image protocol
		return iterm2InlineImage(payload)
	}

	return ""
}

// imageRows calculates the number of terminal rows an image occupies.
func imageRows(payload string) int {
	rows := 1
	if data, err := base64.StdEncoding.DecodeString(payload); err == nil {
		if _, h, ok := clib.ImageDimensions(data); ok {
			cellHeight := getTerminalCellSize()
			if cellHeight == 0 {
				cellHeight = 16
			}
			rows = (h + cellHeight - 1) / cellHeight
			if rows < 1 {
				rows = 1
			}
			debugImageProtocol("image height: %d pixels, cell height: %d pixels, rows needed: %d", h, cellHeight, rows)
		}
	}
	return rows
}

// kittyUploadImage uploads image data to the terminal with a unique ID using
// the Kitty graphics protocol transmit action (a=t). The image is stored in
// the terminal's memory and can be displayed later by ID without re-sending data.
func kittyUploadImage(payload string, id uint32) {
	if payload == "" {
		return
	}

	const chunkSize = 4096
	for offset := 0; offset < len(payload); offset += chunkSize {
		end := offset + chunkSize
		if end > len(payload) {
			end = len(payload)
		}
		more := "0"
		if end < len(payload) {
			more = "1"
		}

		chunk := payload[offset:end]
		if offset == 0 {
			// a=t: transmit (upload) only, don't display yet
			// i=ID: assign this image ID
			fmt.Fprintf(os.Stdout, "\x1b_Gf=100,a=t,i=%d,q=2,m=%s;%s\x1b\\", id, more, chunk)
		} else {
			fmt.Fprintf(os.Stdout, "\x1b_Gm=%s;%s\x1b\\", more, chunk)
		}
	}
	os.Stdout.Sync()
}

// kittyDisplayImage displays a previously uploaded image by its ID at the
// current cursor position. This is very fast since no image data is transmitted.
func kittyDisplayImage(id uint32) string {
	// a=p: put (display) an already-uploaded image by ID
	// C=1: cursor does not move
	return fmt.Sprintf("\x1b_Ga=p,i=%d,q=2,C=1\x1b\\", id)
}

// iterm2ImageEscapeOnly returns only the iTerm2 image protocol escape sequence
// without any row placeholders. Used for out-of-band rendering to stdout.
func iterm2ImageEscapeOnly(payload string) string {
	if payload == "" {
		return ""
	}
	return fmt.Sprintf("\x1b]1337;File=inline=1:%s\x07", payload)
}

// RenderImageToStdout writes an image directly to stdout at the given screen
// row using cursor positioning. This bypasses bubbletea's cell-based renderer
// which cannot handle graphics protocol escape sequences.
//
// For Kitty-protocol terminals, images are uploaded once and then displayed by
// ID on subsequent calls, making scroll rendering nearly instant.
func RenderImageToStdout(placement *ImagePlacement, screenRow int, screenCol ...int) {
	if placement.Base64 == "" {
		return
	}

	col := 1
	if len(screenCol) > 0 && screenCol[0] > 0 {
		col = screenCol[0]
	}

	// Priority: Sixel in multiplexers
	if sixelSupported() {
		debugImageProtocol("Sixel: RenderImageToStdout row=%d col=%d base64len=%d", screenRow, col, len(placement.Base64))

		// Encode once, reuse cached Sixel on subsequent renders (like Kitty's upload-once pattern)
		if placement.SixelEncoded == "" {
			placement.SixelEncoded = sixelImageEscapeOnly(placement.Base64)
			if placement.SixelEncoded == "" {
				debugImageProtocol("Sixel: sixelImageEscapeOnly returned empty")
				return
			}
		}

		debugImageProtocol("Sixel: rendering %d bytes at row=%d col=%d", len(placement.SixelEncoded), screenRow+1, col)
		// Position cursor + render Sixel
		fmt.Fprintf(os.Stdout, "\x1b[s\x1b[%d;%dH%s\x1b[u",
			screenRow+1, col, placement.SixelEncoded)
		os.Stdout.Sync()
		return
	}

	useKitty := kittySupported() || ghosttySupported() || weztermSupported() || waystSupported() || konsoleSupported()
	useIterm2 := iterm2Supported() || warpSupported()

	if useKitty {
		// Upload once, display by ID on subsequent renders
		if !placement.Uploaded {
			placement.ID = allocImageID()
			kittyUploadImage(placement.Base64, placement.ID)
			placement.Uploaded = true
		}
		seq := kittyDisplayImage(placement.ID)
		fmt.Fprintf(os.Stdout, "\x1b[s\x1b[%d;%dH%s\x1b[u", screenRow+1, col, seq)
		os.Stdout.Sync()
	} else if useIterm2 {
		seq := iterm2ImageEscapeOnly(placement.Base64)
		fmt.Fprintf(os.Stdout, "\x1b[s\x1b[%d;%dH%s\x1b[u", screenRow+1, col, seq)
		os.Stdout.Sync()
	}
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
type ImagePlacement struct {
	Line         int    // Line number in the processed body text where the image starts
	Base64       string // Base64-encoded image data (PNG)
	Rows         int    // Number of terminal rows the image occupies
	Uploaded     bool   // Whether the image has been uploaded to the terminal via Kitty ID
	ID           uint32 // Kitty image ID for display-by-reference
	SixelEncoded string // Cached Sixel escape sequence (encode once, reuse on scroll)
}

// ProcessBodyWithInline renders the body and resolves CID inline images when provided.
// Returns the rendered body text, image placements for out-of-band rendering, and any error.
func ProcessBodyWithInline(rawBody string, inline []InlineImage, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, []ImagePlacement, error) {
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
	return processBody(rawBody, inlineMap, h1Style, h2Style, bodyStyle, disableImages)
}

// ProcessBody takes a raw email body, decodes it, and formats it as plain
// text with terminal hyperlinks.
func ProcessBody(rawBody string, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, []ImagePlacement, error) {
	return processBody(rawBody, nil, h1Style, h2Style, bodyStyle, disableImages)
}

func processBody(rawBody string, inline map[string]string, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, []ImagePlacement, error) {
	decodedBody, err := decodeQuotedPrintable(rawBody)
	if err != nil {
		decodedBody = rawBody
	}

	htmlBody := markdownToHTML([]byte(decodedBody))

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
		payload string
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
			text.WriteString(hyperlink(elem.Attr1, elem.Text))

		case clib.HElemImage:
			src := elem.Attr1
			alt := elem.Attr2

			if !disableImages && imageProtocolSupported() {
				var payload string
				if strings.HasPrefix(src, "data:image/") {
					payload = dataURIBase64(src)
				} else if strings.HasPrefix(src, "cid:") {
					cid := strings.TrimPrefix(src, "cid:")
					cid = strings.Trim(cid, "<>")
					if inline != nil {
						payload = inline[cid]
						debugImageProtocol("cid lookup for %s found=%t len=%d", cid, payload != "", len(payload))
					} else {
						debugImageProtocol("cid lookup skipped inline map nil for %s", cid)
					}
				} else if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
					payload = fetchRemoteBase64(src)
				}

				if payload != "" {
					rows := imageRows(payload)
					debugImageProtocol("collected image placement src=%s rows=%d", src, rows)

					idx := imgIndex
					imgIndex++
					pendingImages = append(pendingImages, struct {
						index   int
						payload string
						rows    int
					}{idx, payload, rows})

					text.WriteString(fmt.Sprintf("\n[[MATCHA_IMG:%d]]", idx))
					text.WriteString(fmt.Sprintf("\n%s%d%s\n", imageRowPlaceholderPrefix, rows, imageRowPlaceholderSuffix))
					continue
				}
				debugImageProtocol("no payload for src=%s", src)
			}
			if hyperlinkSupported() {
				text.WriteString(hyperlink(src, fmt.Sprintf("\n [Click here to view image: %s] \n", alt)))
			} else {
				text.WriteString(fmt.Sprintf("\n %s \n", linkStyle().Render(fmt.Sprintf("[Image: %s, %s]", alt, src))))
			}

		case clib.HElemTable:
			headerRows := 0
			if elem.Attr1 != "" {
				fmt.Sscanf(elem.Attr1, "%d", &headerRows)
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
				fmt.Sscanf(matches[1], "%d", &idx)
				for _, pi := range pendingImages {
					if pi.index == idx {
						placements = append(placements, ImagePlacement{
							Line:   lineNum,
							Base64: pi.payload,
							Rows:   pi.rows,
						})
						break
					}
				}
			}
		}

		// Remove the image markers from the text (leave the spacing)
		result = imgMarkerRegex.ReplaceAllString(result, "")
	}

	// Style quoted reply sections (for plain text > quotes)
	result = styleQuotedReplies(result)

	return bodyStyle.Render(result), placements, nil
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
		if strings.HasPrefix(trimmedLine, ">") {
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
			if trimmedLine == "" && i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), ">") {
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
		if from != "" && date != "" {
			header = quoteHeaderStyle().Render(from + "  " + date)
		} else if from != "" {
			header = quoteHeaderStyle().Render(from)
		} else {
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
