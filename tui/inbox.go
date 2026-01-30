package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

var (
	paginationStyle = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	inboxHelpStyle  = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	tabStyle        = lipgloss.NewStyle().Padding(0, 2)
	activeTabStyle  = lipgloss.NewStyle().Padding(0, 2).Foreground(lipgloss.Color("42")).Bold(true).Underline(true)
	tabBarStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).PaddingBottom(1).MarginBottom(1)
)

type item struct {
	title, desc   string
	originalIndex int
	uid           uint32
	accountID     string
	accountEmail  string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title + " " + i.desc }

type itemDelegate struct{}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i.title)

	// For "ALL" view, show account indicator
	if i.accountEmail != "" {
		str = fmt.Sprintf("%d. [%s] %s", index+1, truncateEmail(i.accountEmail), i.title)
	}

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + s[0])
		}
	}

	fmt.Fprint(w, fn(str))
}

// truncateEmail shortens an email for display
func truncateEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) >= 1 && len(parts[0]) > 8 {
		return parts[0][:8] + "..."
	}
	if len(parts) >= 1 {
		return parts[0]
	}
	return email
}

// AccountTab represents a tab for an account
type AccountTab struct {
	ID    string
	Label string
	Email string
}

type Inbox struct {
	list             list.Model
	isFetching       bool
	isRefreshing     bool
	emailsCount      int
	accounts         []config.Account
	emailsByAccount  map[string][]fetcher.Email
	allEmails        []fetcher.Email
	tabs             []AccountTab
	activeTabIndex   int
	width            int
	height           int
	currentAccountID string // Empty means "ALL"
	emailCountByAcct map[string]int
	mailbox          MailboxKind
}

func NewInbox(emails []fetcher.Email, accounts []config.Account) *Inbox {
	return NewInboxWithMailbox(emails, accounts, MailboxInbox)
}

func NewSentInbox(emails []fetcher.Email, accounts []config.Account) *Inbox {
	return NewInboxWithMailbox(emails, accounts, MailboxSent)
}

func NewTrashInbox(emails []fetcher.Email, accounts []config.Account) *Inbox {
	return NewInboxWithMailbox(emails, accounts, MailboxTrash)
}

func NewArchiveInbox(emails []fetcher.Email, accounts []config.Account) *Inbox {
	return NewInboxWithMailbox(emails, accounts, MailboxArchive)
}

func NewInboxWithMailbox(emails []fetcher.Email, accounts []config.Account, mailbox MailboxKind) *Inbox {
	// Build tabs: empty for single account, "ALL" + accounts for multiple
	var tabs []AccountTab
	if len(accounts) <= 1 {
		tabs = []AccountTab{{ID: "", Label: "", Email: ""}}
	} else {
		tabs = []AccountTab{{ID: "", Label: "ALL", Email: ""}}
		for _, acc := range accounts {
			// Use FetchEmail for display, fall back to Email if not set
			displayEmail := acc.FetchEmail
			if displayEmail == "" {
				displayEmail = acc.Email
			}
			tabs = append(tabs, AccountTab{ID: acc.ID, Label: displayEmail, Email: displayEmail})
		}
	}

	// Group emails by account
	emailsByAccount := make(map[string][]fetcher.Email)
	for _, email := range emails {
		emailsByAccount[email.AccountID] = append(emailsByAccount[email.AccountID], email)
	}

	// Track email counts per account
	emailCountByAcct := make(map[string]int)
	for accID, accEmails := range emailsByAccount {
		emailCountByAcct[accID] = len(accEmails)
	}

	inbox := &Inbox{
		accounts:         accounts,
		emailsByAccount:  emailsByAccount,
		allEmails:        emails,
		tabs:             tabs,
		activeTabIndex:   0,
		currentAccountID: "",
		emailCountByAcct: emailCountByAcct,
		mailbox:          mailbox,
	}

	inbox.updateList()
	return inbox
}

// NewInboxSingleAccount creates an inbox for a single account (legacy support)
func NewInboxSingleAccount(emails []fetcher.Email) *Inbox {
	return NewInbox(emails, nil)
}

func (m *Inbox) updateList() {
	var displayEmails []fetcher.Email
	var showAccountLabel bool

	if m.currentAccountID == "" {
		// "ALL" view - show all emails sorted by date
		displayEmails = m.allEmails
		showAccountLabel = !(len(m.accounts) <= 1)
	} else {
		// Specific account view
		displayEmails = m.emailsByAccount[m.currentAccountID]
		showAccountLabel = false
	}

	m.emailsCount = len(displayEmails)

	items := make([]list.Item, len(displayEmails))
	for i, email := range displayEmails {
		accountEmail := ""
		if showAccountLabel {
			// Find the account email for display
			for _, acc := range m.accounts {
				if acc.ID == email.AccountID {
					accountEmail = acc.FetchEmail
					break
				}
			}
		}

		items[i] = item{
			title:         email.Subject,
			desc:          email.From,
			originalIndex: i,
			uid:           email.UID,
			accountID:     email.AccountID,
			accountEmail:  accountEmail,
		}
	}

	l := list.New(items, itemDelegate{}, 20, 14)
	l.Title = m.getTitle()
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = inboxHelpStyle
	l.SetStatusBarItemName("email", "emails")
	l.AdditionalShortHelpKeys = func() []key.Binding {
		bindings := []key.Binding{
			key.NewBinding(key.WithKeys("d"), key.WithHelp("\uf014 d", "delete")),
			key.NewBinding(key.WithKeys("a"), key.WithHelp("\uea98 a", "archive")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("\ue348 r", "refresh")),
		}
		if len(m.tabs) > 1 {
			bindings = append(bindings,
				key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev tab")),
				key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next tab")),
			)
		}
		return bindings
	}

	l.KeyMap.Quit.SetEnabled(false)

	if m.width > 0 {
		l.SetWidth(m.width)
	}

	m.list = l
}

func (m *Inbox) getTitle() string {
	var title string
	if m.currentAccountID == "" {
		title = m.getBaseTitle() + " - All Accounts"
	} else {
		title = m.getBaseTitle()
		for _, acc := range m.accounts {
			if acc.ID == m.currentAccountID {
				if acc.Name != "" {
					title = fmt.Sprintf("%s - %s", m.getBaseTitle(), acc.Name)
				} else {
					title = fmt.Sprintf("%s - %s", m.getBaseTitle(), acc.FetchEmail)
				}
				break
			}
		}
	}
	if m.isRefreshing {
		title += " (refreshing...)"
	}
	if m.isFetching {
		title += " (loading more...)"
	}
	return title
}

func (m *Inbox) getBaseTitle() string {
	switch m.mailbox {
	case MailboxSent:
		return "Sent"
	case MailboxTrash:
		return "Trash"
	case MailboxArchive:
		return "Archive"
	default:
		return "Inbox"
	}
}

func (m *Inbox) Init() tea.Cmd {
	return nil
}

func (m *Inbox) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch keypress := msg.String(); keypress {
		case "left", "h":
			if len(m.tabs) > 1 {
				m.activeTabIndex--
				if m.activeTabIndex < 0 {
					m.activeTabIndex = len(m.tabs) - 1
				}
				m.currentAccountID = m.tabs[m.activeTabIndex].ID
				m.updateList()
				return m, nil
			}
		case "right", "l":
			if len(m.tabs) > 1 {
				m.activeTabIndex++
				if m.activeTabIndex >= len(m.tabs) {
					m.activeTabIndex = 0
				}
				m.currentAccountID = m.tabs[m.activeTabIndex].ID
				m.updateList()
				return m, nil
			}
		case "d":
			selectedItem, ok := m.list.SelectedItem().(item)
			if ok {
				return m, func() tea.Msg {
					return DeleteEmailMsg{UID: selectedItem.uid, AccountID: selectedItem.accountID, Mailbox: m.mailbox}
				}
			}
		case "a":
			selectedItem, ok := m.list.SelectedItem().(item)
			if ok {
				return m, func() tea.Msg {
					return ArchiveEmailMsg{UID: selectedItem.uid, AccountID: selectedItem.accountID, Mailbox: m.mailbox}
				}
			}
		case "r":
			return m, func() tea.Msg {
				return RequestRefreshMsg{Mailbox: m.mailbox}
			}
		case "enter":
			selectedItem, ok := m.list.SelectedItem().(item)
			if ok {
				idx := selectedItem.originalIndex
				uid := selectedItem.uid
				accountID := selectedItem.accountID
				return m, func() tea.Msg {
					return ViewEmailMsg{Index: idx, UID: uid, AccountID: accountID, Mailbox: m.mailbox}
				}
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		return m, nil

	case FetchingMoreEmailsMsg:
		m.isFetching = true
		m.list.Title = m.getTitle()
		return m, nil

	case EmailsAppendedMsg:
		if msg.Mailbox != m.mailbox {
			return m, nil
		}
		m.isFetching = false
		m.list.Title = m.getTitle()

		// Add emails to the appropriate account
		for _, email := range msg.Emails {
			m.emailsByAccount[email.AccountID] = append(m.emailsByAccount[email.AccountID], email)
			m.allEmails = append(m.allEmails, email)
		}
		m.emailCountByAcct[msg.AccountID] = len(m.emailsByAccount[msg.AccountID])

		m.updateList()
		return m, nil

	case RefreshingEmailsMsg:
		if msg.Mailbox != m.mailbox {
			return m, nil
		}
		m.isRefreshing = true
		m.list.Title = m.getTitle()
		return m, nil

	case EmailsRefreshedMsg:
		if msg.Mailbox != m.mailbox {
			return m, nil
		}
		m.isRefreshing = false

		// Replace emails with fresh data
		m.emailsByAccount = msg.EmailsByAccount

		// Flatten all emails
		var allEmails []fetcher.Email
		for _, emails := range msg.EmailsByAccount {
			allEmails = append(allEmails, emails...)
		}

		// Sort by date (newest first)
		for i := 0; i < len(allEmails); i++ {
			for j := i + 1; j < len(allEmails); j++ {
				if allEmails[j].Date.After(allEmails[i].Date) {
					allEmails[i], allEmails[j] = allEmails[j], allEmails[i]
				}
			}
		}

		m.allEmails = allEmails

		// Update email counts
		m.emailCountByAcct = make(map[string]int)
		for accID, accEmails := range m.emailsByAccount {
			m.emailCountByAcct[accID] = len(accEmails)
		}

		m.updateList()
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	if m.shouldFetchMore() {
		cmds = append(cmds, m.fetchMoreCmds()...)
	}
	return m, tea.Batch(cmds...)
}

func (m *Inbox) shouldFetchMore() bool {
	if m.isFetching {
		return false
	}
	if len(m.list.Items()) == 0 {
		return false
	}
	if m.list.FilterState() == list.Filtering {
		return false
	}
	return m.list.Index() >= len(m.list.Items())-1
}

func (m *Inbox) fetchMoreCmds() []tea.Cmd {
	var cmds []tea.Cmd
	if m.currentAccountID == "" {
		if len(m.accounts) == 0 {
			return nil
		}
		for _, acc := range m.accounts {
			accountID := acc.ID
			offset := uint32(len(m.emailsByAccount[accountID]))
			cmds = append(cmds, func(id string, off uint32) tea.Cmd {
				return func() tea.Msg {
					return FetchMoreEmailsMsg{Offset: off, AccountID: id, Mailbox: m.mailbox}
				}
			}(accountID, offset))
		}
		return cmds
	}

	offset := uint32(len(m.emailsByAccount[m.currentAccountID]))
	cmds = append(cmds, func(id string, off uint32) tea.Cmd {
		return func() tea.Msg {
			return FetchMoreEmailsMsg{Offset: off, AccountID: id, Mailbox: m.mailbox}
		}
	}(m.currentAccountID, offset))
	return cmds
}

func (m *Inbox) View() string {
	var b strings.Builder

	// Render tabs if there are multiple accounts
	if len(m.tabs) > 1 {
		var tabViews []string
		for i, tab := range m.tabs {
			label := tab.Label
			if tab.ID == "" {
				label = "ALL"
			}

			if i == m.activeTabIndex {
				tabViews = append(tabViews, activeTabStyle.Render(label))
			} else {
				tabViews = append(tabViews, tabStyle.Render(label))
			}
		}
		tabBar := tabBarStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, tabViews...))
		b.WriteString(tabBar)
		b.WriteString("\n")
	}

	b.WriteString(m.list.View())
	return b.String()
}

// GetCurrentAccountID returns the currently selected account ID
func (m *Inbox) GetCurrentAccountID() string {
	return m.currentAccountID
}

// GetEmailAtIndex returns the email at the given index for the current view
func (m *Inbox) GetEmailAtIndex(index int) *fetcher.Email {
	var displayEmails []fetcher.Email
	if m.currentAccountID == "" {
		displayEmails = m.allEmails
	} else {
		displayEmails = m.emailsByAccount[m.currentAccountID]
	}

	if index >= 0 && index < len(displayEmails) {
		return &displayEmails[index]
	}
	return nil
}

func (m *Inbox) GetMailbox() MailboxKind {
	return m.mailbox
}

// RemoveEmail removes an email by UID and account ID
func (m *Inbox) RemoveEmail(uid uint32, accountID string) {
	// Remove from account-specific list
	if emails, ok := m.emailsByAccount[accountID]; ok {
		var filtered []fetcher.Email
		for _, e := range emails {
			if e.UID != uid {
				filtered = append(filtered, e)
			}
		}
		m.emailsByAccount[accountID] = filtered
	}

	// Remove from all emails list
	var filteredAll []fetcher.Email
	for _, e := range m.allEmails {
		if !(e.UID == uid && e.AccountID == accountID) {
			filteredAll = append(filteredAll, e)
		}
	}
	m.allEmails = filteredAll

	m.updateList()
}

// SetEmails updates all emails (used after fetch)
func (m *Inbox) SetEmails(emails []fetcher.Email, accounts []config.Account) {
	m.accounts = accounts
	m.allEmails = emails

	// Rebuild tabs: empty for single account, "ALL" + accounts for multiple
	var tabs []AccountTab
	if len(accounts) <= 1 {
		tabs = []AccountTab{{ID: "", Label: "", Email: ""}}
	} else {
		tabs = []AccountTab{{ID: "", Label: "ALL", Email: ""}}
		for _, acc := range accounts {
			tabs = append(tabs, AccountTab{ID: acc.ID, Label: acc.FetchEmail, Email: acc.Email})
		}
	}
	m.tabs = tabs

	// Re-group emails by account
	m.emailsByAccount = make(map[string][]fetcher.Email)
	for _, email := range emails {
		m.emailsByAccount[email.AccountID] = append(m.emailsByAccount[email.AccountID], email)
	}

	// Update email counts
	m.emailCountByAcct = make(map[string]int)
	for accID, accEmails := range m.emailsByAccount {
		m.emailCountByAcct[accID] = len(accEmails)
	}

	m.updateList()
}
