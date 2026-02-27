package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
)

var (
	accountItemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedAccountItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42"))
	accountEmailStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dangerStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type SettingsState int

const (
	SettingsMain SettingsState = iota
	SettingsAccounts
)

// Settings displays the settings screen.
type Settings struct {
	cfg              *config.Config
	state            SettingsState
	cursor           int
	confirmingDelete bool
	width            int
	height           int
}

// NewSettings creates a new settings model.
func NewSettings(cfg *config.Config) *Settings {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &Settings{
		cfg:    cfg,
		state:  SettingsMain,
		cursor: 0,
	}
}

// Init initializes the settings model.
func (m *Settings) Init() tea.Cmd {
	return nil
}

// Update handles messages for the settings model.
func (m *Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		if m.state == SettingsMain {
			return m.updateMain(msg)
		} else {
			return m.updateAccounts(msg)
		}
	}
	return m, nil
}

func (m *Settings) updateMain(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		// Options: 0: Email Accounts, 1: Image Display, 2: Edit Signature, 3: Contextual Tips
		if m.cursor < 3 {
			m.cursor++
		}
	case "enter":
		switch m.cursor {
		case 0: // Email Accounts
			m.state = SettingsAccounts
			m.cursor = 0
			return m, nil
		case 1: // Image Display
			m.cfg.DisableImages = !m.cfg.DisableImages
			// Save config immediately
			_ = config.SaveConfig(m.cfg)
			return m, nil
		case 2: // Edit Signature
			return m, func() tea.Msg { return GoToSignatureEditorMsg{} }
		case 3: // Contextual Tips
			m.cfg.HideTips = !m.cfg.HideTips
			// Save config immediately
			_ = config.SaveConfig(m.cfg)
			return m, nil
		}
	case "esc":
		return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
	}
	return m, nil
}

func (m *Settings) updateAccounts(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.confirmingDelete {
		switch msg.String() {
		case "y", "Y":
			if m.cursor < len(m.cfg.Accounts) {
				accountID := m.cfg.Accounts[m.cursor].ID
				m.confirmingDelete = false
				return m, func() tea.Msg {
					return DeleteAccountMsg{AccountID: accountID}
				}
			}
		case "n", "N", "esc":
			m.confirmingDelete = false
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		// +1 for "Add Account" option
		if m.cursor < len(m.cfg.Accounts) {
			m.cursor++
		}
	case "d":
		// Delete selected account (not the "Add Account" option)
		if m.cursor < len(m.cfg.Accounts) && len(m.cfg.Accounts) > 0 {
			m.confirmingDelete = true
		}
	case "enter":
		// If cursor is on "Add Account"
		if m.cursor == len(m.cfg.Accounts) {
			return m, func() tea.Msg { return GoToAddAccountMsg{} }
		}
	case "esc":
		m.state = SettingsMain
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

// View renders the settings screen.
func (m *Settings) View() tea.View {
	if m.state == SettingsMain {
		return tea.NewView(m.viewMain())
	}
	return tea.NewView(m.viewAccounts())
}

func (m *Settings) viewMain() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Settings") + "\n\n")

	// Option 0: Email Accounts
	if m.cursor == 0 {
		b.WriteString(selectedAccountItemStyle.Render("> Email Accounts"))
	} else {
		b.WriteString(accountItemStyle.Render("  Email Accounts"))
	}
	b.WriteString("\n")

	// Option 1: Image Display
	status := "ON"
	if m.cfg.DisableImages {
		status = "OFF"
	}
	text := fmt.Sprintf("Image Display: %s", status)
	if m.cursor == 1 {
		b.WriteString(selectedAccountItemStyle.Render("> " + text))
	} else {
		b.WriteString(accountItemStyle.Render("  " + text))
	}
	b.WriteString("\n")

	// Option 2: Edit Signature
	sigText := "Edit Signature"
	if config.HasSignature() {
		sigText = "Edit Signature (configured)"
	}
	if m.cursor == 2 {
		b.WriteString(selectedAccountItemStyle.Render("> " + sigText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + sigText))
	}
	b.WriteString("\n")

	// Option 3: Contextual Tips
	tipsStatus := "ON"
	if m.cfg.HideTips {
		tipsStatus = "OFF"
	}
	tipsText := fmt.Sprintf("Contextual Tips: %s", tipsStatus)
	if m.cursor == 3 {
		b.WriteString(selectedAccountItemStyle.Render("> " + tipsText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + tipsText))
	}
	b.WriteString("\n\n")

	if !m.cfg.HideTips {
		tip := ""
		switch m.cursor {
		case 0:
			tip = "Manage your connected email accounts."
		case 1:
			tip = "Toggle displaying images in emails."
		case 2:
			tip = "Configure the signature appended to your outgoing emails."
		case 3:
			tip = "Toggle displaying helpful contextual tips like this one."
		}
		if tip != "" {
			b.WriteString(TipStyle.Render("Tip: "+tip) + "\n\n")
		}
	}

	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: select/toggle • esc: back"))

	return docStyle.Render(b.String())
}

func (m *Settings) viewAccounts() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Account Settings"))
	b.WriteString("\n\n")

	if len(m.cfg.Accounts) == 0 {
		b.WriteString(accountEmailStyle.Render("  No accounts configured.\n"))
		b.WriteString("\n")
	}

	for i, account := range m.cfg.Accounts {
		displayName := account.Email
		if account.Name != "" {
			displayName = fmt.Sprintf("%s (%s)", account.Name, account.FetchEmail)
		}

		providerInfo := account.ServiceProvider
		if account.ServiceProvider == "custom" {
			providerInfo = fmt.Sprintf("custom: %s", account.IMAPServer)
		}

		line := fmt.Sprintf("%s - %s", displayName, accountEmailStyle.Render(providerInfo))

		if m.cursor == i {
			b.WriteString(selectedAccountItemStyle.Render(fmt.Sprintf("> %s", line)))
		} else {
			b.WriteString(accountItemStyle.Render(fmt.Sprintf("  %s", line)))
		}
		b.WriteString("\n")
	}

	// Add Account option
	addAccountText := "Add New Account"
	if m.cursor == len(m.cfg.Accounts) {
		b.WriteString(selectedAccountItemStyle.Render(fmt.Sprintf("> %s", addAccountText)))
	} else {
		b.WriteString(accountItemStyle.Render(fmt.Sprintf("  %s", addAccountText)))
	}
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: select • d: delete account • esc: back"))

	if m.confirmingDelete {
		accountName := m.cfg.Accounts[m.cursor].Email
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				dangerStyle.Render("Delete account?"),
				accountEmailStyle.Render(accountName),
				HelpStyle.Render("\n(y/n)"),
			),
		)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	return docStyle.Render(b.String())
}

// UpdateConfig updates the configuration (used when accounts are deleted).
func (m *Settings) UpdateConfig(cfg *config.Config) {
	m.cfg = cfg
	if m.state == SettingsAccounts && m.cursor >= len(cfg.Accounts) {
		m.cursor = len(cfg.Accounts)
	}
}
