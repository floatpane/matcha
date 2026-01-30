package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

var (
	mailboxTabStyle       = lipgloss.NewStyle().Padding(0, 3)
	activeMailboxTabStyle = lipgloss.NewStyle().Padding(0, 3).Foreground(lipgloss.Color("42")).Bold(true).Underline(true)
	mailboxTabBarStyle    = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).PaddingBottom(1).MarginBottom(1)
)

// TrashArchive is a combined view for trash and archive emails with a toggle
type TrashArchive struct {
	trashInbox   *Inbox
	archiveInbox *Inbox
	activeView   MailboxKind // MailboxTrash or MailboxArchive
	width        int
	height       int
	accounts     []config.Account
}

// NewTrashArchive creates a new combined trash/archive view
func NewTrashArchive(trashEmails, archiveEmails []fetcher.Email, accounts []config.Account) *TrashArchive {
	return &TrashArchive{
		trashInbox:   NewTrashInbox(trashEmails, accounts),
		archiveInbox: NewArchiveInbox(archiveEmails, accounts),
		activeView:   MailboxTrash,
		accounts:     accounts,
	}
}

func (m *TrashArchive) Init() tea.Cmd {
	return nil
}

func (m *TrashArchive) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			// Toggle between trash and archive
			if m.activeView == MailboxTrash {
				m.activeView = MailboxArchive
			} else {
				m.activeView = MailboxTrash
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Pass to both inboxes
		m.trashInbox.Update(msg)
		m.archiveInbox.Update(msg)
		return m, nil

	case FetchingMoreEmailsMsg, EmailsAppendedMsg, RefreshingEmailsMsg, EmailsRefreshedMsg:
		// Forward to the appropriate inbox based on mailbox
		if m.activeView == MailboxTrash {
			m.trashInbox.Update(msg)
		} else {
			m.archiveInbox.Update(msg)
		}
		return m, nil
	}

	// Forward other messages to the active inbox
	var cmd tea.Cmd
	if m.activeView == MailboxTrash {
		_, cmd = m.trashInbox.Update(msg)
	} else {
		_, cmd = m.archiveInbox.Update(msg)
	}
	return m, cmd
}

func (m *TrashArchive) View() string {
	var b strings.Builder

	// Render the mailbox toggle tabs
	var tabViews []string
	if m.activeView == MailboxTrash {
		tabViews = append(tabViews, activeMailboxTabStyle.Render("Trash"))
		tabViews = append(tabViews, mailboxTabStyle.Render("Archive"))
	} else {
		tabViews = append(tabViews, mailboxTabStyle.Render("Trash"))
		tabViews = append(tabViews, activeMailboxTabStyle.Render("Archive"))
	}
	tabBar := mailboxTabBarStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, tabViews...))
	b.WriteString(tabBar)
	b.WriteString("\n")

	// Add help text for tab switching
	helpText := helpStyle.Render("Press tab to switch between Trash and Archive")
	b.WriteString(helpText)
	b.WriteString("\n\n")

	// Render the active inbox
	if m.activeView == MailboxTrash {
		b.WriteString(m.trashInbox.View())
	} else {
		b.WriteString(m.archiveInbox.View())
	}

	return b.String()
}

// GetActiveMailbox returns the currently active mailbox kind
func (m *TrashArchive) GetActiveMailbox() MailboxKind {
	return m.activeView
}

// GetActiveInbox returns the currently active inbox
func (m *TrashArchive) GetActiveInbox() *Inbox {
	if m.activeView == MailboxTrash {
		return m.trashInbox
	}
	return m.archiveInbox
}

// RemoveEmail removes an email from the appropriate inbox
func (m *TrashArchive) RemoveEmail(uid uint32, accountID string, mailbox MailboxKind) {
	if mailbox == MailboxTrash {
		m.trashInbox.RemoveEmail(uid, accountID)
	} else if mailbox == MailboxArchive {
		m.archiveInbox.RemoveEmail(uid, accountID)
	}
}

// SetTrashEmails updates the trash emails
func (m *TrashArchive) SetTrashEmails(emails []fetcher.Email, accounts []config.Account) {
	m.trashInbox.SetEmails(emails, accounts)
	m.accounts = accounts
}

// SetArchiveEmails updates the archive emails
func (m *TrashArchive) SetArchiveEmails(emails []fetcher.Email, accounts []config.Account) {
	m.archiveInbox.SetEmails(emails, accounts)
	m.accounts = accounts
}

// AdditionalShortHelpKeys returns additional help keys for the trash/archive view
func (m *TrashArchive) AdditionalShortHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch view")),
	}
}
