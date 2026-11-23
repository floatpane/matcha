package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/theme"
)

// draftItem represents a draft in the list
type draftItem struct {
	draft config.Draft
}

func (i draftItem) Title() string {
	if i.draft.Subject != "" {
		return i.draft.Subject
	}
	return "(No subject)"
}

func (i draftItem) Description() string {
	to := i.draft.To
	if to == "" {
		to = "(No recipient)"
	}
	timeAgo := formatTimeAgo(i.draft.UpdatedAt)
	return fmt.Sprintf("To: %s • %s", to, timeAgo)
}

func (i draftItem) FilterValue() string {
	return i.draft.Subject + " " + i.draft.To + " " + i.draft.Body
}

// formatTimeAgo returns a human-readable time difference
func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Local().Format("Jan 2, 2006")
	}
}

// draftsDoubleClick is the longest gap between two clicks on the same draft
// that still counts as a double click.
const draftsDoubleClick = 500 * time.Millisecond

// Drafts is the model for the drafts list view
type Drafts struct {
	list          list.Model
	delegate      list.DefaultDelegate
	drafts        []config.Draft
	width         int
	height        int
	confirmDelete bool
	selectedDraft *config.Draft
	lastClickTime time.Time
	lastClickIdx  int
}

// NewDrafts creates a new drafts list view
func NewDrafts(drafts []config.Draft) *Drafts {
	items := make([]list.Item, len(drafts))
	for i, d := range drafts {
		items[i] = draftItem{draft: d}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Drafts"
	l.Styles.Title = lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent).Bold(true)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetStatusBarItemName("draft", "drafts")
	l.AdditionalShortHelpKeys = func() []key.Binding {
		kb := config.Keybinds
		open := kbKey(kb.Drafts.Open, "enter")
		del := kbKey(kb.Drafts.Delete, "d")
		return []key.Binding{
			key.NewBinding(key.WithKeys(open), key.WithHelp("\ue5fe "+open, "open")),
			key.NewBinding(key.WithKeys(del), key.WithHelp("\uea81 "+del, "delete")),
		}
	}
	l.KeyMap.Quit.SetEnabled(false)

	return &Drafts{
		list:         l,
		delegate:     delegate,
		drafts:       drafts,
		lastClickIdx: -1,
	}
}

func (m *Drafts) Init() tea.Cmd {
	return nil
}

// listHeaderHeight is the number of rows the list draws above its first
// item, measured from the list's own title and status bar styles.
func (m *Drafts) listHeaderHeight() int {
	h := 0
	if m.list.ShowTitle() || (m.list.ShowFilter() && m.list.FilteringEnabled()) {
		h += lipgloss.Height(m.list.Styles.TitleBar.Render(m.list.Title))
	}
	if m.list.ShowStatusBar() {
		h += lipgloss.Height(m.list.Styles.StatusBar.Render("status"))
	}
	return h
}

// itemAt maps a screen row to the index (among the visible, possibly
// filtered, items) of the draft drawn there. Rows between items, below the
// last item on the page, or on the pagination and help lines map to nothing.
func (m *Drafts) itemAt(y int) (int, bool) {
	row := y - m.listHeaderHeight()
	if row < 0 {
		return 0, false
	}
	itemH := m.delegate.Height()
	slot, within := row/(itemH+m.delegate.Spacing()), row%(itemH+m.delegate.Spacing())
	if within >= itemH {
		return 0, false
	}
	total := len(m.list.VisibleItems())
	if slot >= m.list.Paginator.ItemsOnPage(total) {
		return 0, false
	}
	start, _ := m.list.Paginator.GetSliceBounds(total)
	return start + slot, true
}

// handleClick selects the clicked draft; a double click opens it.
func (m *Drafts) handleClick(msg tea.MouseClickMsg) tea.Cmd {
	if msg.Button != tea.MouseLeft || m.confirmDelete || m.list.FilterState() == list.Filtering {
		return nil
	}
	idx, ok := m.itemAt(msg.Y)
	if !ok {
		return nil
	}
	m.list.Select(idx)
	now := time.Now()
	isDoubleClick := idx == m.lastClickIdx && now.Sub(m.lastClickTime) < draftsDoubleClick
	m.lastClickTime = now
	m.lastClickIdx = idx
	if !isDoubleClick {
		return nil
	}
	m.lastClickIdx = -1 // a third click starts a new pair
	if draftItm, ok := m.list.SelectedItem().(draftItem); ok {
		d := draftItm.draft
		return func() tea.Msg { return OpenDraftMsg{Draft: d} }
	}
	return nil
}

func (m *Drafts) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		if m.confirmDelete {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseWheelDown:
			m.list.CursorDown()
		case tea.MouseWheelUp:
			m.list.CursorUp()
		default:
		}
		return m, nil

	case tea.MouseClickMsg:
		return m, m.handleClick(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 4)
		return m, nil

	case tea.KeyPressMsg:
		// Handle delete confirmation
		if m.confirmDelete {
			switch msg.String() {
			case "y", "Y":
				if m.selectedDraft != nil {
					draftID := m.selectedDraft.ID
					m.confirmDelete = false
					m.selectedDraft = nil
					return m, func() tea.Msg {
						return DeleteSavedDraftMsg{DraftID: draftID}
					}
				}
			case "n", "N", config.Keybinds.Global.Cancel:
				m.confirmDelete = false
				m.selectedDraft = nil
			}
			return m, nil
		}

		// Skip key handling during filtering
		if m.list.FilterState() == list.Filtering {
			break
		}

		kb := config.Keybinds
		switch msg.String() {
		case kb.Global.Cancel:
			// With a filter applied, the first cancel clears the filter;
			// only an unfiltered list leaves Drafts.
			if m.list.FilterState() == list.FilterApplied {
				m.list.ResetFilter()
				return m, nil
			}
			return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
		case kb.Drafts.Open:
			if item, ok := m.list.SelectedItem().(draftItem); ok {
				return m, func() tea.Msg {
					return OpenDraftMsg{Draft: item.draft}
				}
			}
		case kb.Drafts.Delete:
			if item, ok := m.list.SelectedItem().(draftItem); ok {
				m.confirmDelete = true
				m.selectedDraft = &item.draft
				return m, nil
			}
		}

	case DraftDeletedMsg:
		if msg.Err == nil {
			// Remove the deleted draft from the list
			var newDrafts []config.Draft
			for _, d := range m.drafts {
				if d.ID != msg.DraftID {
					newDrafts = append(newDrafts, d)
				}
			}
			m.drafts = newDrafts

			items := make([]list.Item, len(m.drafts))
			for i, d := range m.drafts {
				items[i] = draftItem{draft: d}
			}
			m.list.SetItems(items)
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *Drafts) View() tea.View {
	var b strings.Builder

	var v tea.View
	switch {
	case m.confirmDelete:
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				"Delete this draft?",
				HelpStyle.Render("\n(y/n)"),
			),
		)
		v = tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog))
	case len(m.drafts) == 0:
		emptyMsg := lipgloss.NewStyle().
			Foreground(theme.ActiveTheme.Secondary).
			Render("No drafts saved.\n\nPress esc to go back.")
		v = tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, emptyMsg))
	default:
		// list.View() still returns string in v2
		b.WriteString(m.list.View())
		v = tea.NewView(b.String())
	}
	if config.MouseEnabled != nil && *config.MouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// SetDrafts updates the drafts list
func (m *Drafts) SetDrafts(drafts []config.Draft) {
	m.drafts = drafts
	items := make([]list.Item, len(drafts))
	for i, d := range drafts {
		items[i] = draftItem{draft: d}
	}
	m.list.SetItems(items)
}

// IsFilterActive reports whether the list filter is being typed or applied.
func (m *Drafts) IsFilterActive() bool {
	return m != nil && (m.list.FilterState() == list.Filtering || m.list.FilterState() == list.FilterApplied)
}
