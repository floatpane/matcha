package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/internal/httpclient"
	"github.com/floatpane/matcha/internal/marketplace"
	"github.com/floatpane/matcha/plugins"
)

var (
	mpTitleStyle     lipgloss.Style
	mpItemNameStyle  lipgloss.Style
	mpItemDescStyle  lipgloss.Style
	mpInstalledStyle lipgloss.Style
	mpSelectedStyle  lipgloss.Style
	mpCursorStyle    lipgloss.Style
	mpStatusStyle    lipgloss.Style
	mpErrorStyle     lipgloss.Style
	mpFooterStyle    lipgloss.Style
	mpAuthorStyle    lipgloss.Style
	mpVerifiedStyle  lipgloss.Style
)

type marketplaceState int

const (
	marketplaceLoading marketplaceState = iota
	marketplaceReady
	marketplaceError
)

// RegistryFetchedMsg signals that the plugin registry was fetched.
type RegistryFetchedMsg struct {
	Entries []plugins.PluginEntry
	Err     error
}

// MarketplacePlugin wraps marketplace.PluginInfo for TUI display
type MarketplacePlugin struct {
	Info marketplace.PluginInfo
	File string // derived filename
}

// PluginInstalledMsg signals that a plugin was installed from the marketplace.
type PluginInstalledMsg struct {
	Name string
	Err  error
}

// PluginDeletedMsg signals that a plugin was deleted from the marketplace.
type PluginDeletedMsg struct {
	Name string
	Err  error
}

// mpStatusClearMsg clears the transient status line, unless a newer status
// replaced the one it was scheduled for.
type mpStatusClearMsg struct{ seq int }

const (
	// mpStatusTTL is how long a result message stays in the status line.
	mpStatusTTL = 4 * time.Second
	// mpErrorTTL is how long an error stays in the status line.
	mpErrorTTL = 8 * time.Second
	// mpEntryHeight is the number of rows one plugin entry takes.
	mpEntryHeight = 2
	// mpDoubleClick is the longest gap between two clicks on the same row
	// that still counts as a double click.
	mpDoubleClick = 500 * time.Millisecond
)

type Marketplace struct {
	entries           []plugins.PluginEntry
	installed         map[string]bool
	cursor            int
	offset            int // scroll offset
	width             int
	height            int
	state             marketplaceState
	status            string // transient status message
	statusSeq         int    // bumped on every status change so stale clears are ignored
	statusBusy        bool   // status describes a running install/delete
	standalone        bool   // true when launched via `matcha marketplace` (not from main menu)
	lastClickTime     time.Time
	lastClickEntry    int  // entry of the last click, for double-click detection
	confirmingInstall bool // true when prompting the user to confirm plugin installation
	installTarget     plugins.PluginEntry
	confirmingDelete  bool   // true when prompting the user to confirm plugin removal
	deleteTarget      string // name of the plugin pending deletion
}

func NewMarketplace(standalone bool) Marketplace {
	return Marketplace{
		installed:  installedPlugins(),
		standalone: standalone,
	}
}

func (m Marketplace) Init() tea.Cmd {
	// Try new marketplace API first, fallback to old registry
	return fetchFromNewMarketplace
}

// headerView renders the logo and title drawn above the plugin list,
// including the blank line that separates them from the entries.
func (m Marketplace) headerView() string {
	return renderLogo(choiceLogo) + "\n" + mpTitleStyle.Render(" "+t("marketplace.title")+" ") + "\n"
}

// itemsStartY returns the screen row of the first visible entry, derived from
// the rendered header so a plugin banner of any height maps clicks correctly.
func (m Marketplace) itemsStartY() int {
	return docStyle.GetMarginTop() + m.headerHeight()
}

// innerWidth is the usable width inside docStyle's margins (0 = unknown).
func (m Marketplace) innerWidth() int {
	if m.width <= 0 {
		return 0
	}
	return max(1, m.width-docStyle.GetHorizontalMargins())
}

// dialogOpen reports whether a confirmation prompt is on screen. Prompts are
// modal: mouse input must not move the cursor behind them.
func (m Marketplace) dialogOpen() bool {
	return m.confirmingInstall || m.confirmingDelete
}

// setStatus shows text in the status line. A ttl > 0 clears it after that
// long; ttl == 0 marks a running operation that stays until its result.
func (m *Marketplace) setStatus(text string, ttl time.Duration) tea.Cmd {
	m.statusSeq++
	m.status = text
	m.statusBusy = ttl == 0
	if ttl == 0 {
		return nil
	}
	seq := m.statusSeq
	return tea.Tick(ttl, func(time.Time) tea.Msg { return mpStatusClearMsg{seq: seq} })
}

// clearStatus removes the status line.
func (m *Marketplace) clearStatus() {
	m.statusSeq++
	m.status = ""
	m.statusBusy = false
}

// moveCursor moves the selection by delta, keeping it on screen. Moving away
// dismisses a finished operation's message.
func (m *Marketplace) moveCursor(delta int) {
	if len(m.entries) == 0 {
		return
	}
	next := max(0, min(len(m.entries)-1, m.cursor+delta))
	if next == m.cursor {
		return
	}
	m.cursor = next
	if !m.statusBusy {
		m.clearStatus()
	}
	m.offset, _ = m.window()
}

// entryAt maps a screen row to the entry drawn there, if any. Rows below the
// last visible entry (footer, padding) map to nothing.
func (m Marketplace) entryAt(y int) (int, bool) {
	row := y - m.itemsStartY()
	start, end := m.window()
	if row < 0 || row >= (end-start)*mpEntryHeight {
		return 0, false
	}
	return start + row/mpEntryHeight, true
}

// promptInstall asks to install the entry under the cursor.
func (m Marketplace) promptInstall() (tea.Model, tea.Cmd) {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return m, nil
	}
	entry := m.entries[m.cursor]
	if m.installed[entry.Name] {
		// m is a value: update it before it is returned.
		cmd := m.setStatus(fmt.Sprintf("%s is already installed", entry.Name), mpStatusTTL)
		return m, cmd
	}
	m.confirmingInstall = true
	m.installTarget = entry
	return m, nil
}

func fetchFromNewMarketplace() tea.Msg {
	plugins, err := marketplace.ListPlugins()
	if err != nil {
		// Fallback to old registry
		return fetchRegistry()
	}

	marketplacePlugins := make([]MarketplacePlugin, len(plugins))
	for i, p := range plugins {
		marketplacePlugins[i] = MarketplacePlugin{
			Info: p,
			File: p.Name + ".lua",
		}
	}

	return RegistryFetchedMsg{
		Entries: convertToPluginEntries(marketplacePlugins),
		Err:     nil,
	}
}

func convertToPluginEntries(mpPlugins []MarketplacePlugin) []plugins.PluginEntry {
	entries := make([]plugins.PluginEntry, len(mpPlugins))
	for i, mp := range mpPlugins {
		entries[i] = plugins.PluginEntry{
			Name:              mp.Info.Name,
			Title:             mp.Info.Title,
			Description:       mp.Info.Description,
			File:              mp.File,
			URL:               mp.Info.RepositoryURL,
			AuthorDisplayName: mp.Info.Author.DisplayName,
			AuthorVerified:    mp.Info.IsTrustedAuthor(),
		}
	}
	return entries
}

func fetchRegistry() tea.Msg {
	entries, err := plugins.FetchRegistry()
	return RegistryFetchedMsg{Entries: entries, Err: err}
}

func (m Marketplace) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		if m.state != marketplaceReady || m.dialogOpen() {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseWheelDown:
			m.moveCursor(1)
		case tea.MouseWheelUp:
			m.moveCursor(-1)
		default:
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || m.state != marketplaceReady || m.dialogOpen() {
			return m, nil
		}
		entryIdx, ok := m.entryAt(msg.Y)
		if !ok {
			return m, nil
		}
		now := time.Now()
		isDoubleClick := entryIdx == m.lastClickEntry && now.Sub(m.lastClickTime) < mpDoubleClick
		m.lastClickTime = now
		m.lastClickEntry = entryIdx
		m.moveCursor(entryIdx - m.cursor)
		if isDoubleClick {
			m.lastClickTime = time.Time{} // a third click starts a new pair
			return m.promptInstall()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.offset, _ = m.window()
		return m, nil

	case mpStatusClearMsg:
		if msg.seq == m.statusSeq {
			m.clearStatus()
		}
		return m, nil

	case NotifyMsg:
		// Inside the app, the app shows notifications itself. Standalone
		// (`matcha marketplace`) there is nobody else to show them.
		if m.standalone {
			cmd := m.setStatus(msg.Message, mpErrorTTL)
			return m, cmd
		}
		return m, nil

	case RegistryFetchedMsg:
		if msg.Err != nil {
			m.state = marketplaceError
			return m, func() tea.Msg { return NotifyMsg{Message: msg.Err.Error()} }
		}
		m.entries = msg.Entries
		m.state = marketplaceReady
		return m, nil

	case PluginInstalledMsg:
		if msg.Err != nil {
			m.clearStatus() // drop "Installing…"
			return m, func() tea.Msg {
				return NotifyMsg{Message: fmt.Sprintf("Failed to install %s: %v", msg.Name, msg.Err)}
			}
		}
		if m.installed == nil {
			m.installed = make(map[string]bool)
		}
		m.installed[msg.Name] = true
		cmd := m.setStatus(fmt.Sprintf("Installed %s", msg.Name), mpStatusTTL)
		return m, cmd

	case PluginDeletedMsg:
		if msg.Err != nil {
			m.clearStatus() // drop "Deleting…"
			return m, func() tea.Msg {
				return NotifyMsg{Message: fmt.Sprintf("Failed to delete %s: %v", msg.Name, msg.Err)}
			}
		}
		delete(m.installed, msg.Name)
		cmd := m.setStatus(fmt.Sprintf("Deleted %s", msg.Name), mpStatusTTL)
		return m, cmd

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}
	return m, nil
}

func (m Marketplace) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	kb := config.Keybinds

	// Confirmation dialog takes over input regardless of state.
	if m.confirmingDelete {
		switch msg.String() {
		case "y", "Y":
			target := m.deleteTarget
			m.confirmingDelete = false
			m.deleteTarget = ""
			m.setStatus(fmt.Sprintf("Deleting %s...", target), 0)
			return m, deletePlugin(target)
		case "n", "N", kb.Global.Cancel:
			m.confirmingDelete = false
			m.deleteTarget = ""
			return m, nil
		}
		return m, nil
	}

	if m.confirmingInstall {
		switch msg.String() {
		case "y", "Y":
			entry := m.installTarget
			m.confirmingInstall = false
			m.installTarget = plugins.PluginEntry{}
			if entry.AuthorVerified {
				m.setStatus(fmt.Sprintf("Installing %s...", entry.Name), 0)
			} else {
				m.setStatus(fmt.Sprintf("Installing %s (unverified author)...", entry.Name), 0)
			}
			return m, installPlugin(entry)
		case "n", "N", kb.Global.Cancel:
			m.confirmingInstall = false
			m.installTarget = plugins.PluginEntry{}
			return m, nil
		}
		return m, nil
	}

	if m.state != marketplaceReady {
		if msg.String() == "q" || msg.String() == kb.Global.Cancel || msg.String() == kb.Global.Quit {
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
		}
		return m, nil
	}

	switch msg.String() {
	case "q", kb.Global.Cancel:
		if m.standalone {
			return m, tea.Quit
		}
		return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
	case kb.Global.Quit:
		return m, tea.Quit
	case "up", kb.Global.NavUp:
		m.moveCursor(-1)
	case keyDown, kb.Global.NavDown:
		m.moveCursor(1)
	case keyEnter:
		return m.promptInstall()
	case kb.Inbox.Delete:
		if m.cursor < len(m.entries) {
			entry := m.entries[m.cursor]
			if m.installed[entry.Name] {
				m.confirmingDelete = true
				m.deleteTarget = entry.Name
				return m, nil
			}
		}
	}
	return m, nil
}

// headerHeight is the number of rows above the first entry (logo, title and
// the blank line below it).
func (m Marketplace) headerHeight() int {
	return blockHeight(m.headerView())
}

// footerView renders the rows pinned to the bottom: scroll position, the
// transient status and the keybind help, each fitted to the screen width.
func (m Marketplace) footerView() string {
	width := m.innerWidth()
	var lines []string
	// scroll position indicator (shown whenever there are entries)
	if m.state == marketplaceReady && len(m.entries) > 0 {
		lines = append(lines, mpFooterStyle.Render(fmt.Sprintf("  %d/%d", m.cursor+1, len(m.entries))))
	}
	// transient status line
	if m.status != "" {
		status := "  " + m.status
		if width > 0 {
			status = ansi.Truncate(status, width, "…")
		}
		lines = append(lines, mpStatusStyle.Render(status))
	}
	// keybind help (always present)
	lines = append(lines, helpStyle.Render(wrapBlock(keybindHelp("marketplace.help"), width)))
	return strings.Join(lines, "\n")
}

func (m Marketplace) footerHeight() int {
	return blockHeight(m.footerView())
}

// visibleRows is how many entries fit between the header and the footer.
func (m Marketplace) visibleRows() int {
	available := m.height - docStyle.GetVerticalMargins() - m.headerHeight() - m.footerHeight()
	// Each entry takes mpEntryHeight lines (name + description)
	return max(1, available/mpEntryHeight)
}

// window returns the range of entries on screen. The stored offset is
// clamped so the cursor stays visible even after the footer grew or the
// terminal shrank; the view and click mapping share it.
func (m Marketplace) window() (start, end int) {
	visible := m.visibleRows()
	start = m.offset
	if m.cursor >= start+visible {
		start = m.cursor - visible + 1
	}
	if m.cursor < start {
		start = m.cursor
	}
	start = max(0, min(start, len(m.entries)-visible))
	return start, min(len(m.entries), start+visible)
}

func getPluginInfo(name string) *marketplace.PluginInfo {
	info, err := marketplace.FetchPluginInfo(name)
	if err != nil {
		return nil
	}
	return info
}

func installPlugin(entry plugins.PluginEntry) tea.Cmd {
	return func() tea.Msg {
		// Try to get plugin info from new marketplace
		pluginInfo := getPluginInfo(entry.Name)

		var data []byte
		var err error

		if pluginInfo != nil {
			// Download from marketplace URL
			data, err = downloadFromURL(pluginInfo.FileURL)
		} else {
			// Fallback to old registry
			data, err = plugins.FetchPlugin(entry)
		}

		if err != nil {
			return PluginInstalledMsg{Name: entry.Name, Err: err}
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return PluginInstalledMsg{Name: entry.Name, Err: err}
		}

		dir := filepath.Join(home, ".config", "matcha", "plugins")
		if err := os.MkdirAll(dir, 0750); err != nil {
			return PluginInstalledMsg{Name: entry.Name, Err: err}
		}

		dest := filepath.Join(dir, entry.File)
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return PluginInstalledMsg{Name: entry.Name, Err: err}
		}

		return PluginInstalledMsg{Name: entry.Name}
	}
}

func downloadFromURL(url string) ([]byte, error) {
	client := httpclient.New(httpclient.RegistryFetchTimeout)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build download request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func deletePlugin(name string) tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return PluginDeletedMsg{Name: name, Err: err}
		}

		dest := filepath.Join(home, ".config", "matcha", "plugins", name+".lua")
		if err := os.Remove(dest); err != nil {
			return PluginDeletedMsg{Name: name, Err: err}
		}

		return PluginDeletedMsg{Name: name}
	}
}

func installedPlugins() map[string]bool {
	installed := make(map[string]bool)
	home, err := os.UserHomeDir()
	if err != nil {
		return installed
	}
	dir := filepath.Join(home, ".config", "matcha", "plugins")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return installed
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".lua") {
			name := strings.TrimSuffix(e.Name(), ".lua")
			installed[name] = true
		}
	}
	return installed
}

func (m Marketplace) View() tea.View {
	width := m.innerWidth()
	fit := func(line string) string {
		if width > 0 {
			return ansi.Truncate(line, width, "…")
		}
		return line
	}

	// headerView ends with the blank line above the entries.
	lines := strings.Split(m.headerView(), "\n")

	switch m.state {
	case marketplaceLoading:
		lines = append(lines, itemStyle.Render(t("common.loading")))
	case marketplaceError:
		lines = append(lines, itemStyle.Render(mpErrorStyle.Render(t("common.error"))))
	case marketplaceReady:
		start, end := m.window()
		for i := start; i < end; i++ {
			entry := m.entries[i]
			cursor := "  "
			nameStyle := mpItemNameStyle
			if i == m.cursor {
				cursor = mpCursorStyle.Render("▌ ")
				nameStyle = mpSelectedStyle
			}

			name := nameStyle.Render(entry.Title)
			if m.installed[entry.Name] {
				name += " " + mpInstalledStyle.Render(" "+t("marketplace.installed")+" ")
			}
			if entry.AuthorDisplayName != "" {
				author := mpAuthorStyle.Render(" by " + entry.AuthorDisplayName)
				if entry.AuthorVerified {
					author += " " + mpVerifiedStyle.Render("✓")
				}
				name += author
			}

			// One row each, never wrapped: click mapping counts on it.
			lines = append(lines,
				fit(cursor+name),
				fit("    "+mpItemDescStyle.Render(entry.Description)))
		}
	}

	// Footer bar: scroll position + transient status, always pinned to the bottom
	// so the keybind help line stays visible regardless of list length.
	content := strings.Join(lines, "\n")
	help := m.footerView()

	// Pin the footer to the bottom: pad the body so it fills the available
	// height, leaving the footer as the last visible rows.
	gap := 2
	if m.height > 0 {
		usable := m.height - docStyle.GetVerticalMargins()
		gap = max(1, usable-blockHeight(content)-blockHeight(help)+1)
	}

	rendered := docStyle.Render(content + strings.Repeat("\n", gap) + help)

	// Confirmation overlay: prompt the user before deleting an installed plugin.
	if m.confirmingDelete {
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				dangerStyle.Render(t("marketplace.delete_confirm")),
				accountEmailStyle.Render(m.deleteTarget),
				helpStyle.Render("(y/n)"),
			),
		)
		if m.width > 0 && m.height > 0 {
			rendered = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog, lipgloss.WithWhitespaceChars(" "))
		} else {
			rendered = "\n\n" + dialog
		}
	}

	// Confirmation overlay: prompt the user before installing a plugin.
	if m.confirmingInstall {
		entry := m.installTarget
		var lines []string
		if entry.AuthorVerified {
			lines = append(lines, mpSelectedStyle.Render(tpl("marketplace.install_confirm", map[string]interface{}{
				"title": entry.Title,
			})))
		} else {
			lines = append(lines, dangerStyle.Render(tpl("marketplace.install_confirm", map[string]interface{}{
				"title": entry.Title,
			})))
			lines = append(lines, dangerStyle.Render(t("marketplace.unverified_warning")))
		}
		if entry.AuthorDisplayName != "" {
			lines = append(lines, accountEmailStyle.Render(tpl("marketplace.install_author", map[string]interface{}{
				"author": entry.AuthorDisplayName,
			})))
		}
		lines = append(lines, helpStyle.Render("(y/n)"))
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center, lines...),
		)
		if m.width > 0 && m.height > 0 {
			rendered = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog, lipgloss.WithWhitespaceChars(" "))
		} else {
			rendered = "\n\n" + dialog
		}
	}

	v := tea.NewView(rendered)
	if config.MouseEnabled != nil && *config.MouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
