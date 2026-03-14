package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/theme"
)

var (
	// In bubbles v2, list.DefaultStyles() takes a boolean for hasDarkBackground
	paginationStyle = list.DefaultStyles(true).PaginationStyle.PaddingLeft(4)
	inboxHelpStyle  = list.DefaultStyles(true).HelpStyle.PaddingLeft(4).PaddingBottom(1)
	tabStyle        = lipgloss.NewStyle().Padding(0, 2)
	activeTabStyle  = lipgloss.NewStyle().Padding(0, 2).Foreground(lipgloss.Color("42")).Bold(true).Underline(true)
	tabBarStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).PaddingBottom(1).MarginBottom(1)
)

var dateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
var senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true)

type item struct {
	title, desc   string
	originalIndex int
	uid           uint32
	accountID     string
	accountEmail  string
	date          time.Time
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

	prefix := fmt.Sprintf("%d. ", index+1)
	sender := parseSenderName(i.desc)
	styledSender := senderStyle.Render(sender)
	separator := " · "

	// For "ALL" view, show account indicator instead of number
	if i.accountEmail != "" {
		prefix = fmt.Sprintf("%d. [%s] ", index+1, truncateEmail(i.accountEmail))
	}

	// Format and right-align date
	dateStr := formatRelativeDate(i.date)
	styledDate := dateStyle.Render(dateStr)
	dateWidth := lipgloss.Width(styledDate)

	listWidth := m.Width()
	isSelected := index == m.Index()
	cursorWidth := 0
	if isSelected {
		cursorWidth = 2 // "> " prefix
	}

	// Available width for the whole left side (prefix + sender + separator + subject)
	maxLeft := listWidth - dateWidth - 2 - cursorWidth // 2 for spacing
	if maxLeft < 10 {
		maxLeft = 10
	}

	prefixWidth := lipgloss.Width(prefix)
	senderWidth := lipgloss.Width(styledSender)
	sepWidth := len(separator)
	subjectBudget := maxLeft - prefixWidth - senderWidth - sepWidth

	subject := i.title
	if subjectBudget < 4 {
		subjectBudget = 4
	}
	if lipgloss.Width(subject) > subjectBudget {
		for lipgloss.Width(subject) > subjectBudget-1 && len(subject) > 0 {
			subject = subject[:len(subject)-1]
		}
		subject += "…"
	}

	str := prefix + styledSender + separator + subject

	// Pad to push date to the right
	padding := listWidth - lipgloss.Width(str) - dateWidth - cursorWidth
	if padding < 1 {
		padding = 1
	}

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + s[0])
		}
	}

	fmt.Fprint(w, fn(str+strings.Repeat(" ", padding)+styledDate))
}

// formatRelativeDate formats a time as relative if within the last week,
// otherwise as an absolute date.
func formatRelativeDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	d := now.Sub(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		if t.Year() == now.Year() {
			return t.Format("Jan 02")
		}
		return t.Format("Jan 02, 2006")
	}
}

// parseSenderName extracts the display name from a "Name <email>" string,
// falling back to the local part of the email address.
func parseSenderName(from string) string {
	if idx := strings.Index(from, " <"); idx > 0 {
		return strings.TrimSpace(from[:idx])
	}
	// No display name — use local part of email
	if idx := strings.Index(from, "@"); idx > 0 {
		return from[:idx]
	}
	return from
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
	list               list.Model
	isFetching         bool
	isRefreshing       bool
	emailsCount        int
	accounts           []config.Account
	emailsByAccount    map[string][]fetcher.Email
	allEmails          []fetcher.Email
	tabs               []AccountTab
	activeTabIndex     int
	width              int
	height             int
	currentAccountID   string // Empty means "ALL"
	emailCountByAcct   map[string]int
	mailbox            MailboxKind
	folderName         string          // Custom folder name override for title
	noMoreByAccount    map[string]bool // Per-account: true when pagination returns 0 results
	extraShortHelpKeys []key.Binding
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
	// Capture current index to restore later
	currentIndex := m.list.Index()

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
			date:          email.Date,
		}
	}

	l := list.New(items, itemDelegate{}, 20, 14)
	l.Title = m.getTitle()
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent).Bold(true)
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
		bindings = append(bindings, m.extraShortHelpKeys...)
		return bindings
	}

	l.KeyMap.Quit.SetEnabled(false)

	// Disable default help to render it manually at the bottom
	l.SetShowHelp(false)

	if m.width > 0 {
		l.SetWidth(m.width)
	}
	if m.height > 0 {
		l.SetHeight(m.height / 2)
	}

	// Restore index
	// If index is out of bounds (e.g. list shrank), clamp it.
	if currentIndex >= len(items) {
		currentIndex = len(items) - 1
	}
	if currentIndex < 0 {
		currentIndex = 0
	}
	l.Select(currentIndex)

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
	if m.folderName != "" {
		return m.folderName
	}
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
	case tea.KeyPressMsg:
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
			m.isRefreshing = true
			m.list.Title = m.getTitle()
			// Copy counts to avoid race conditions if used elsewhere (though here it's just passing data)
			counts := make(map[string]int)
			for k, v := range m.emailCountByAcct {
				counts[k] = v
			}
			return m, func() tea.Msg {
				return RequestRefreshMsg{Mailbox: m.mailbox, Counts: counts}
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
		m.list.SetHeight(msg.Height / 2)
		if m.shouldFetchMore() {
			return m, tea.Batch(m.fetchMoreCmds()...)
		}
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

		if len(msg.Emails) == 0 {
			if m.noMoreByAccount == nil {
				m.noMoreByAccount = make(map[string]bool)
			}
			m.noMoreByAccount[msg.AccountID] = true
			return m, nil
		}

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
		// Only clear the refreshing indicator. The actual email data is
		// merged by the main model (preserving paginated emails) and
		// pushed to us via SetEmails, so we must not overwrite it here.
		m.isRefreshing = false
		m.list.Title = m.getTitle()
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
	if m.isFetching || m.isRefreshing {
		return false
	}
	if m.allAccountsExhausted() {
		return false
	}
	if len(m.list.Items()) == 0 {
		return false
	}
	if m.list.FilterState() == list.Filtering {
		return false
	}
	// Fetch if we've reached the bottom OR if we don't have enough items to fill the view
	return m.list.Index() >= len(m.list.Items())-1 || len(m.list.Items()) < m.list.Height()
}

// allAccountsExhausted returns true if all relevant accounts have no more emails to fetch.
func (m *Inbox) allAccountsExhausted() bool {
	if len(m.noMoreByAccount) == 0 {
		return false
	}
	if m.currentAccountID != "" {
		return m.noMoreByAccount[m.currentAccountID]
	}
	// "ALL" view: all accounts must be exhausted
	for _, acc := range m.accounts {
		if !m.noMoreByAccount[acc.ID] {
			return false
		}
	}
	return len(m.accounts) > 0
}

func (m *Inbox) fetchMoreCmds() []tea.Cmd {
	var cmds []tea.Cmd
	limit := uint32(m.list.Height())
	if limit < 20 {
		limit = 20
	}

	if m.currentAccountID == "" {
		if len(m.accounts) == 0 {
			return nil
		}
		for _, acc := range m.accounts {
			accountID := acc.ID
			if m.noMoreByAccount[accountID] {
				continue
			}
			offset := uint32(len(m.emailsByAccount[accountID]))
			cmds = append(cmds, func(id string, off uint32) tea.Cmd {
				return func() tea.Msg {
					return FetchMoreEmailsMsg{Offset: off, AccountID: id, Mailbox: m.mailbox, Limit: limit}
				}
			}(accountID, offset))
		}
		return cmds
	}

	if m.noMoreByAccount[m.currentAccountID] {
		return nil
	}
	offset := uint32(len(m.emailsByAccount[m.currentAccountID]))
	cmds = append(cmds, func(id string, off uint32) tea.Cmd {
		return func() tea.Msg {
			return FetchMoreEmailsMsg{Offset: off, AccountID: id, Mailbox: m.mailbox, Limit: limit}
		}
	}(m.currentAccountID, offset))
	return cmds
}

func (m *Inbox) View() tea.View {
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

	// Ensure we don't start gap calculation on the same line as the list
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}

	helpView := inboxHelpStyle.Render(m.list.Help.View(m.list))

	if m.height > 0 {
		usedHeight := lipgloss.Height(b.String())
		helpHeight := lipgloss.Height(helpView)

		gap := m.height - usedHeight - helpHeight
		if gap > 0 {
			b.WriteString(strings.Repeat("\n", gap))
		}
	} else {
		b.WriteString("\n")
	}

	b.WriteString(helpView)

	return tea.NewView(b.String())
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

// SetSize sets the width and height of the inbox, then updates the list.
func (m *Inbox) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.list.SetWidth(width)
	m.list.SetHeight(height / 2)
}

// SetFolderName sets a custom folder name for the inbox title.
func (m *Inbox) SetFolderName(name string) {
	m.folderName = name
	m.list.Title = m.getTitle()
}

// SetEmails updates all emails (used after fetch)
func (m *Inbox) SetEmails(emails []fetcher.Email, accounts []config.Account) {
	m.accounts = accounts
	m.allEmails = emails
	m.noMoreByAccount = make(map[string]bool)

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
