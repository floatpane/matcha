package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	overlay "github.com/floatpane/bubble-overlay"
	"github.com/floatpane/matcha/theme"
)

// paletteMaxVisible caps how many command rows are drawn at once; the visible
// window scrolls to keep the cursor in view.
const paletteMaxVisible = 8

// PaletteCommand is a single selectable entry in the command palette.
type PaletteCommand struct {
	// Title is the human-readable label shown in the list.
	Title string
	// Hint is an optional keybind shown dimmed on the right (e.g. "r").
	Hint string
	// Keywords are extra, hidden search terms used for matching only.
	Keywords string
	// Action produces the message dispatched when the command is chosen.
	Action func() tea.Msg
}

// CommandPalette is a modal overlay that fuzzy-filters a list of commands,
// modeled after the Zed / VS Code command palette. It is owned and driven by
// the top-level model, which decides when to open it and feeds it key events.
type CommandPalette struct {
	input    textinput.Model
	all      []PaletteCommand
	filtered []int // indices into all, in display order
	cursor   int   // index into filtered
	top      int   // first visible row (index into filtered)
	width    int
	height   int
}

// NewCommandPalette builds a palette over the given commands sized to the
// available terminal width and height.
func NewCommandPalette(commands []PaletteCommand, width, height int) *CommandPalette {
	ti := textinput.New()
	ti.Placeholder = "Type a command…"
	ti.Prompt = "> "
	ti.CharLimit = 128
	ti.Focus()
	ti.SetStyles(ThemedTextInputStyles())

	p := &CommandPalette{
		input:  ti,
		all:    commands,
		width:  width,
		height: height,
	}
	p.filter()
	return p
}

func (p *CommandPalette) Init() tea.Cmd { return textinput.Blink }

// SetSize updates the dimensions used to center and size the overlay.
func (p *CommandPalette) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// Update feeds an event to the palette. List navigation is handled here; any
// other key is forwarded to the text input and re-filters the list. Selection
// and dismissal are handled by the owner via SelectedCmd.
func (p *CommandPalette) Update(msg tea.Msg) tea.Cmd {
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		switch wheel.Button {
		case tea.MouseWheelDown:
			p.moveCursor(1)
		case tea.MouseWheelUp:
			p.moveCursor(-1)
		}
		return nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "up", "ctrl+p", "ctrl+k":
			p.moveCursor(-1)
			return nil
		case keyDown, "ctrl+n", "ctrl+j":
			p.moveCursor(1)
			return nil
		}
	}

	prev := p.input.Value()
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	if p.input.Value() != prev {
		p.filter()
	}
	return cmd
}

// SelectedCmd returns the action command for the highlighted entry, or nil if
// the list is empty.
func (p *CommandPalette) SelectedCmd() tea.Cmd {
	if p.cursor < 0 || p.cursor >= len(p.filtered) {
		return nil
	}
	action := p.all[p.filtered[p.cursor]].Action
	if action == nil {
		return nil
	}
	return func() tea.Msg { return action() }
}

func (p *CommandPalette) moveCursor(delta int) {
	if len(p.filtered) == 0 {
		return
	}
	p.cursor = (p.cursor + delta + len(p.filtered)) % len(p.filtered)
	switch {
	case p.cursor < p.top:
		p.top = p.cursor
	case p.cursor >= p.top+paletteMaxVisible:
		p.top = p.cursor - paletteMaxVisible + 1
	}
}

// filter rebuilds the visible list from the current query using a
// case-insensitive subsequence match against each command's title and
// keywords. An empty query shows everything in its original order.
func (p *CommandPalette) filter() {
	query := strings.ToLower(strings.TrimSpace(p.input.Value()))
	p.filtered = p.filtered[:0]
	for i, c := range p.all {
		if query == "" || subsequenceMatch(strings.ToLower(c.Title+" "+c.Keywords), query) {
			p.filtered = append(p.filtered, i)
		}
	}
	p.cursor = 0
	p.top = 0
}

// subsequenceMatch reports whether every rune of query appears in haystack in
// order (not necessarily contiguously) — the classic fuzzy-finder match.
func subsequenceMatch(haystack, query string) bool {
	if query == "" {
		return true
	}
	qr := []rune(query)
	qi := 0
	for _, hc := range haystack {
		if hc == qr[qi] {
			qi++
			if qi == len(qr) {
				return true
			}
		}
	}
	return false
}

// boxWidth returns the inner width available for command rows.
func (p *CommandPalette) boxWidth() int {
	w := p.width - 8
	if w > 64 {
		w = 64
	}
	if w < 32 {
		w = 32
	}
	return w
}

// View renders the palette box on its own (without the background).
func (p *CommandPalette) View() string {
	t := theme.ActiveTheme
	inner := p.boxWidth()
	p.input.SetWidth(inner - lipgloss.Width(p.input.Prompt))

	var b strings.Builder
	b.WriteString(p.input.View())
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(t.DimText).Render(strings.Repeat("─", inner)))
	b.WriteString("\n")

	if len(p.filtered) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(t.MutedText).Render("No matching commands"))
	} else {
		end := p.top + paletteMaxVisible
		if end > len(p.filtered) {
			end = len(p.filtered)
		}
		for i := p.top; i < end; i++ {
			b.WriteString(p.renderRow(p.all[p.filtered[i]], i == p.cursor, inner))
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(t.SubtleText).Render("↑/↓ navigate · enter run · esc close"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Padding(1, 2).
		Render(b.String())
}

func (p *CommandPalette) renderRow(cmd PaletteCommand, selected bool, inner int) string {
	t := theme.ActiveTheme
	title := cmd.Title
	prefix := "  "
	titleStyle := lipgloss.NewStyle().Foreground(t.DimText)
	if selected {
		prefix = "> "
		titleStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	}

	rendered := titleStyle.Render(prefix + title)
	if cmd.Hint == "" {
		return rendered
	}
	hint := lipgloss.NewStyle().Foreground(t.MutedText).Render(cmd.Hint)
	gap := inner - lipgloss.Width(prefix+title) - lipgloss.Width(cmd.Hint)
	if gap < 1 {
		gap = 1
	}
	return rendered + strings.Repeat(" ", gap) + hint
}

// Render composites the palette box as a floating layer centered over the given
// background using overlay.Center from github.com/floatpane/bubble-overlay.
// The background is preserved on all sides; only the box's own cells are replaced.
func (p *CommandPalette) Render(background string, screenW, screenH int) string {
	return overlay.Center(background, p.View(), screenW, screenH)
}
