package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/theme"
)

type SearchOverlay struct {
	input   textinput.Model
	query   backend.SearchQuery
	results []fetcher.Email
	loading bool
	done    bool
	err     string
	width   int
}

func NewSearchOverlay(width, height int) *SearchOverlay {
	ti := textinput.New()
	ti.Placeholder = "from:alice subject:invoice since:2026-01-01"
	ti.Prompt = "/ "
	ti.CharLimit = 256
	ti.Focus()
	ti.SetStyles(ThemedTextInputStyles())
	if width < 44 {
		width = 44
	}
	return &SearchOverlay{input: ti, width: width}
}

func (o *SearchOverlay) Init() tea.Cmd { return textinput.Blink }

func (o *SearchOverlay) Update(msg tea.Msg, mailbox MailboxKind, accountID string) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		o.width = msg.Width
		return nil
	case SearchResultsMsg:
		o.loading, o.done, o.err = false, msg.Err == nil, ""
		o.query = msg.Query
		if msg.Err != nil {
			o.err = msg.Err.Error()
			return nil
		}
		o.results = msg.Emails
		return nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
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
			o.loading, o.done, o.err, o.results = true, false, "", nil
			query := backend.ParseSearchQuery(raw)
			return func() tea.Msg { return SearchRequestedMsg{Query: query, Mailbox: mailbox, AccountID: accountID} }
		}
	}

	var cmd tea.Cmd
	o.input, cmd = o.input.Update(msg)
	return cmd
}

func (o *SearchOverlay) View() string {
	boxWidth := o.width - 4
	if boxWidth < 40 {
		boxWidth = 40
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ActiveTheme.Accent).Padding(1, 2).Width(boxWidth)
	content := "Search mail\n\n" + o.input.View()
	if o.loading {
		content += "\n\nSearching..."
	}
	if o.err != "" {
		content += "\n\n" + lipgloss.NewStyle().Foreground(theme.ActiveTheme.Danger).Render(o.err)
	}
	if o.done {
		content += fmt.Sprintf("\n\n%d result(s). Press Enter to apply, Esc to dismiss.\n", len(o.results))
		content += o.resultsView()
	}
	return style.Render(content)
}

func (o *SearchOverlay) resultsView() string {
	limit := len(o.results)
	if limit > 10 {
		limit = 10
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		email := o.results[i]
		fmt.Fprintf(&b, "%d. %s - %s\n", i+1, parseSenderName(email.From), email.Subject)
	}
	return b.String()
}
