package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/floatpane/matcha/config"
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
		return t.Format("Jan 2, 2006")
	}
}

// Drafts is the model for the drafts list view
type Drafts struct {
	list          list.Model
	drafts        []config.Draft
	width         int
	height        int
	confirmDelete bool
	selectedDraft *config.Draft
}

// NewDrafts creates a new drafts list view
func NewDrafts(drafts []config.Draft) *Drafts {
	items := make([]list.Item, len(drafts))
	for i, d := range drafts {
		items[i] = draftItem{draft: d}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Drafts"
	l.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetStatusBarItemName("draft", "drafts")
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("\ue5fe enter", "open")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("\uea81 d", "delete")),
		}
	}
	l.KeyMap.Quit.SetEnabled(false)

	return &Drafts{
		list:   l,
		drafts: drafts,
	}
}

func (m *Drafts) Init() tea.Cmd {
	return nil
}

func (m *Drafts) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 4)
		return m, nil

	case tea.KeyMsg:
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
			case "n", "N", "esc":
				m.confirmDelete = false
				m.selectedDraft = nil
			}
			return m, nil
		}

		// Skip key handling during filtering
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
		case "enter":
			if item, ok := m.list.SelectedItem().(draftItem); ok {
				return m, func() tea.Msg {
					return OpenDraftMsg{Draft: item.draft}
				}
			}
		case "d":
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

func (m *Drafts) View() string {
	var b strings.Builder

	if m.confirmDelete {
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				"Delete this draft?",
				HelpStyle.Render("\n(y/n)"),
			),
		)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	if len(m.drafts) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("No drafts saved.\n\nPress esc to go back.")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, emptyMsg)
	}

	b.WriteString(m.list.View())
	return b.String()
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
