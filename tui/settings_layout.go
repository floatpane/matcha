package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// settingsMenuWidth is the total width of the left category pane,
	// including its right padding and border.
	settingsMenuWidth = 30
	// settingsMenuPadRight is the gap between the category labels and the
	// menu's right border.
	settingsMenuPadRight = 2
	// settingsContentPad is the gap between the menu border and the content.
	settingsContentPad = 2
	// settingsMinBodyRows is how many body rows a page keeps visible before
	// optional extras (tips, previews) are dropped to make room.
	settingsMinBodyRows = 5
)

// settingsPage is one settings content pane split into a pinned header, a
// body that scrolls to keep the focused rows visible, an optional extra block
// (tips, previews) that is dropped first when space runs out, and a pinned
// footer (help, errors). Every body entry is rendered as exactly one line.
type settingsPage struct {
	// key identifies the page's scroll state; pages sharing a key share it.
	key    string
	header string
	body   []string
	// focusTop..focusBot is the body range kept on screen (-1: none).
	focusTop int
	focusBot int
	extra    string
	footer   string
	// rows reports that body line i is list entry i, so clicks on the body
	// select entries. Forms leave it false.
	rows bool
}

// pageLayout records where a rendered page put its body so mouse events can
// be mapped back to body rows.
type pageLayout struct {
	bodyTop int // row of the first visible body line, relative to the content top
	start   int // index of the first visible body line
	end     int // one past the index of the last visible body line
}

// blockHeight counts the lines of a rendered block ("" has none).
func blockHeight(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// clipLines keeps at most n lines of s (n <= 0 keeps everything).
func clipLines(s string, n int) string {
	if n <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}

// wrapBlock word-wraps s to width cells (width <= 0 leaves s untouched).
func wrapBlock(s string, width int) string {
	if s == "" || width <= 0 {
		return s
	}
	return lipgloss.NewStyle().Width(width).Render(s)
}

// layoutPage renders p into at most maxH lines of width cells (0 = no limit)
// and reports where the visible body rows ended up.
func (m *Settings) layoutPage(p settingsPage, width, maxH int) (string, pageLayout) {
	header := wrapBlock(p.header, width)
	extra := wrapBlock(p.extra, width)
	footer := wrapBlock(p.footer, width)
	body := p.body
	if width > 0 {
		body = make([]string, len(p.body))
		for i, line := range p.body {
			body[i] = ansi.Truncate(line, width, "…")
		}
	}

	limit := maxH
	if limit <= 0 {
		limit = blockHeight(header) + len(body) + blockHeight(extra) + blockHeight(footer)
	}
	avail := limit - blockHeight(header) - blockHeight(footer)
	if extra != "" {
		if avail-blockHeight(extra) >= min(len(body), settingsMinBodyRows) {
			avail -= blockHeight(extra)
		} else {
			extra = ""
		}
	}
	// The body wins over the footer on tiny screens: the final clip below
	// cuts the footer instead.
	if avail < 1 && len(body) > 0 {
		avail = 1
	}

	start := m.scrollOffset(p, len(body), avail)
	end := min(len(body), start+avail)

	sections := make([]string, 0, 4)
	if header != "" {
		sections = append(sections, header)
	}
	if end > start {
		sections = append(sections, strings.Join(body[start:end], "\n"))
	}
	if extra != "" {
		sections = append(sections, extra)
	}
	if footer != "" {
		sections = append(sections, footer)
	}
	out := clipLines(strings.Join(sections, "\n"), limit)
	return out, pageLayout{bodyTop: blockHeight(header), start: start, end: end}
}

// scrollOffset returns the first visible body line of p for a window of
// avail rows, moving the page's stored offset only as far as needed to keep
// the focused range on screen.
func (m *Settings) scrollOffset(p settingsPage, n, avail int) int {
	if m.scroll == nil {
		m.scroll = make(map[string]int)
	}
	off := m.scroll[p.key]
	if avail <= 0 || n <= avail {
		off = 0
	} else {
		top, bot := p.focusTop, p.focusBot
		if top >= 0 {
			if bot < top {
				bot = top
			}
			if bot-top+1 > avail {
				bot = top + avail - 1
			}
			if bot >= off+avail {
				off = bot - avail + 1
			}
			if top < off {
				off = top
			}
		}
		off = max(0, min(off, n-avail))
	}
	m.scroll[p.key] = off
	return off
}
