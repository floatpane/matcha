package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

const sidebarWidth = 25

var (
	sidebarStyle = lipgloss.NewStyle().
			Width(sidebarWidth).
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(true).
			PaddingRight(1).
			PaddingLeft(1)

	sidebarTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true).
				PaddingBottom(1)

	folderStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1)

	activeFolderStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				PaddingRight(1).
				Background(lipgloss.Color("42")).
				Foreground(lipgloss.Color("#000000")).
				Bold(true)

	moveOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#25A065")).
				Padding(1, 2)

	moveOverlayTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true).
				PaddingBottom(1)

	moveItemStyle = lipgloss.NewStyle().
			PaddingLeft(1)

	moveSelectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				Foreground(lipgloss.Color("42")).
				Bold(true)

	inboxPaneStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(true).
			PaddingRight(1)

	previewPaneStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				PaddingLeft(1)

	focusedBorderColor   = lipgloss.Color("42")
	unfocusedBorderColor = lipgloss.Color("240")
)

type PaneType int

const (
	FocusInbox PaneType = iota
	FocusPreview
)

// FolderInbox combines a folder sidebar with an email list.
type FolderInbox struct {
	folders         []string
	activeFolderIdx int
	currentFolder   string
	inbox           *Inbox
	accounts        []config.Account
	width           int
	height          int
	isLoadingEmails bool

	// Move-to-folder overlay state
	movingEmail      bool
	moveTargetIdx    int
	moveUID          uint32   // Legacy: single UID
	moveUIDs         []uint32 // Batch: multiple UIDs
	moveAccountID    string
	moveSourceFolder string

	// Split pane state
	previewPane        *EmailView
	previewedUID       uint32
	previewedAccountID string
	// previewSearchEmail holds an Email handed in by OpenSplitPreview for hits
	// that do not live in m.inbox.allEmails (search results across folders).
	// findEmailByUID falls back to it when allEmails has no match.
	previewSearchEmail *fetcher.Email
	focusedPane        PaneType
}

// sortFolders sorts folder names with INBOX always first, then alphabetically.
func sortFolders(folders []string) []string {
	sorted := make([]string, len(folders))
	copy(sorted, folders)
	sort.SliceStable(sorted, func(i, j int) bool {
		iUpper := strings.ToUpper(sorted[i])
		jUpper := strings.ToUpper(sorted[j])
		if iUpper == "INBOX" {
			return true
		}
		if jUpper == "INBOX" {
			return false
		}
		return sorted[i] < sorted[j]
	})
	return sorted
}

// SetDateFormat propagates the configured date layout to the inner inbox.
func (m *FolderInbox) SetDateFormat(layout string) {
	if m.inbox != nil {
		m.inbox.SetDateFormat(layout)
	}
}

// NewFolderInbox creates a new FolderInbox with the given folders and accounts.
func NewFolderInbox(folders []string, accounts []config.Account) *FolderInbox {
	folders = sortFolders(folders)
	currentFolder := "INBOX"
	if len(folders) > 0 {
		currentFolder = folders[0]
	}

	inbox := NewInbox(nil, accounts)
	inbox.SetFolderName(currentFolder)

	fi := &FolderInbox{
		folders:         folders,
		activeFolderIdx: 0,
		currentFolder:   currentFolder,
		inbox:           inbox,
		accounts:        accounts,
	}
	fi.updateHelpKeys()
	return fi
}

func (m *FolderInbox) Init() tea.Cmd {
	return nil
}

func (m *FolderInbox) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If move overlay is active, handle its input
	if m.movingEmail {
		return m.updateMoveOverlay(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Don't intercept keys while filtering
		if m.inbox.list.FilterState() == list.Filtering {
			break
		}

		kb := config.Keybinds

		// Route input to preview pane when focused
		if m.previewPane != nil && m.focusedPane == FocusPreview {
			s := msg.String()
			if s != kb.Folder.FocusInbox && s != kb.Folder.FocusPreview && s != kb.Global.Cancel && s != "q" {
				var cmd tea.Cmd
				_, cmd = m.previewPane.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case kb.Folder.FocusPreview:
			// Switch focus to preview pane
			if m.previewPane != nil && m.focusedPane == FocusInbox {
				m.focusedPane = FocusPreview
				return m, nil
			}
		case kb.Folder.FocusInbox:
			// Switch focus to inbox pane
			if m.previewPane != nil && m.focusedPane == FocusPreview {
				m.focusedPane = FocusInbox
				return m, nil
			}
		case kb.Folder.NextFolder:
			m.activeFolderIdx++
			if m.activeFolderIdx >= len(m.folders) {
				m.activeFolderIdx = 0
			}
			return m, m.switchFolder()
		case kb.Folder.PrevFolder:
			m.activeFolderIdx--
			if m.activeFolderIdx < 0 {
				m.activeFolderIdx = len(m.folders) - 1
			}
			return m, m.switchFolder()
		case kb.Global.Cancel:
			// Close split preview if open
			if m.previewPane != nil {
				m.closeSplitPreview()
				return m, nil
			}
			// Otherwise let inbox handle (or parent)
		case kb.Folder.Move:
			// Start move-to-folder flow
			if m.inbox.visualMode && len(m.inbox.selectedUIDs) > 0 {
				// Batch move
				m.movingEmail = true
				m.moveTargetIdx = 0
				m.moveUIDs = make([]uint32, len(m.inbox.selectionOrder))
				copy(m.moveUIDs, m.inbox.selectionOrder)
				m.moveAccountID = ""
				for _, acctID := range m.inbox.selectedUIDs {
					m.moveAccountID = acctID
					break
				}
				m.moveSourceFolder = m.currentFolder
				return m, nil
			} else {
				// Single move
				selectedItem, ok := m.inbox.list.SelectedItem().(item)
				if ok {
					m.movingEmail = true
					m.moveTargetIdx = 0
					m.moveUID = selectedItem.uid
					m.moveUIDs = []uint32{selectedItem.uid}
					m.moveAccountID = selectedItem.accountID
					m.moveSourceFolder = m.currentFolder
					return m, nil
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.previewPane != nil || m.previewedUID != 0 {
			// Recalculate pane widths for split mode
			inboxWidth := m.calculateInboxWidth()
			previewWidth := m.calculatePreviewWidth()
			m.inbox.SetSize(inboxWidth-2, msg.Height)
			if m.previewPane != nil {
				// Forward resize to EmailView with preview pane dimensions
				previewMsg := tea.WindowSizeMsg{Width: previewWidth - 2, Height: msg.Height - 2}
				m.previewPane.Update(previewMsg)
			}
		} else {
			// Original two-pane resize
			inboxWidth := msg.Width - sidebarWidth - 3
			if inboxWidth < 20 {
				inboxWidth = 20
			}
			m.inbox.SetSize(inboxWidth, msg.Height)
		}
		return m, nil

	case FolderEmailsFetchedMsg:
		// Ignore stale responses for folders the user has navigated away from
		if msg.FolderName != m.currentFolder {
			return m, nil
		}
		m.isLoadingEmails = false
		m.inbox.isFetching = false
		m.inbox.isRefreshing = false
		m.inbox.SetEmails(msg.Emails, m.accounts)
		m.inbox.SetFolderName(msg.FolderName)
		return m, nil

	case FolderEmailsAppendedMsg:
		if msg.FolderName != m.currentFolder {
			return m, nil
		}
		m.inbox.isFetching = false
		m.inbox.list.Title = m.inbox.getTitle()
		if len(msg.Emails) == 0 {
			if m.inbox.noMoreByAccount == nil {
				m.inbox.noMoreByAccount = make(map[string]bool)
			}
			m.inbox.noMoreByAccount[msg.AccountID] = true
			return m, nil
		}
		for _, email := range msg.Emails {
			m.inbox.emailsByAccount[email.AccountID] = append(m.inbox.emailsByAccount[email.AccountID], email)
			m.inbox.allEmails = append(m.inbox.allEmails, email)
		}
		m.inbox.emailCountByAcct[msg.AccountID] = len(m.inbox.emailsByAccount[msg.AccountID])
		m.inbox.updateList()
		return m, nil

	case EmailMovedMsg:
		if msg.Err != nil {
			// Error handled by main model
			return m, nil
		}
		m.inbox.RemoveEmail(msg.UID, msg.AccountID)
		// Clear preview if moved email was being previewed
		if msg.UID == m.previewedUID {
			m.closeSplitPreview()
		}
		return m, nil

	case UpdatePreviewMsg:
		// Stale update, ignore
		if msg.UID == m.previewedUID && m.previewPane != nil {
			return m, nil
		}
		m.previewedUID = msg.UID
		m.previewedAccountID = msg.AccountID
		// Will trigger fetch in main.go
		return m, nil

	case PreviewBodyFetchedMsg:
		// Stale fetch or no preview active
		if msg.UID != m.previewedUID {
			return m, nil
		}
		if msg.Err != nil {
			// Show error in preview pane
			return m, nil
		}
		// Find email and create preview
		email := m.findEmailByUID(msg.UID, msg.AccountID)
		if email == nil {
			return m, nil
		}
		// Update email with body
		email.Body = msg.Body
		email.Attachments = msg.Attachments
		// Create preview pane with column offset for image rendering
		previewWidth := m.calculatePreviewWidth()
		inboxWidth := m.calculateInboxWidth()
		colOffset := sidebarWidth + 2 + inboxWidth + 2 // borders + padding
		m.previewPane = NewEmailViewPreview(*email, previewWidth, m.height, colOffset, false)
		return m, nil
	}

	// Forward to inbox
	var cmd tea.Cmd
	_, cmd = m.inbox.Update(msg)

	// Intercept FetchMoreEmailsMsg from inbox and convert to folder-aware version
	if cmd != nil {
		wrappedCmd := m.wrapInboxCmd(cmd)
		return m, wrappedCmd
	}

	return m, cmd
}

// wrapInboxCmd intercepts messages from the inbox and adds folder context.
func (m *FolderInbox) wrapInboxCmd(cmd tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		msg := cmd()
		switch inner := msg.(type) {
		case FetchMoreEmailsMsg:
			return FetchFolderMoreEmailsMsg{
				Offset:     inner.Offset,
				AccountID:  inner.AccountID,
				FolderName: m.currentFolder,
				Limit:      inner.Limit,
			}
		case RequestRefreshMsg:
			inner.FolderName = m.currentFolder
			return inner
		case SearchRequestedMsg:
			inner.FolderName = m.currentFolder
			return inner
		}
		return msg
	}
}

func (m *FolderInbox) updateMoveOverlay(msg tea.Msg) (tea.Model, tea.Cmd) {
	kb := config.Keybinds
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case kb.Global.Cancel:
			m.movingEmail = false
			return m, nil
		case "up", kb.Global.NavUp:
			m.moveTargetIdx--
			if m.moveTargetIdx < 0 {
				m.moveTargetIdx = len(m.moveFolderChoices()) - 1
			}
			return m, nil
		case "down", kb.Global.NavDown:
			m.moveTargetIdx++
			choices := m.moveFolderChoices()
			if m.moveTargetIdx >= len(choices) {
				m.moveTargetIdx = 0
			}
			return m, nil
		case "enter":
			choices := m.moveFolderChoices()
			if len(choices) > 0 && m.moveTargetIdx < len(choices) {
				destFolder := choices[m.moveTargetIdx]
				m.movingEmail = false

				if len(m.moveUIDs) > 1 {
					// Batch move
					uids := m.moveUIDs
					m.moveUIDs = nil

					// Exit visual mode in inbox
					m.inbox.visualMode = false
					m.inbox.selectedUIDs = make(map[uint32]string)
					m.inbox.selectionOrder = []uint32{}
					m.inbox.updateListTitle()

					return m, func() tea.Msg {
						return BatchMoveEmailsMsg{
							UIDs:         uids,
							AccountID:    m.moveAccountID,
							SourceFolder: m.moveSourceFolder,
							DestFolder:   destFolder,
						}
					}
				} else {
					// Single move
					return m, func() tea.Msg {
						return MoveEmailToFolderMsg{
							UID:          m.moveUID,
							AccountID:    m.moveAccountID,
							SourceFolder: m.moveSourceFolder,
							DestFolder:   destFolder,
						}
					}
				}
			}
		}
	}
	return m, nil
}

// moveFolderChoices returns all folders except the current one.
func (m *FolderInbox) moveFolderChoices() []string {
	var choices []string
	for _, f := range m.folders {
		if f != m.currentFolder {
			choices = append(choices, f)
		}
	}
	return choices
}

func (m *FolderInbox) switchFolder() tea.Cmd {
	if m.activeFolderIdx >= 0 && m.activeFolderIdx < len(m.folders) {
		prevFolder := m.currentFolder
		m.currentFolder = m.folders[m.activeFolderIdx]
		m.isLoadingEmails = true
		m.inbox.SetFolderName(m.currentFolder)
		// Clear current emails while loading
		m.inbox.SetEmails(nil, m.accounts)
		folder := m.currentFolder
		return func() tea.Msg {
			return SwitchFolderMsg{FolderName: folder, PreviousFolder: prevFolder}
		}
	}
	return nil
}

func (m *FolderInbox) View() tea.View {
	// Render sidebar
	sidebar := m.renderSidebar()

	var content string

	if m.previewPane != nil {
		// Three-pane layout: folders | inbox | email preview
		inboxPane := m.renderInboxPane()
		previewPane := m.renderPreviewPane()
		content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, inboxPane, previewPane)
	} else if m.previewedUID != 0 {
		// Split pane loading state (body being fetched)
		inboxPane := m.renderInboxPane()
		emptyPreview := m.renderEmptyPreview()
		content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, inboxPane, emptyPreview)
	} else {
		// Two-pane layout (original): folders | inbox
		inboxView := m.inbox.View().Content
		content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, inboxView)
	}

	// If move overlay is active, render it on top
	if m.movingEmail {
		content = m.renderWithMoveOverlay(content)
	}

	return tea.NewView(content)
}

func (m *FolderInbox) renderSidebar() string {
	var b strings.Builder

	// Account name as title
	title := t("folder_inbox.folders_title")
	if len(m.accounts) > 0 {
		acc := m.accounts[0]
		if acc.Name != "" {
			title = acc.Name
		} else if acc.FetchEmail != "" {
			title = acc.FetchEmail
		}
	}
	b.WriteString(sidebarTitleStyle.Render(title))
	b.WriteString("\n")

	for i, folder := range m.folders {
		displayName := m.formatFolderName(folder)
		if i == m.activeFolderIdx {
			b.WriteString(activeFolderStyle.Width(sidebarWidth - 4).Render(displayName))
		} else {
			b.WriteString(folderStyle.Render(displayName))
		}
		if i < len(m.folders)-1 {
			b.WriteString("\n")
		}
	}

	sidebarHeight := m.height
	if sidebarHeight < 1 {
		sidebarHeight = 20
	}

	return sidebarStyle.Height(sidebarHeight - 2).Render(b.String())
}

// formatFolderName makes IMAP folder names more readable.
func (m *FolderInbox) formatFolderName(name string) string {
	// Strip common IMAP prefixes for cleaner display
	name = strings.TrimPrefix(name, "[Gmail]/")
	name = strings.TrimPrefix(name, "[Google Mail]/")
	// Truncate to fit sidebar
	maxLen := sidebarWidth - 5
	if len(name) > maxLen {
		name = name[:maxLen-1] + "\u2026"
	}
	return name
}

func (m *FolderInbox) renderWithMoveOverlay(content string) string {
	choices := m.moveFolderChoices()
	if len(choices) == 0 {
		return content
	}

	var b strings.Builder
	title := t("folder_inbox.move_to_folder")
	if len(m.moveUIDs) > 1 {
		title = tn("folder_inbox.move_multiple", len(m.moveUIDs), map[string]interface{}{
			"count": len(m.moveUIDs),
		})
	}
	b.WriteString(moveOverlayTitleStyle.Render(title))
	b.WriteString("\n")

	for i, folder := range choices {
		displayName := m.formatFolderName(folder)
		if i == m.moveTargetIdx {
			b.WriteString(moveSelectedItemStyle.Render("> " + displayName))
		} else {
			b.WriteString(moveItemStyle.Render("  " + displayName))
		}
		if i < len(choices)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render(t("folder_inbox.help")))

	overlay := moveOverlayStyle.Render(b.String())

	// Place overlay in the center of content
	contentLines := strings.Split(content, "\n")
	overlayLines := strings.Split(overlay, "\n")
	contentHeight := len(contentLines)
	overlayHeight := len(overlayLines)
	overlayWidth := lipgloss.Width(overlay)

	startRow := (contentHeight - overlayHeight) / 2
	if startRow < 0 {
		startRow = 0
	}
	startCol := (m.width - overlayWidth) / 2
	if startCol < 0 {
		startCol = 0
	}

	// Overlay the box on top of the content
	for i, overlayLine := range overlayLines {
		row := startRow + i
		if row >= len(contentLines) {
			break
		}
		line := contentLines[row]
		lineWidth := lipgloss.Width(line)

		// Build the new line: prefix + overlay + suffix
		if startCol >= lineWidth {
			contentLines[row] = line + strings.Repeat(" ", startCol-lineWidth) + overlayLine
		} else {
			// We need to place the overlay at startCol
			// Due to ANSI escape codes, we can't simply slice the string
			// Instead, place the overlay line padded to the left
			contentLines[row] = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, overlayLine)
		}
	}

	return strings.Join(contentLines, "\n")
}

// SetFolders updates the folder list.
func (m *FolderInbox) SetFolders(folders []string) {
	m.folders = sortFolders(folders)
	// Keep current folder if it still exists (search sorted list)
	found := false
	for i, f := range m.folders {
		if f == m.currentFolder {
			m.activeFolderIdx = i
			found = true
			break
		}
	}
	if !found && len(m.folders) > 0 {
		m.activeFolderIdx = 0
		m.currentFolder = m.folders[0]
	}
}

// SetEmails updates the inbox emails.
func (m *FolderInbox) SetEmails(emails []fetcher.Email, accounts []config.Account) {
	m.accounts = accounts
	m.inbox.SetEmails(emails, accounts)
}

// GetCurrentFolder returns the currently selected folder name.
func (m *FolderInbox) GetCurrentFolder() string {
	return m.currentFolder
}

// GetInbox returns the embedded inbox.
func (m *FolderInbox) GetInbox() *Inbox {
	return m.inbox
}

// GetAccounts returns the accounts.
func (m *FolderInbox) GetAccounts() []config.Account {
	return m.accounts
}

// RemoveEmail removes an email from the embedded inbox.
func (m *FolderInbox) RemoveEmail(uid uint32, accountID string) {
	m.inbox.RemoveEmail(uid, accountID)
}

// updateHelpKeys refreshes the inbox help keys based on preview state
func (m *FolderInbox) updateHelpKeys() {
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next folder")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev folder")),
		key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "move")),
	}
	if m.previewPane != nil || m.previewedUID != 0 {
		bindings = append(bindings,
			key.NewBinding(key.WithKeys("]"), key.WithHelp("]/[", "switch pane")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close preview")),
		)
	}
	m.inbox.extraShortHelpKeys = bindings
}

// SetLoadingEmails sets the loading state.
func (m *FolderInbox) SetLoadingEmails(loading bool) {
	m.isLoadingEmails = loading
	if loading {
		m.inbox.isFetching = true
	} else {
		m.inbox.isFetching = false
	}
	m.inbox.list.Title = m.inbox.getTitle()
}

// SetRefreshing sets the refreshing state (used when user presses "r").
func (m *FolderInbox) SetRefreshing(refreshing bool) {
	m.inbox.isRefreshing = refreshing
	m.inbox.list.Title = m.inbox.getTitle()
}

// GetFolders returns the current folder list.
func (m *FolderInbox) GetFolders() []string {
	return m.folders
}

// Helper to get the formatted inbox title
func folderInboxTitle(folder string) string {
	return fmt.Sprintf("Folder: %s", folder)
}

// renderInboxPane renders inbox with border for split pane mode
func (m *FolderInbox) renderInboxPane() string {
	inboxWidth := m.calculateInboxWidth()

	borderColor := unfocusedBorderColor
	if m.focusedPane == FocusInbox {
		borderColor = focusedBorderColor
	}

	paneStyle := inboxPaneStyle.
		BorderForeground(borderColor).
		Width(inboxWidth).
		Height(m.height)

	m.inbox.SetSize(inboxWidth-2, m.height)
	return paneStyle.Render(m.inbox.View().Content)
}

// renderPreviewPane renders email preview with border
func (m *FolderInbox) renderPreviewPane() string {
	if m.previewPane == nil {
		return m.renderEmptyPreview()
	}

	previewWidth := m.calculatePreviewWidth()

	borderColor := unfocusedBorderColor
	if m.focusedPane == FocusPreview {
		borderColor = focusedBorderColor
	}

	paneStyle := previewPaneStyle.
		BorderForeground(borderColor).
		Width(previewWidth).
		Height(m.height)

	return paneStyle.Render(m.previewPane.View().Content)
}

// renderEmptyPreview renders placeholder when no email selected
func (m *FolderInbox) renderEmptyPreview() string {
	previewWidth := m.calculatePreviewWidth()

	emptyStyle := lipgloss.NewStyle().
		Width(previewWidth).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(lipgloss.Color("240"))

	return emptyStyle.Render("Loading...")
}

// OpenSplitPreview opens the split preview pane for a specific email.
// email may be non-nil for hits coming from search results (which are not in
// m.inbox.allEmails); when set, it is used as a fallback by findEmailByUID
// so the preview can render without a follow-up lookup.
func (m *FolderInbox) OpenSplitPreview(uid uint32, accountID string, email *fetcher.Email) {
	m.previewPane = nil // Will be created when body arrives
	m.previewedUID = uid
	m.previewedAccountID = accountID
	m.previewSearchEmail = email
	m.focusedPane = FocusPreview
	// Recalculate inbox width for split mode
	inboxWidth := m.calculateInboxWidth()
	m.inbox.SetSize(inboxWidth-2, m.height)
	m.updateHelpKeys()
}

// closeSplitPreview closes the preview pane and returns to inbox-only
func (m *FolderInbox) closeSplitPreview() {
	ClearKittyGraphics()
	m.previewPane = nil
	m.previewedUID = 0
	m.previewedAccountID = ""
	m.previewSearchEmail = nil
	m.focusedPane = FocusInbox
	// Restore full inbox width
	inboxWidth := m.width - sidebarWidth - 3
	if inboxWidth < 20 {
		inboxWidth = 20
	}
	m.inbox.SetSize(inboxWidth, m.height)
	m.updateHelpKeys()
}

// findEmailByUID finds email in inbox by UID and account ID. Falls back to
// the email handed in by OpenSplitPreview so search hits that are not in
// allEmails (cross-folder or uncached) still render in the preview pane.
func (m *FolderInbox) findEmailByUID(uid uint32, accountID string) *fetcher.Email {
	for i := range m.inbox.allEmails {
		if m.inbox.allEmails[i].UID == uid && m.inbox.allEmails[i].AccountID == accountID {
			return &m.inbox.allEmails[i]
		}
	}
	if m.previewSearchEmail != nil &&
		m.previewSearchEmail.UID == uid &&
		m.previewSearchEmail.AccountID == accountID {
		return m.previewSearchEmail
	}
	return nil
}

// calculatePreviewWidth calculates width for preview pane
func (m *FolderInbox) calculatePreviewWidth() int {
	remainingWidth := m.width - sidebarWidth - 4 // 4 for borders
	inboxWidth := int(float64(remainingWidth) * 0.4)
	if inboxWidth < 30 {
		inboxWidth = 30
	}
	previewWidth := remainingWidth - inboxWidth
	if previewWidth < 40 {
		previewWidth = 40
	}
	return previewWidth
}

// calculateInboxWidth calculates width for inbox pane in split mode
func (m *FolderInbox) calculateInboxWidth() int {
	remainingWidth := m.width - sidebarWidth - 4
	inboxWidth := int(float64(remainingWidth) * 0.4)
	if inboxWidth < 30 {
		inboxWidth = 30
	}
	return inboxWidth
}
