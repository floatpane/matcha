package tui

import (
	"maps"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	overlay "github.com/floatpane/bubble-overlay"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/theme"
)

const sidebarWidth = 25

// sidebarHeaderRows is the height of the sidebar header (title, subtitle,
// blank spacer) that sits above the first folder row.
const sidebarHeaderRows = 3

var (
	sidebarStyle = lipgloss.NewStyle().
			Width(sidebarWidth).
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(true).
			PaddingRight(1).
			PaddingLeft(1)

	sidebarTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true)

	sidebarSubtitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	folderStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	folderUnreadStyle    = lipgloss.NewStyle().Bold(true)
	folderBadgeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)

	activeFolderStyle = lipgloss.NewStyle().
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

	// Split panes share a single separator: the preview pane's left (or top)
	// border. Its color shows whether the preview has focus.
	previewPaneStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				PaddingLeft(1)

	previewPaneVerticalStyle = lipgloss.NewStyle().
					BorderStyle(lipgloss.NormalBorder()).
					BorderTop(true)

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
	unread          map[string]int
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

	// Jump-to-folder overlay state (from command palette)
	jumpingToFolder bool
	jumpTargetIdx   int
	jumpFilterInput textinput.Model
	jumpFiltered    []int // indices into folders when filtering

	// Image rendering preference, propagated from config.
	disableImages bool

	// sidebarOffset is the index of the first folder row drawn in the
	// sidebar when there are more folders than rows.
	sidebarOffset int

	// overlayHit records where the move/jump overlay drew its items, for
	// mouse hit-testing.
	overlayHit overlayListHit

	// Split pane state
	previewPane        *EmailView
	previewedUID       uint32
	previewedAccountID string
	// previewSearchEmail holds an Email handed in by OpenSplitPreview for hits
	// that do not live in m.inbox.allEmails (search results across folders).
	// findEmailByUID falls back to it when allEmails has no match.
	previewSearchEmail *fetcher.Email
	focusedPane        PaneType
	// splitOrientation controls the split-pane layout direction.
	// Either config.SplitPaneHorizontal (default) or config.SplitPaneVertical.
	splitOrientation string
}

// SetSplitOrientation sets the split pane orientation. When the preview is
// already open, the caller should issue a follow-up WindowSizeMsg so the panes
// re-layout under the new orientation.
func (m *FolderInbox) SetSplitOrientation(orientation string) {
	if orientation == config.SplitPaneVertical {
		m.splitOrientation = config.SplitPaneVertical
	} else {
		m.splitOrientation = config.SplitPaneHorizontal
	}
}

func (m *FolderInbox) isVerticalSplit() bool {
	return m.splitOrientation == config.SplitPaneVertical
}

// IsVerticalSplit reports whether the split pane is oriented vertically.
func (m *FolderInbox) IsVerticalSplit() bool {
	return m.isVerticalSplit()
}

// CloseSplitPreview closes the split preview pane and returns to inbox-only.
func (m *FolderInbox) CloseSplitPreview() {
	m.closeSplitPreview()
}

// GetPreviewedUID returns the UID of the email currently shown in the split
// preview pane, or 0 when no preview is open.
func (m *FolderInbox) GetPreviewedUID() uint32 {
	return m.previewedUID
}

// GetPreviewedAccountID returns the account ID of the email currently shown
// in the split preview pane, or "" when no preview is open.
func (m *FolderInbox) GetPreviewedAccountID() string {
	return m.previewedAccountID
}

func (m *FolderInbox) GetUnreadCountsCopy() map[string]int {
	if m.unread == nil {
		return make(map[string]int)
	}
	result := make(map[string]int)
	maps.Copy(result, m.unread)
	return result
}

// sortFolders sorts folder names with INBOX always first, then alphabetically.
func sortFolders(folders []string) []string {
	sorted := make([]string, len(folders))
	copy(sorted, folders)
	sort.SliceStable(sorted, func(i, j int) bool {
		iUpper := strings.ToUpper(sorted[i])
		jUpper := strings.ToUpper(sorted[j])
		if iUpper == keyINBOX {
			return true
		}
		if jUpper == keyINBOX {
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

// SetDetailedDates propagates the detailed date display toggle.
func (m *FolderInbox) SetDetailedDates(enabled bool) {
	if m.inbox != nil {
		m.inbox.SetDetailedDates(enabled)
	}
}

// SetDefaultThreaded propagates the global default threading toggle.
func (m *FolderInbox) SetDefaultThreaded(v bool) {
	if m.inbox != nil {
		m.inbox.SetDefaultThreaded(v)
	}
}

// SetDisableImages propagates the global image-display preference. Affects
// future split-view previews; an already-open preview keeps its current state.
func (m *FolderInbox) SetDisableImages(v bool) {
	m.disableImages = v
}

// NewFolderInbox creates a new FolderInbox with the given folders and accounts.
func NewFolderInbox(folders []string, accounts []config.Account) *FolderInbox {
	folders = sortFolders(folders)
	currentFolder := keyINBOX
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

func (m *FolderInbox) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocyclo
	switch msg.(type) {
	case remoteImagesLoadedMsg, LoadRemoteImagesMsg:
		if m.previewPane != nil {
			_, cmd := m.previewPane.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	// If move overlay is active, handle its input
	if m.movingEmail {
		return m.updateMoveOverlay(msg)
	}

	// If jump-to-folder overlay is active, handle its input
	if m.jumpingToFolder {
		return m.updateJumpOverlay(msg)
	}

	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		return m, m.handleMouseWheel(msg)

	case tea.MouseClickMsg:
		return m, m.handleMouseClick(msg)

	case tea.KeyPressMsg:
		// Don't intercept keys while filtering
		if m.inbox.list.FilterState() == list.Filtering {
			break
		}

		// Don't intercept keys while the inbox search overlay is active.
		// Otherwise folder-level bindings like "m" (move) would shadow text input.
		if m.inbox.searchOverlay != nil {
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
			}
			// Single move
			selectedItem, ok := m.inbox.list.SelectedItem().(item)
			if ok && selectedItem.uid != 0 {
				m.movingEmail = true
				m.moveTargetIdx = 0
				m.moveUID = selectedItem.uid
				m.moveUIDs = []uint32{selectedItem.uid}
				m.moveAccountID = selectedItem.accountID
				m.moveSourceFolder = m.currentFolder
				return m, nil
			}
		}

	case JumpToFolderMsg:
		// Start jump-to-folder flow from command palette
		m.jumpingToFolder = true
		m.jumpTargetIdx = 0
		// Initialize filter input
		ti := textinput.New()
		ti.Placeholder = "Filter folders..."
		ti.Prompt = "> "
		ti.CharLimit = 64
		ti.SetStyles(ThemedTextInputStyles())
		m.jumpFilterInput = ti
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, m.layoutPanes()

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
		email.BodyMIMEType = msg.BodyMIMEType
		email.Attachments = msg.Attachments
		// Create the preview at the exact size and screen position of the
		// preview pane so its images line up with the text.
		r := m.previewContentRect()
		m.previewPane = NewEmailViewPreview(*email, r.w, r.h, r.x, r.y, m.disableImages)
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
// Many inbox updates (most clicks, for one) return no command, so a nil cmd
// stays nil.
func (m *FolderInbox) wrapInboxCmd(cmd tea.Cmd) tea.Cmd {
	// Capture the folder now: the command runs later, on another goroutine,
	// by which time the user may have switched folders.
	return withFolderContext(cmd, m.currentFolder)
}

func withFolderContext(cmd tea.Cmd, folder string) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch inner := cmd().(type) {
		case FetchMoreEmailsMsg:
			return FetchFolderMoreEmailsMsg{
				Offset:     inner.Offset,
				AccountID:  inner.AccountID,
				FolderName: folder,
				Limit:      inner.Limit,
			}
		case RequestRefreshMsg:
			inner.FolderName = folder
			return inner
		case SearchRequestedMsg:
			inner.FolderName = folder
			return inner
		case tea.BatchMsg:
			// The ALL tab requests more mail for each account in a batch.
			wrapped := make(tea.BatchMsg, 0, len(inner))
			for _, c := range inner {
				if c != nil {
					wrapped = append(wrapped, withFolderContext(c, folder))
				}
			}
			return wrapped
		default:
			return inner
		}
	}
}

func (m *FolderInbox) updateMoveOverlay(msg tea.Msg) (tea.Model, tea.Cmd) {
	kb := config.Keybinds
	if count := len(m.moveFolderChoices()); count > 0 {
		switch action, idx := m.overlayHit.mouse(msg, m.moveTargetIdx, count); action {
		case overlayMouseSelect:
			m.moveTargetIdx = idx
			return m, nil
		case overlayMouseActivate:
			m.moveTargetIdx = idx
			return m.updateMoveOverlay(tea.KeyPressMsg{Code: tea.KeyEnter})
		case overlayMouseDismiss:
			m.movingEmail = false
			return m, nil
		case overlayMouseNone:
		}
	}
	if msg, ok := msg.(tea.KeyPressMsg); ok {
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
		case keyDown, kb.Global.NavDown:
			m.moveTargetIdx++
			choices := m.moveFolderChoices()
			if m.moveTargetIdx >= len(choices) {
				m.moveTargetIdx = 0
			}
			return m, nil
		case keyEnter:
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
				}
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
	return m, nil
}

func (m *FolderInbox) updateJumpOverlay(msg tea.Msg) (tea.Model, tea.Cmd) {
	kb := config.Keybinds
	if count := len(m.getJumpList()); count > 0 {
		switch action, idx := m.overlayHit.mouse(msg, m.jumpTargetIdx, count); action {
		case overlayMouseSelect:
			m.jumpTargetIdx = idx
			return m, nil
		case overlayMouseActivate:
			m.jumpTargetIdx = idx
			m.jumpFilterInput.Blur()
			return m.updateJumpOverlay(tea.KeyPressMsg{Code: tea.KeyEnter})
		case overlayMouseDismiss:
			m.jumpingToFolder = false
			m.jumpFilterInput.Blur()
			m.jumpFilterInput.SetValue("")
			m.jumpFiltered = nil
			return m, nil
		case overlayMouseNone:
		}
	}

	// If filter input is focused, route all keys to it
	if m.jumpFilterInput.Focused() {
		if msg, ok := msg.(tea.KeyPressMsg); ok {
			switch msg.String() {
			case keyEnter:
				// Enter exits filter mode and focuses back on list
				m.jumpFilterInput.Blur()
				return m, nil
			case kb.Global.Cancel:
				// Esc cancels the entire jump overlay
				m.jumpingToFolder = false
				m.jumpFilterInput.Blur()
				m.jumpFilterInput.SetValue("")
				m.jumpFiltered = nil
				return m, nil
			default:
				// All other keys go to the filter input
				var cmd tea.Cmd
				m.jumpFilterInput, cmd = m.jumpFilterInput.Update(msg)
				// Apply filter after each keystroke
				m.applyJumpFilter()
				m.jumpTargetIdx = 0
				return m, cmd
			}
		}
		return m, nil
	}

	// Filter input not focused - handle navigation
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case kb.Global.Cancel:
			m.jumpingToFolder = false
			m.jumpFilterInput.SetValue("")
			m.jumpFiltered = nil
			return m, nil
		case kb.Inbox.Filter:
			// Start filtering - focus the input
			m.jumpFilterInput.Focus()
			return m, textinput.Blink
		case "up", kb.Global.NavUp:
			list := m.getJumpList()
			if len(list) > 0 {
				m.jumpTargetIdx--
				if m.jumpTargetIdx < 0 {
					m.jumpTargetIdx = len(list) - 1
				}
			}
			return m, nil
		case keyDown, kb.Global.NavDown:
			list := m.getJumpList()
			if len(list) > 0 {
				m.jumpTargetIdx++
				if m.jumpTargetIdx >= len(list) {
					m.jumpTargetIdx = 0
				}
			}
			return m, nil
		case keyEnter:
			list := m.getJumpList()
			if len(list) > 0 && m.jumpTargetIdx < len(list) {
				var targetFolder string
				if len(m.jumpFiltered) > 0 {
					targetFolder = m.folders[m.jumpFiltered[m.jumpTargetIdx]]
				} else {
					targetFolder = m.folders[m.jumpTargetIdx]
				}
				m.jumpingToFolder = false
				m.jumpFilterInput.SetValue("")
				m.jumpFiltered = nil
				// Find the index of the target folder
				for i, f := range m.folders {
					if f == targetFolder {
						m.activeFolderIdx = i
						break
					}
				}
				return m, m.switchFolder()
			}
		}
	}
	return m, nil
}

// applyJumpFilter filters the folder list based on the current filter string
func (m *FolderInbox) applyJumpFilter() {
	query := strings.ToLower(m.jumpFilterInput.Value())
	if query == "" {
		m.jumpFiltered = nil
		return
	}

	m.jumpFiltered = m.jumpFiltered[:0]
	for i, folder := range m.folders {
		if strings.Contains(strings.ToLower(folder), query) {
			m.jumpFiltered = append(m.jumpFiltered, i)
		}
	}
}

// getJumpList returns the current list of folders (filtered or all)
func (m *FolderInbox) getJumpList() []int {
	if len(m.jumpFiltered) > 0 {
		return m.jumpFiltered
	}
	// Return indices for all folders
	all := make([]int, len(m.folders))
	for i := range m.folders {
		all[i] = i
	}
	return all
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
	var main string
	if m.hasSplit() {
		inboxPane := m.renderInboxPane()
		previewPane := m.renderPreviewPane()
		if m.isVerticalSplit() {
			// Folders | (inbox stacked above preview)
			main = lipgloss.JoinVertical(lipgloss.Left, inboxPane, previewPane)
		} else {
			// Folders | inbox | email preview
			main = lipgloss.JoinHorizontal(lipgloss.Top, inboxPane, previewPane)
		}
	} else {
		main = m.renderInboxPane()
	}

	content := main
	// The sidebar is hidden when a plugin toggles it off; the inbox then
	// expands to the full width (see effectiveSidebarWidth).
	if m.effectiveSidebarWidth() > 0 {
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.renderSidebar(), main)
	}
	content = fitBox(content, m.width, m.height)

	// If move overlay is active, render it on top
	if m.movingEmail {
		content = m.renderWithMoveOverlay(content)
	}

	// If jump-to-folder overlay is active, render it on top
	if m.jumpingToFolder {
		content = m.renderWithJumpOverlay(content)
	}

	// Composite plugin-injected custom components (floating overlays and
	// anchored blocks) onto the final content.
	content = renderCustomComponents(content, m.width, m.height)

	return tea.NewView(content)
}

// sidebarHeaderRowCount is the number of sidebar rows above the first folder.
func (m *FolderInbox) sidebarHeaderRowCount() int {
	if isUIVisible("header") {
		return sidebarHeaderRows
	}
	return 0
}

// sidebarFolderRows is how many folder rows fit in the sidebar.
func (m *FolderInbox) sidebarFolderRows() int {
	return max(1, m.availableHeight()-m.sidebarHeaderRowCount())
}

// ensureActiveFolderVisible scrolls the sidebar so the active folder is shown.
func (m *FolderInbox) ensureActiveFolderVisible() {
	rows := m.sidebarFolderRows()
	if m.activeFolderIdx < m.sidebarOffset {
		m.sidebarOffset = m.activeFolderIdx
	}
	if m.activeFolderIdx >= m.sidebarOffset+rows {
		m.sidebarOffset = m.activeFolderIdx - rows + 1
	}
	m.sidebarOffset = max(0, min(m.sidebarOffset, len(m.folders)-rows))
}

func (m *FolderInbox) renderSidebar() string {
	contentWidth := max(1, sidebarWidth-sidebarStyle.GetHorizontalFrameSize())
	var b strings.Builder

	// The sidebar header (account name and address) is part of the "header"
	// component. Skip it when a plugin has hidden the header.
	if isUIVisible("header") {
		title := t("folder_inbox.folders_title")
		subtitle := ""
		if acc, ok := m.sidebarAccount(); ok {
			switch {
			case acc.Name != "":
				title = acc.Name
				subtitle = acc.FetchEmail
			case acc.FetchEmail != "":
				title = acc.FetchEmail
			}
		}
		b.WriteString(sidebarTitleStyle.Render(ansi.Truncate(title, contentWidth, "\u2026")))
		b.WriteString("\n")
		b.WriteString(sidebarSubtitleStyle.Render(ansi.Truncate(subtitle, contentWidth, "\u2026")))
		b.WriteString("\n\n")
	}

	m.ensureActiveFolderVisible()
	end := min(len(m.folders), m.sidebarOffset+m.sidebarFolderRows())
	for i := m.sidebarOffset; i < end; i++ {
		b.WriteString(m.renderFolderRow(m.folders[i], i == m.activeFolderIdx, contentWidth))
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	height := m.availableHeight()
	return sidebarStyle.Height(height).MaxHeight(height).Render(b.String())
}

// sidebarAccount returns the account whose header the sidebar should show. It
// follows the inbox's active account tab; the "ALL" tab has no single account,
// so the header falls back to the generic folders title.
func (m *FolderInbox) sidebarAccount() (config.Account, bool) {
	if len(m.accounts) == 0 {
		return config.Account{}, false
	}
	id := m.inbox.currentAccountID
	if id == "" && len(m.accounts) > 1 {
		return config.Account{}, false
	}
	for _, acc := range m.accounts {
		if acc.ID == id {
			return acc, true
		}
	}
	return m.accounts[0], true
}

// renderFolderRow draws one sidebar row: icon, name, and a right-aligned
// unread count. The active row is filled edge to edge.
func (m *FolderInbox) renderFolderRow(folder string, active bool, width int) string {
	unread := m.unread[folder]
	badge := ""
	badgeWidth := 0
	if unread > 0 {
		badge = strconv.Itoa(unread)
		badgeWidth = lipgloss.Width(badge) + 1
	}

	icon := folderIcon(folder)
	// Leading space, icon, space, name, gap, badge, trailing space.
	fixed := 3 + lipgloss.Width(icon) + badgeWidth
	name := ansi.Truncate(m.formatFolderName(folder), max(width-fixed, 0), "\u2026")
	gap := strings.Repeat(" ", max(width-3-lipgloss.Width(icon)-lipgloss.Width(name)-lipgloss.Width(badge), 0))

	if active {
		return activeFolderStyle.Width(width).Render(" " + icon + " " + name + gap + badge)
	}
	nameStyle := folderStyle
	if unread > 0 {
		nameStyle = folderUnreadStyle
	}
	return folderStyle.Render(" "+icon+" ") + nameStyle.Render(name) + gap + folderBadgeStyle.Render(badge)
}

// folderIcon picks a Nerd Font glyph for well-known folder names.
func folderIcon(folder string) string {
	name := strings.ToLower(folder)
	switch {
	case strings.Contains(name, "inbox"):
		return "\ueb1c"
	case strings.Contains(name, "sent"):
		return "\uf1d8"
	case strings.Contains(name, "draft"):
		return "\uec0e"
	case strings.Contains(name, "trash"), strings.Contains(name, "deleted"):
		return "\uf1f8"
	case strings.Contains(name, "archive"), strings.Contains(name, "all mail"):
		return "\uf187"
	case strings.Contains(name, "spam"), strings.Contains(name, "junk"):
		return "\uf05e"
	case strings.Contains(name, "star"), strings.Contains(name, "flag"):
		return "\uf005"
	}
	return "\uf07b"
}

// formatFolderName makes IMAP folder names more readable.
func (m *FolderInbox) formatFolderName(name string) string {
	// Strip common IMAP prefixes for cleaner display
	name = strings.TrimPrefix(name, "[Gmail]/")
	name = strings.TrimPrefix(name, "[Google Mail]/")
	// Truncate to fit sidebar without splitting multi-byte characters.
	return ansi.Truncate(name, sidebarWidth-5, "\u2026")
}

func (m *FolderInbox) renderWithMoveOverlay(content string) string {
	choices := m.moveFolderChoices()
	if len(choices) == 0 {
		return content
	}

	title := t("folder_inbox.move_to_folder")
	if len(m.moveUIDs) > 1 {
		title = tn("folder_inbox.move_multiple", len(m.moveUIDs), map[string]interface{}{
			keyCount: len(m.moveUIDs),
		})
	}
	header := moveOverlayTitleStyle.Render(title) + "\n"
	footer := "\n\n" + m.overlayHelp(keybindHelp("folder_inbox.help"))
	names := make([]string, len(choices))
	for i, folder := range choices {
		names[i] = m.formatFolderName(folder)
	}
	return m.renderOverlayList(content, header, names, m.moveTargetIdx, footer)
}

// overlayHelp wraps an overlay's help line so the box fits the screen width.
func (m *FolderInbox) overlayHelp(text string) string {
	inner := max(10, m.width-moveOverlayStyle.GetHorizontalFrameSize())
	if lipgloss.Width(text) <= inner {
		return helpStyle.Render(text)
	}
	return helpStyle.Width(inner).Render(text)
}

// overlayListHit is the screen geometry of an overlay's visible items.
type overlayListHit struct {
	top, left, width int // first item row and the box's columns
	first, count     int // list index of the first visible item, item count
}

type overlayMouseAction int

const (
	overlayMouseNone overlayMouseAction = iota
	overlayMouseSelect
	overlayMouseActivate
	overlayMouseDismiss
)

// mouse maps a mouse event on an overlay list to an action. The wheel moves
// the selection, clicking an item selects it (clicking the selected item
// activates it) and clicking outside the box dismisses the overlay.
func (h overlayListHit) mouse(msg tea.Msg, selected, total int) (overlayMouseAction, int) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			return overlayMouseSelect, min(selected+1, total-1)
		case tea.MouseWheelUp:
			return overlayMouseSelect, max(selected-1, 0)
		default:
		}
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || h.width == 0 {
			return overlayMouseNone, selected
		}
		if msg.X < h.left || msg.X >= h.left+h.width {
			return overlayMouseDismiss, selected
		}
		row := msg.Y - h.top
		if row < 0 || row >= h.count {
			return overlayMouseNone, selected
		}
		idx := h.first + row
		if idx >= total {
			return overlayMouseNone, selected
		}
		if idx == selected {
			return overlayMouseActivate, idx
		}
		return overlayMouseSelect, idx
	}
	return overlayMouseNone, selected
}

// renderOverlayList draws a framed list centered over content, showing a
// window of items around the selection that fits the screen, and records the
// item geometry for mouse hit-testing.
func (m *FolderInbox) renderOverlayList(content, header string, items []string, selected int, footer string) string {
	// The box holds the header lines, one line per visible item and the
	// footer lines; keep it within the screen height.
	room := m.height - moveOverlayStyle.GetVerticalFrameSize() - strings.Count(header, "\n") - strings.Count(footer, "\n")
	maxVisible := max(1, min(len(items), room))
	start := 0
	if selected >= maxVisible {
		start = selected - maxVisible + 1
	}
	end := min(len(items), start+maxVisible)
	maxName := max(8, m.width-moveOverlayStyle.GetHorizontalFrameSize()-moveItemStyle.GetHorizontalFrameSize()-2)

	var b strings.Builder
	b.WriteString(header)
	for i := start; i < end; i++ {
		name := ansi.Truncate(items[i], maxName, "\u2026")
		if i == selected {
			b.WriteString(moveSelectedItemStyle.Render("▌ " + name))
		} else {
			b.WriteString(moveItemStyle.Render("  " + name))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString(footer)
	box := moveOverlayStyle.Render(b.String())

	// Same placement as overlay.Center.
	boxW, boxH := lipgloss.Width(box), lipgloss.Height(box)
	top := max(0, (m.height-boxH)/2)
	left := max(0, (m.width-boxW)/2)
	m.overlayHit = overlayListHit{
		top:   top + moveOverlayStyle.GetBorderTopSize() + moveOverlayStyle.GetPaddingTop() + strings.Count(header, "\n"),
		left:  left,
		width: boxW,
		first: start,
		count: end - start,
	}

	// Composite the box as a floating layer centered over the content, the same
	// way the command palette does. The background is preserved on all sides;
	// only the box's own cells are replaced.
	return overlay.Center(content, box, m.width, m.height)
}

func (m *FolderInbox) renderWithJumpOverlay(content string) string {
	if len(m.folders) == 0 {
		return content
	}

	var header strings.Builder
	// Show filter input if focused, otherwise just the title
	if m.jumpFilterInput.Focused() {
		header.WriteString(moveOverlayTitleStyle.Render("Filter folders:"))
		header.WriteString("\n")
		m.jumpFilterInput.SetWidth(max(10, min(40, m.width-12)))
		header.WriteString(m.jumpFilterInput.View())
		header.WriteString("\n\n")
	} else {
		title := t("folder_inbox.jump_to_folder")
		if m.jumpFilterInput.Value() != "" {
			title = title + " [" + m.jumpFilterInput.Value() + "]"
		}
		header.WriteString(moveOverlayTitleStyle.Render(title))
		header.WriteString("\n")
	}

	// Get the current list (filtered or all)
	indices := m.getJumpList()
	names := make([]string, len(indices))
	for i, folderIdx := range indices {
		names[i] = m.formatFolderName(m.folders[folderIdx])
	}

	var footer strings.Builder
	// Show message if no matches
	if len(indices) == 0 {
		footer.WriteString(lipgloss.NewStyle().Foreground(theme.ActiveTheme.MutedText).Render("No matching folders"))
	}
	footer.WriteString("\n\n")

	// Update help text based on filter state
	kb := config.Keybinds
	cancel := kbKey(kb.Global.Cancel, "esc")
	var helpText string
	switch {
	case m.jumpFilterInput.Focused():
		helpText = joinHelp(
			helpItem("", keyEnter, "apply filter"),
			helpItem("", cancel, "cancel"),
		)
	case m.jumpFilterInput.Value() != "":
		helpText = joinHelp(
			helpItem("", filterKey(), "clear filter"),
			helpItem("", navVimLabel(), "navigate"),
			helpItem("", keyEnter, "jump"),
			helpItem("", cancel, "cancel"),
		)
	default:
		helpText = joinHelp(
			helpItem("", filterKey(), "filter folders"),
			helpItem("", navVimLabel(), "navigate"),
			helpItem("", keyEnter, "jump"),
			helpItem("", cancel, "cancel"),
		)
	}
	footer.WriteString(m.overlayHelp(helpText))

	return m.renderOverlayList(content, header.String(), names, m.jumpTargetIdx, footer.String())
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

func (m *FolderInbox) SetUnreadCounts(counts map[string]int) {
	m.unread = counts
}

func (m *FolderInbox) DecrementUnreadCount(folder string) {
	if m.unread == nil {
		return
	}
	if m.unread[folder] > 0 {
		m.unread[folder]--
	}
}

// IncrementUnreadCount increases the unread counter for a folder. It is used to
// restore the counter when a delete/archive action is undone.
func (m *FolderInbox) IncrementUnreadCount(folder string) {
	if m.unread == nil {
		m.unread = make(map[string]int)
	}
	m.unread[folder]++
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

// HasSplitPreview reports whether the split preview pane is currently open.
func (m *FolderInbox) HasSplitPreview() bool {
	return m.previewPane != nil
}

// GetPreviewPane returns the split preview EmailView, or nil when no preview
// is open.
func (m *FolderInbox) GetPreviewPane() *EmailView {
	return m.previewPane
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
	kb := config.Keybinds
	nextFolder := kbKey(kb.Folder.NextFolder, "tab")
	prevFolder := kbKey(kb.Folder.PrevFolder, "shift+tab")
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys(nextFolder), key.WithHelp(nextFolder, "next folder")),
		key.NewBinding(key.WithKeys(prevFolder), key.WithHelp(prevFolder, "prev folder")),
	}
	if m.previewPane != nil || m.previewedUID != 0 {
		switchPane := joinKeys(kbKey(kb.Folder.FocusPreview, "]"), kbKey(kb.Folder.FocusInbox, "["))
		bindings = append(bindings,
			key.NewBinding(key.WithKeys(kbKey(kb.Folder.FocusPreview, "]")), key.WithHelp(switchPane, "switch pane")),
			key.NewBinding(key.WithKeys(kbKey(kb.Global.Cancel, "esc")), key.WithHelp(kbKey(kb.Global.Cancel, "esc"), "close preview")),
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

// paneRect is a rectangle of screen cells.
type paneRect struct {
	x, y, w, h int
}

func (r paneRect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

// hasSplit reports whether the split preview (or its loading placeholder) is shown.
func (m *FolderInbox) hasSplit() bool {
	return m.previewPane != nil || m.previewedUID != 0
}

// contentRect is the area right of the sidebar.
func (m *FolderInbox) contentRect() paneRect {
	s := m.effectiveSidebarWidth()
	return paneRect{x: s, y: 0, w: max(1, m.width-s), h: m.availableHeight()}
}

// inboxPaneRect is the area of the email list.
func (m *FolderInbox) inboxPaneRect() paneRect {
	c := m.contentRect()
	if !m.hasSplit() {
		return c
	}
	if m.isVerticalSplit() {
		return paneRect{x: c.x, y: c.y, w: c.w, h: splitFirstPaneSize(c.h, 5)}
	}
	return paneRect{x: c.x, y: c.y, w: splitFirstPaneSize(c.w, 30), h: c.h}
}

// previewPaneRect is the whole preview pane, including its separator border.
func (m *FolderInbox) previewPaneRect() paneRect {
	c := m.contentRect()
	in := m.inboxPaneRect()
	if m.isVerticalSplit() {
		return paneRect{x: c.x, y: c.y + in.h, w: c.w, h: max(1, c.h-in.h)}
	}
	return paneRect{x: c.x + in.w, y: c.y, w: max(1, c.w-in.w), h: c.h}
}

// previewContentRect is where the email view is drawn inside the preview pane.
func (m *FolderInbox) previewContentRect() paneRect {
	p := m.previewPaneRect()
	if m.isVerticalSplit() {
		top := previewPaneVerticalStyle.GetBorderTopSize()
		return paneRect{x: p.x, y: p.y + top, w: p.w, h: max(1, p.h-top)}
	}
	left := previewPaneStyle.GetBorderLeftSize() + previewPaneStyle.GetPaddingLeft()
	return paneRect{x: p.x + left, y: p.y, w: max(1, p.w-left), h: p.h}
}

// splitFirstPaneSize returns the size of the first (inbox) pane of a split
// that is total cells long: 40%, but at least minPane, leaving the second
// pane at least minPane too when possible.
func splitFirstPaneSize(total, minPane int) int {
	size := max(total*2/5, minPane)
	if total-size < minPane {
		size = total / 2
	}
	return max(1, min(size, total-1))
}

// layoutPanes sizes the inbox and preview to the current layout. It returns
// the preview's follow-up command, if any (remote image reloads).
func (m *FolderInbox) layoutPanes() tea.Cmd {
	in := m.inboxPaneRect()
	m.inbox.SetSize(in.w, in.h)
	m.ensureActiveFolderVisible()
	if m.previewPane == nil {
		return nil
	}
	r := m.previewContentRect()
	m.previewPane.SetColumnOffset(r.x)
	m.previewPane.SetRowOffset(r.y)
	if r.w == m.previewPane.width && r.h == m.previewPane.height {
		return nil
	}
	_, cmd := m.previewPane.Update(tea.WindowSizeMsg{Width: r.w, Height: r.h})
	return cmd
}

// renderInboxPane renders the email list at exactly its pane size.
func (m *FolderInbox) renderInboxPane() string {
	in := m.inboxPaneRect()
	return fitBox(m.inbox.View().Content, in.w, in.h)
}

// renderPreviewPane renders the preview (or its loading placeholder) with the
// separator border that divides it from the inbox.
func (m *FolderInbox) renderPreviewPane() string {
	r := m.previewContentRect()
	borderColor := unfocusedBorderColor
	if m.focusedPane == FocusPreview {
		borderColor = focusedBorderColor
	}

	var body string
	if m.previewPane == nil {
		body = lipgloss.NewStyle().
			Width(r.w).
			Height(r.h).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(theme.ActiveTheme.MutedText).
			Render("Loading...")
		body = fitBox(body, r.w, r.h)
	} else {
		body = fitBox(m.previewPane.View().Content, r.w, r.h)
	}

	style := previewPaneStyle
	if m.isVerticalSplit() {
		style = previewPaneVerticalStyle
	}
	return style.BorderForeground(borderColor).Render(body)
}

// VisibleImages reports the preview's out-of-band images. Images are hidden
// while an overlay is drawn on top of the preview.
func (m *FolderInbox) VisibleImages() []ScreenImage {
	if m.previewPane == nil || m.movingEmail || m.jumpingToFolder || m.inbox.IsSearchOverlayOpen() {
		return nil
	}
	return m.previewPane.VisibleImages()
}

// handleMouseWheel scrolls the pane under the pointer. Over the sidebar the
// wheel moves to the previous/next folder.
func (m *FolderInbox) handleMouseWheel(msg tea.MouseWheelMsg) tea.Cmd {
	if msg.X < m.effectiveSidebarWidth() {
		if len(m.folders) == 0 {
			return nil
		}
		next := m.activeFolderIdx
		switch msg.Button {
		case tea.MouseWheelDown:
			next++
		case tea.MouseWheelUp:
			next--
		default:
			return nil
		}
		if next < 0 || next >= len(m.folders) {
			return nil
		}
		m.activeFolderIdx = next
		return m.switchFolder()
	}
	if m.hasSplit() && m.previewPaneRect().contains(msg.X, msg.Y) {
		if m.previewPane == nil {
			return nil
		}
		_, cmd := m.previewPane.Update(msg)
		return cmd
	}
	in := m.inboxPaneRect()
	if !in.contains(msg.X, msg.Y) {
		return nil
	}
	local := msg
	local.X -= in.x
	local.Y -= in.y
	_, cmd := m.inbox.Update(local)
	return m.wrapInboxCmd(cmd)
}

// handleMouseClick routes a click to the folder list, the inbox or the
// preview, focusing the pane that was clicked.
func (m *FolderInbox) handleMouseClick(msg tea.MouseClickMsg) tea.Cmd {
	if msg.Button != tea.MouseLeft {
		return nil
	}
	if msg.X < m.effectiveSidebarWidth() {
		row := msg.Y - m.sidebarHeaderRowCount()
		if row < 0 || row >= m.sidebarFolderRows() {
			return nil
		}
		idx := m.sidebarOffset + row
		if idx >= len(m.folders) || idx == m.activeFolderIdx {
			return nil
		}
		m.activeFolderIdx = idx
		return m.switchFolder()
	}
	if m.hasSplit() && m.previewPaneRect().contains(msg.X, msg.Y) {
		m.focusedPane = FocusPreview
		if m.previewPane == nil {
			return nil
		}
		// The email view maps clicks with its own screen offset.
		_, cmd := m.previewPane.Update(msg)
		return cmd
	}
	in := m.inboxPaneRect()
	if !in.contains(msg.X, msg.Y) {
		return nil
	}
	if m.hasSplit() {
		m.focusedPane = FocusInbox
	}
	local := msg
	local.X -= in.x
	local.Y -= in.y
	_, cmd := m.inbox.Update(local)
	return m.wrapInboxCmd(cmd)
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
	m.layoutPanes()
	m.updateHelpKeys()
}

// closeSplitPreview closes the preview pane and returns to inbox-only
func (m *FolderInbox) closeSplitPreview() {
	m.previewPane = nil
	m.previewedUID = 0
	m.previewedAccountID = ""
	m.previewSearchEmail = nil
	m.focusedPane = FocusInbox
	m.layoutPanes()
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

// effectiveSidebarWidth returns the width consumed by the sidebar (including
// its right border and padding) when visible, or 0 when a plugin has hidden it
// via matcha.ui.set_visible("sidebar", false). All width-calculation methods
// call this so the inbox and preview panes expand to fill the freed space.
func (m *FolderInbox) effectiveSidebarWidth() int {
	if !isUIVisible("sidebar") {
		return 0
	}
	// Leave the inbox some room on very narrow terminals.
	if m.width > 0 && m.width < sidebarWidth+20 {
		return 0
	}
	return sidebarWidth
}

// availableHeight returns the number of rows the folder inbox may use. The
// inbox draws its own help bar on its last row, so this is the full height.
func (m *FolderInbox) availableHeight() int {
	return max(1, m.height)
}

// fitBox pads or truncates s so that it is exactly width cells wide and
// height lines tall. It keeps panes from pushing their neighbours around.
func fitBox(s string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		w := ansi.StringWidth(line)
		if w > width {
			lines[i] = ansi.Truncate(line, width, "")
			w = ansi.StringWidth(lines[i])
		}
		if w < width {
			lines[i] += strings.Repeat(" ", width-w)
		}
	}
	blank := strings.Repeat(" ", width)
	for len(lines) < height {
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}
