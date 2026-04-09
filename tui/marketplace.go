package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/plugins"
	"github.com/floatpane/matcha/theme"
)

var (
	mpTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	mpItemNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	mpItemDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	mpInstalledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("35"))

	mpSelectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	mpCursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	mpStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
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

// PluginInstalledMsg signals that a plugin was installed from the marketplace.
type PluginInstalledMsg struct {
	Name string
	Err  error
}

type Marketplace struct {
	entries    []plugins.PluginEntry
	installed  map[string]bool
	cursor     int
	offset     int // scroll offset
	width      int
	height     int
	state      marketplaceState
	errMsg     string
	status     string // transient status message
	standalone bool   // true when launched via `matcha marketplace` (not from main menu)
}

func NewMarketplace(standalone bool) Marketplace {
	return Marketplace{
		installed:  installedPlugins(),
		standalone: standalone,
	}
}

func (m Marketplace) Init() tea.Cmd {
	return fetchRegistry
}

func fetchRegistry() tea.Msg {
	entries, err := plugins.FetchRegistry()
	return RegistryFetchedMsg{Entries: entries, Err: err}
}

func (m Marketplace) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case RegistryFetchedMsg:
		if msg.Err != nil {
			m.state = marketplaceError
			m.errMsg = msg.Err.Error()
			return m, nil
		}
		m.entries = msg.Entries
		m.state = marketplaceReady
		return m, nil

	case PluginInstalledMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("Failed to install %s: %v", msg.Name, msg.Err)
		} else {
			m.status = fmt.Sprintf("Installed %s", msg.Name)
			m.installed[msg.Name] = true
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.state != marketplaceReady {
			if msg.String() == "q" || msg.String() == "esc" || msg.String() == "ctrl+c" {
				if m.standalone {
					return m, tea.Quit
				}
				return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "esc":
			if m.standalone {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
		case "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				visible := m.visibleRows()
				if m.cursor >= m.offset+visible {
					m.offset = m.cursor - visible + 1
				}
			}
		case "enter":
			if m.cursor < len(m.entries) {
				entry := m.entries[m.cursor]
				if m.installed[entry.Name] {
					m.status = fmt.Sprintf("%s is already installed", entry.Name)
					return m, nil
				}
				m.status = fmt.Sprintf("Installing %s...", entry.Name)
				return m, installPlugin(entry)
			}
		}
	}
	return m, nil
}

func (m Marketplace) visibleRows() int {
	// Each entry takes 2 lines (name + description), plus header/footer
	available := m.height - 8 // header + footer + padding
	if available < 1 {
		return 1
	}
	return available / 2
}

func installPlugin(entry plugins.PluginEntry) tea.Cmd {
	return func() tea.Msg {
		data, err := plugins.FetchPlugin(entry)
		if err != nil {
			return PluginInstalledMsg{Name: entry.Name, Err: err}
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return PluginInstalledMsg{Name: entry.Name, Err: err}
		}

		dir := filepath.Join(home, ".config", "matcha", "plugins")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return PluginInstalledMsg{Name: entry.Name, Err: err}
		}

		dest := filepath.Join(dir, entry.File)
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return PluginInstalledMsg{Name: entry.Name, Err: err}
		}

		return PluginInstalledMsg{Name: entry.Name}
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
	var b strings.Builder

	accentStyle := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent)
	b.WriteString(accentStyle.Render(choiceLogo))
	b.WriteString("\n")
	b.WriteString(mpTitleStyle.Render(" Plugin Marketplace "))
	b.WriteString("\n\n")

	switch m.state {
	case marketplaceLoading:
		b.WriteString("  Fetching plugins...\n")
	case marketplaceError:
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString(errStyle.Render(fmt.Sprintf("  Error: %s", m.errMsg)))
		b.WriteString("\n")
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
				name += " " + mpInstalledStyle.Render("[installed]")
			}

			b.WriteString(fmt.Sprintf("%s%s\n", cursor, name))
			b.WriteString(fmt.Sprintf("    %s\n", mpItemDescStyle.Render(entry.Description)))
		}

		if len(m.entries) > visible {
			b.WriteString(fmt.Sprintf("\n  %d/%d plugins", m.cursor+1, len(m.entries)))
		}
	}

	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(mpStatusStyle.Render("  " + m.status))
	}

	mainContent := b.String()
	help := helpStyle.Render("↑/↓ navigate • enter install • q back")

	if m.height > 0 {
		currentHeight := lipgloss.Height(DocStyle.Render(mainContent + "\n" + help))
		gap := m.height - currentHeight
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	return tea.NewView(DocStyle.Render(mainContent + "\n" + help))
}
