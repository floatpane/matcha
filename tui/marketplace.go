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

type Marketplace struct {
	entries           []plugins.PluginEntry
	installed         map[string]bool
	cursor            int
	offset            int // scroll offset
	width             int
	height            int
	state             marketplaceState
	status            string // transient status message
	standalone        bool   // true when launched via `matcha marketplace` (not from main menu)
	lastClickTime     time.Time
	lastClickY        int
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

func (m Marketplace) itemsStartY() int {
	// docStyle top margin (1) + choiceLogo blank+5 lines (6) + explicit \n (1) = 8
	// mpTitleStyle title (1) + \n\n (2) = 3
	return 8 + 3
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
		if m.state != marketplaceReady {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseWheelDown:
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				visible := m.visibleRows()
				if m.cursor >= m.offset+visible {
					m.offset = m.cursor - visible + 1
				}
			}
		case tea.MouseWheelUp:
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || m.state != marketplaceReady {
			return m, nil
		}
		rowInList := msg.Y - m.itemsStartY()
		if rowInList >= 0 {
			entryIdx := m.offset + rowInList/2
			if entryIdx >= 0 && entryIdx < len(m.entries) {
				now := time.Now()
				isDoubleClick := msg.Y == m.lastClickY && now.Sub(m.lastClickTime) < 500*time.Millisecond
				m.lastClickTime = now
				m.lastClickY = msg.Y
				m.cursor = entryIdx
				if isDoubleClick {
					entry := m.entries[m.cursor]
					if m.installed[entry.Name] {
						m.status = fmt.Sprintf("%s is already installed", entry.Name)
						return m, nil
					}
					m.confirmingInstall = true
					m.installTarget = entry
					return m, nil
				}
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
			return m, func() tea.Msg {
				return NotifyMsg{Message: fmt.Sprintf("Failed to install %s: %v", msg.Name, msg.Err)}
			}
		}
		m.status = fmt.Sprintf("Installed %s", msg.Name)
		m.installed[msg.Name] = true
		return m, nil

	case PluginDeletedMsg:
		if msg.Err != nil {
			return m, func() tea.Msg {
				return NotifyMsg{Message: fmt.Sprintf("Failed to delete %s: %v", msg.Name, msg.Err)}
			}
		}
		m.status = fmt.Sprintf("Deleted %s", msg.Name)
		delete(m.installed, msg.Name)
		return m, nil

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
			m.status = fmt.Sprintf("Deleting %s...", target)
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
				m.status = fmt.Sprintf("Installing %s...", entry.Name)
			} else {
				m.status = fmt.Sprintf("Installing %s (unverified author)...", entry.Name)
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
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
		}
	case keyDown, kb.Global.NavDown:
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			visible := m.visibleRows()
			if m.cursor >= m.offset+visible {
				m.offset = m.cursor - visible + 1
			}
		}
	case keyEnter:
		if m.cursor < len(m.entries) {
			entry := m.entries[m.cursor]
			if m.installed[entry.Name] {
				m.status = fmt.Sprintf("%s is already installed", entry.Name)
				return m, nil
			}
			m.confirmingInstall = true
			m.installTarget = entry
			return m, nil
		}
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

func (m Marketplace) headerHeight() int {
	// renderLogo(choiceLogo) + \n + mpTitleStyle(1) + \n\n
	return lipgloss.Height(renderLogo(choiceLogo)) + 1 + 1 + 1
}

func (m Marketplace) footerHeight() int {
	h := 0
	// scroll position indicator (shown whenever there are entries)
	if m.state == marketplaceReady && len(m.entries) > 0 {
		h++
	}
	// transient status line
	if m.status != "" {
		h++
	}
	// keybind help line (always present)
	h++
	return h
}

func (m Marketplace) visibleRows() int {
	// docStyle margin (1 top + 1 bottom) + header + footer
	reserved := 2 + m.headerHeight() + m.footerHeight()
	available := m.height - reserved
	if available < 1 {
		return 1
	}
	// Each entry takes 2 lines (name + description)
	return available / 2
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
	var body strings.Builder

	body.WriteString(renderLogo(choiceLogo))
	body.WriteString("\n")
	body.WriteString(mpTitleStyle.Render(" " + t("marketplace.title") + " "))
	body.WriteString("\n\n")

	switch m.state {
	case marketplaceLoading:
		body.WriteString(itemStyle.Render(t("common.loading")))
		body.WriteString("\n")
	case marketplaceError:
		body.WriteString(itemStyle.Render(mpErrorStyle.Render(t("common.error"))))
		body.WriteString("\n")
	case marketplaceReady:
		visible := m.visibleRows()
		end := m.offset + visible
		if end > len(m.entries) {
			end = len(m.entries)
		}

		for i := m.offset; i < end; i++ {
			entry := m.entries[i]
			cursor := "  "
			nameStyle := mpItemNameStyle
			if i == m.cursor {
				cursor = mpCursorStyle.Render("> ")
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

			fmt.Fprintf(&body, "%s%s\n", cursor, name)
			fmt.Fprintf(&body, "    %s\n", mpItemDescStyle.Render(entry.Description))
		}
	}

	// Footer bar: scroll position + transient status, always pinned to the bottom
	// so the keybind help line stays visible regardless of list length.
	var footer strings.Builder
	if m.state == marketplaceReady && len(m.entries) > 0 {
		fmt.Fprintf(&footer, "%s\n", mpFooterStyle.Render(fmt.Sprintf("  %d/%d", m.cursor+1, len(m.entries))))
	}
	if m.status != "" {
		footer.WriteString(mpStatusStyle.Render("  " + m.status))
		footer.WriteString("\n")
	}
	footer.WriteString(helpStyle.Render(t("marketplace.help")))

	content := body.String()
	help := footer.String()

	// Pin the footer to the bottom: pad the body so it fills the available
	// height, leaving the footer as the last visible rows.
	if m.height > 0 {
		usable := m.height - 2 // docStyle top + bottom margin
		bodyHeight := lipgloss.Height(content)
		footerHeight := lipgloss.Height(help)
		gap := usable - bodyHeight - footerHeight
		if gap > 0 {
			content += strings.Repeat("\n", gap)
		}
	} else {
		content += "\n\n"
	}

	rendered := docStyle.Render(content + help)

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
