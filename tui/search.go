package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/theme"
)

const (
	// searchBoxMaxWidth caps the floating search box (borders included).
	searchBoxMaxWidth = 72
	// searchMaxResults is the most results previewed in the box.
	searchMaxResults = 10
)

// SearchOverlay is the search prompt the inbox draws as a floating box. It is
// sized to fit inside width × height: lines are truncated rather than wrapped
// and the result preview shrinks to the rows available.
type SearchOverlay struct {
	input   textinput.Model
	query   backend.SearchQuery
	results []fetcher.Email
	loading bool
	done    bool
	// pending is the raw query of the search in flight; results for any other
	// query (the input changed meanwhile) are stale and dropped.
	pending string
	width   int
	height  int
}

func NewSearchOverlay(width, height int) *SearchOverlay {
	ti := textinput.New()
	ti.Placeholder = "from:alice subject:invoice since:2026-01-01"
	ti.Prompt = "/ "
	ti.CharLimit = 256
	ti.Focus()
	ti.SetStyles(ThemedTextInputStyles())
	o := &SearchOverlay{input: ti}
	o.setSize(width, height)
	return o
}

func (o *SearchOverlay) Init() tea.Cmd { return textinput.Blink }

// setSize records the area the box must fit in and sizes the input so a full
// line never wraps (the input draws its prompt plus one cursor cell).
func (o *SearchOverlay) setSize(width, height int) {
	o.width, o.height = width, height
	o.input.SetWidth(max(1, o.innerWidth()-lipgloss.Width(o.input.Prompt)-1))
}

func (o *SearchOverlay) Update(msg tea.Msg, mailbox MailboxKind, accountID string) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		o.setSize(msg.Width, msg.Height)
		return nil
	case SearchResultsMsg:
		if !o.loading || msg.Query.Raw != o.pending {
			return nil // stale: the query changed after this search started
		}
		o.loading, o.done, o.pending = false, msg.Err == nil, ""
		o.query = msg.Query
		if msg.Err != nil {
			return func() tea.Msg { return NotifyMsg{Message: msg.Err.Error()} }
		}
		o.results = msg.Emails
		return nil
	case tea.KeyPressMsg:
		if msg.String() == keyEnter {
			if o.loading {
				return nil
			}
			if o.done {
				results := append([]fetcher.Email(nil), o.results...)
				query := o.query
				return func() tea.Msg { return ApplySearchResultsMsg{Query: query, Emails: results} }
			}
			raw := o.input.Value()
			if raw == "" {
				return nil
			}
			o.loading, o.done, o.results, o.pending = true, false, nil, raw
			query := backend.ParseSearchQuery(raw)
			return func() tea.Msg { return SearchRequestedMsg{Query: query, Mailbox: mailbox, AccountID: accountID} }
		}
	}

	prev := o.input.Value()
	var cmd tea.Cmd
	o.input, cmd = o.input.Update(msg)
	if o.input.Value() != prev {
		// Results (or a search in flight) belong to the old query: Enter
		// must search again rather than apply them.
		o.loading, o.done, o.results, o.pending = false, false, nil, ""
		o.query = backend.SearchQuery{}
	}
	return cmd
}

// boxWidth is the total width of the box (borders included); it never
// exceeds the available width.
func (o *SearchOverlay) boxWidth() int {
	w := searchBoxMaxWidth
	if o.width > 0 {
		w = min(w, o.width)
	}
	return max(w, o.frame(true).GetHorizontalFrameSize()+1)
}

// innerWidth is the width available to the box content.
func (o *SearchOverlay) innerWidth() int {
	return o.boxWidth() - o.frame(true).GetHorizontalFrameSize()
}

// frame is the box border and padding; roomy adds vertical padding.
func (o *SearchOverlay) frame(roomy bool) lipgloss.Style {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ActiveTheme.Accent).
		Padding(0, 2)
	if roomy {
		style = style.Padding(1, 2)
	}
	return style
}

func (o *SearchOverlay) View() string {
	inner := o.innerWidth()
	lines := []string{
		ansi.Truncate("Search mail", inner, "…"),
		"",
		o.input.View(),
	}
	if o.loading {
		lines = append(lines, "", "Searching...")
	}
	if o.done {
		summary := fmt.Sprintf("%d result(s). Press Enter to apply, Esc to dismiss.", len(o.results))
		lines = append(lines, "")
		lines = append(lines, strings.Split(wrapBlock(summary, inner), "\n")...)
	}

	// Fit the box to the height: drop the vertical padding first, then show
	// only as many results as there are rows left.
	frame := o.frame(true)
	results := 0
	if o.done {
		results = min(len(o.results), searchMaxResults)
	}
	if o.height > 0 {
		if len(lines)+results+frame.GetVerticalFrameSize() > o.height {
			frame = o.frame(false)
		}
		results = max(0, min(results, o.height-frame.GetVerticalFrameSize()-len(lines)))
	}
	for i := range results {
		email := o.results[i]
		line := fmt.Sprintf("%d. %s - %s", i+1, parseSenderName(email.From), email.Subject)
		lines = append(lines, ansi.Truncate(singleRowText(line), inner, "…"))
	}
	// Last resort for very short areas: cut from the bottom.
	if o.height > 0 && len(lines) > o.height-frame.GetVerticalFrameSize() {
		lines = lines[:max(1, o.height-frame.GetVerticalFrameSize())]
	}

	return frame.Width(o.boxWidth()).Render(strings.Join(lines, "\n"))
}
