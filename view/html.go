package view

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/quotedprintable"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	_ "image/gif"
	_ "image/jpeg"

	"charm.land/lipgloss/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
	"golang.org/x/sys/unix"
)

var linkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9BC4FF")) // Cyan

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

// getCellHeightFromFd attempts to get the terminal cell height from a file descriptor.
// Returns 0 if it fails or if pixel dimensions are not available.
func getCellHeightFromFd(fd int) int {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 0
	}

	// ws.Row = number of character rows
	// ws.Ypixel = height in pixels
	// Some terminals don't report pixel dimensions (return 0)
	if ws.Row > 0 && ws.Ypixel > 0 {
		cellHeight := int(ws.Ypixel) / int(ws.Row)
		if cellHeight > 0 {
			debugImageProtocol("terminal cell height: %d pixels (rows=%d, ypixel=%d, fd=%d)", cellHeight, ws.Row, ws.Ypixel, fd)
			return cellHeight
		}
	}

	// Terminal reported dimensions but no pixel info - this is common
	if ws.Row > 0 && ws.Ypixel == 0 {
		debugImageProtocol("terminal fd=%d has rows=%d but no pixel info (ypixel=0)", fd, ws.Row)
	}

	return 0
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
		os.Getenv("WEZTERM_EXECUTABLE") != "" {
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
		return fmt.Sprintf("\x1b]8;;%s\x07%s\x1b]8;;\x07", url, linkStyle.Render(text))
	} else {
		// Fallback to plain text format for unsupported terminals
		// Use HTML-encoded angle brackets to prevent HTML parser from treating them as tags
		if text == url {
			return fmt.Sprintf("&lt;%s&gt;", linkStyle.Render(url))
		}
		return fmt.Sprintf("%s &lt;%s&gt;", linkStyle.Render(text), linkStyle.Render(url))
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

// markdownToHTML converts a Markdown string to an HTML string.
func markdownToHTML(md []byte) []byte {
	var buf bytes.Buffer
	p := goldmark.New(
		goldmark.WithRendererOptions(
			html.WithUnsafe(), // Allow raw HTML in email.
		),
	)
	if err := p.Convert(md, &buf); err != nil {
		return md // Fallback to original markdown.
	}
	return buf.Bytes()
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

// ImageProtocolSupported checks if any supported image protocol terminal is detected.
func ImageProtocolSupported() bool {
	return imageProtocolSupported()
}

// imageProtocolSupported checks if any supported image protocol terminal is detected.
func imageProtocolSupported() bool {
	return kittySupported() || ghosttySupported() || iterm2Supported() ||
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

func fetchRemoteBase64(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ""
	}
	client := &http.Client{Timeout: 5 * time.Second}
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
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		debugImageProtocol("remote fetch read error url=%s err=%v", url, err)
		return ""
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		debugImageProtocol("remote decode failed url=%s err=%v", url, err)
		return ""
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		debugImageProtocol("remote png encode failed url=%s err=%v", url, err)
		return ""
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	debugImageProtocol("remote fetch ok url=%s len=%d", url, len(encoded))
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
		if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			cellHeight := getTerminalCellSize()
			h := img.Bounds().Dy()
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
		if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			cellHeight := getTerminalCellSize()
			h := img.Bounds().Dy()
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

// renderInlineImage renders an image using the appropriate protocol for the detected terminal
func renderInlineImage(payload string) string {
	if payload == "" {
		return ""
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

// ProcessBodyWithInline renders the body and resolves CID inline images when provided.
func ProcessBodyWithInline(rawBody string, inline []InlineImage, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, error) {
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
func ProcessBody(rawBody string, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, error) {
	return processBody(rawBody, nil, h1Style, h2Style, bodyStyle, disableImages)
}

func processBody(rawBody string, inline map[string]string, h1Style, h2Style, bodyStyle lipgloss.Style, disableImages bool) (string, error) {
	decodedBody, err := decodeQuotedPrintable(rawBody)
	if err != nil {
		decodedBody = rawBody
	}

	htmlBody := markdownToHTML([]byte(decodedBody))

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlBody))
	if err != nil {
		return "", fmt.Errorf("could not parse email body: %w", err)
	}

	doc.Find("style, script").Remove()

	// Style headers by setting their text content.
	// We use SetText so the h1/h2 tags remain in the document for spacing logic.
	doc.Find("h1").Each(func(i int, s *goquery.Selection) {
		s.SetText(h1Style.Render(s.Text()))
	})

	doc.Find("h2").Each(func(i int, s *goquery.Selection) {
		s.SetText(h2Style.Render(s.Text()))
	})

	// Add newlines after block elements for better spacing.
	doc.Find("p, div, h1, h2").Each(func(i int, s *goquery.Selection) {
		s.After("\n\n")
	})

	// Replace <br> tags with newlines
	doc.Find("br").Each(func(i int, s *goquery.Selection) {
		s.ReplaceWithHtml("\n")
	})

	// Handle blockquote elements (quoted replies)
	// We collect quote data and use placeholders, then render after doc.Text()
	var quoteData []struct {
		from, date string
		content    string
	}
	doc.Find("blockquote").Each(func(i int, s *goquery.Selection) {
		// Try to extract sender info from cite attribute or preceding text
		cite, _ := s.Attr("cite")
		quoteText := strings.TrimSpace(s.Text())

		// Look for "On DATE, EMAIL wrote:" pattern in previous sibling or cite
		var from, date string
		prevText := ""
		if prev := s.Prev(); prev.Length() > 0 {
			prevText = strings.TrimSpace(prev.Text())
		}

		onWroteRegex := regexp.MustCompile(`On\s+(.+?),\s+(.+?)\s+wrote:`)
		if matches := onWroteRegex.FindStringSubmatch(prevText); matches != nil {
			date = parseDateForDisplay(matches[1])
			from = matches[2]
			// Remove the "On ... wrote:" from the previous element
			s.Prev().Remove()
		} else if matches := onWroteRegex.FindStringSubmatch(cite); matches != nil {
			date = parseDateForDisplay(matches[1])
			from = matches[2]
		}

		// Store quote data and use placeholder
		quoteData = append(quoteData, struct {
			from, date string
			content    string
		}{from, date, quoteText})
		placeholder := fmt.Sprintf("\n[[MATCHA_QUOTE:%d]]\n", len(quoteData)-1)
		s.ReplaceWithHtml(placeholder)
	})

	// Format links and images
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		s.ReplaceWithHtml(hyperlink(href, s.Text()))
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
				if rendered := renderInlineImage(payload); rendered != "" {
					debugImageProtocol("rendered inline image src=%s len=%d dataURI=%t cid=%t (kitty=%t ghostty=%t iterm2=%t wezterm=%t wayst=%t warp=%t konsole=%t)", src, len(payload), strings.HasPrefix(src, "data:"), strings.HasPrefix(src, "cid:"), kittySupported(), ghosttySupported(), iterm2Supported(), weztermSupported(), waystSupported(), warpSupported(), konsoleSupported())
					s.ReplaceWithHtml("\n" + rendered + "\n")
					return
				}
				debugImageProtocol("payload present but renderer returned empty src=%s len=%d", src, len(payload))
			} else {
				debugImageProtocol("no payload for src=%s dataURI=%t cid=%t", src, strings.HasPrefix(src, "data:"), strings.HasPrefix(src, "cid:"))
			}
		} else {
			debugImageProtocol("image protocol not supported for src=%s (kitty=%t ghostty=%t iterm2=%t wezterm=%t wayst=%t warp=%t konsole=%t)", src, kittySupported(), ghosttySupported(), iterm2Supported(), weztermSupported(), waystSupported(), warpSupported(), konsoleSupported())
		}
		if hyperlinkSupported() {
			s.ReplaceWithHtml(hyperlink(src, fmt.Sprintf("\n [Click here to view image: %s] \n", alt)))
		} else {
			s.ReplaceWithHtml(fmt.Sprintf("\n %s \n", linkStyle.Render(fmt.Sprintf("[Image: %s, %s]", alt, src))))
		}
	})

	text := doc.Text()

	// Collapse excessive newlines, but not the image row placeholders
	re := regexp.MustCompile(`\n{3,}`)
	text = re.ReplaceAllString(text, "\n\n")

	// Now expand the image row placeholders to actual newlines
	text = expandImageRowPlaceholders(text)

	// Replace quote placeholders with styled quote boxes
	quoteRegex := regexp.MustCompile(`\[\[MATCHA_QUOTE:(\d+)\]\]`)
	text = quoteRegex.ReplaceAllStringFunc(text, func(match string) string {
		idxStr := quoteRegex.FindStringSubmatch(match)[1]
		var idx int
		fmt.Sscanf(idxStr, "%d", &idx)
		if idx >= 0 && idx < len(quoteData) {
			q := quoteData[idx]
			return renderQuoteBox(q.from, q.date, strings.Split(q.content, "\n"))
		}
		return match
	})

	// Style quoted reply sections (for plain text > quotes)
	text = styleQuotedReplies(text)

	return bodyStyle.Render(text), nil
}

// quoteBoxStyle is the style for the quoted reply box border
var quoteBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("240")).
	Padding(0, 1).
	Foreground(lipgloss.Color("240"))

// quoteHeaderStyle is the style for the header line in the quote box
var quoteHeaderStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("240"))

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
			header = quoteHeaderStyle.Render(from + "  " + date)
		} else if from != "" {
			header = quoteHeaderStyle.Render(from)
		} else {
			header = quoteHeaderStyle.Render(date)
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

	return quoteBoxStyle.Render(boxContent)
}
