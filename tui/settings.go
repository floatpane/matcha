package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/theme"
)

var (
	accountItemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedAccountItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42"))
	accountEmailStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dangerStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	settingsFocusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	settingsBlurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type SettingsState int

const (
	SettingsMain SettingsState = iota
	SettingsAccounts
	SettingsMailingLists
	SettingsSMIMEConfig
	SettingsTheme
)

// Settings displays the settings screen.
type Settings struct {
	cfg              *config.Config
	state            SettingsState
	cursor           int
	confirmingDelete bool
	width            int
	height           int

	// S/MIME Config fields
	editingAccountIdx int
	focusIndex        int
	smimeCertInput    textinput.Model
	smimeKeyInput     textinput.Model

	// PGP Config fields
	pgpPublicKeyInput  textinput.Model
	pgpPrivateKeyInput textinput.Model
	pgpKeySource       string // "file" or "yubikey"
	pgpPINInput        textinput.Model
}

// NewSettings creates a new settings model.
func NewSettings(cfg *config.Config) *Settings {
	if cfg == nil {
		cfg = &config.Config{}
	}

	tiStyles := ThemedTextInputStyles()

	certInput := textinput.New()
	certInput.Placeholder = "/path/to/cert.pem"
	certInput.Prompt = "> "
	certInput.CharLimit = 256
	certInput.SetStyles(tiStyles)

	keyInput := textinput.New()
	keyInput.Placeholder = "/path/to/private_key.pem"
	keyInput.Prompt = "> "
	keyInput.CharLimit = 256
	keyInput.SetStyles(tiStyles)

	pgpPubInput := textinput.New()
	pgpPubInput.Placeholder = "/path/to/public_key.asc"
	pgpPubInput.Prompt = "> "
	pgpPubInput.CharLimit = 256
	pgpPubInput.SetStyles(tiStyles)

	pgpPrivInput := textinput.New()
	pgpPrivInput.Placeholder = "/path/to/private_key.asc"
	pgpPrivInput.Prompt = "> "
	pgpPrivInput.CharLimit = 256
	pgpPrivInput.SetStyles(tiStyles)

	pgpPINInput := textinput.New()
	pgpPINInput.Placeholder = "YubiKey PIN (6-8 digits)"
	pgpPINInput.Prompt = "> "
	pgpPINInput.CharLimit = 16
	pgpPINInput.EchoMode = textinput.EchoPassword
	pgpPINInput.EchoCharacter = '*'
	pgpPINInput.SetStyles(tiStyles)

	return &Settings{
		cfg:                cfg,
		state:              SettingsMain,
		cursor:             0,
		smimeCertInput:     certInput,
		smimeKeyInput:      keyInput,
		pgpPublicKeyInput:  pgpPubInput,
		pgpPrivateKeyInput: pgpPrivInput,
		pgpKeySource:       "file", // Default to file-based keys
		pgpPINInput:        pgpPINInput,
	}
}

// Init initializes the settings model.
func (m *Settings) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the settings model.
func (m *Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.smimeCertInput.SetWidth(m.width - 6)
		m.smimeKeyInput.SetWidth(m.width - 6)
		m.pgpPublicKeyInput.SetWidth(m.width - 6)
		m.pgpPrivateKeyInput.SetWidth(m.width - 6)
		m.pgpPINInput.SetWidth(m.width - 6)
		return m, nil

	case tea.KeyPressMsg:
		if m.state == SettingsMain {
			return m.updateMain(msg)
		} else if m.state == SettingsTheme {
			return m.updateTheme(msg)
		} else if m.state == SettingsMailingLists {
			return m.updateMailingLists(msg)
		} else if m.state == SettingsSMIMEConfig {
			var m2 *Settings
			m2, cmd = m.updateSMIMEConfig(msg)
			cmds = append(cmds, cmd)
			return m2, tea.Batch(cmds...)
		} else {
			return m.updateAccounts(msg)
		}
	}

	if m.state == SettingsSMIMEConfig {
		m.smimeCertInput, cmd = m.smimeCertInput.Update(msg)
		cmds = append(cmds, cmd)
		m.smimeKeyInput, cmd = m.smimeKeyInput.Update(msg)
		cmds = append(cmds, cmd)
		m.pgpPublicKeyInput, cmd = m.pgpPublicKeyInput.Update(msg)
		cmds = append(cmds, cmd)
		m.pgpPrivateKeyInput, cmd = m.pgpPrivateKeyInput.Update(msg)
		cmds = append(cmds, cmd)
		m.pgpPINInput, cmd = m.pgpPINInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Settings) updateMain(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		// Options: 0: Email Accounts, 1: Theme, 2: Image Display, 3: Edit Signature, 4: Contextual Tips, 5: Desktop Notifications, 6: Mailing Lists
		if m.cursor < 6 {
			m.cursor++
		}
	case "enter":
		switch m.cursor {
		case 0: // Email Accounts
			m.state = SettingsAccounts
			m.cursor = 0
			return m, nil
		case 1: // Theme
			m.state = SettingsTheme
			// Position cursor on the currently active theme
			themes := theme.AllThemes()
			m.cursor = 0
			for i, t := range themes {
				if t.Name == theme.ActiveTheme.Name {
					m.cursor = i
					break
				}
			}
			return m, nil
		case 2: // Image Display
			m.cfg.DisableImages = !m.cfg.DisableImages
			_ = config.SaveConfig(m.cfg)
			return m, nil
		case 3: // Edit Signature
			return m, func() tea.Msg { return GoToSignatureEditorMsg{} }
		case 4: // Contextual Tips
			m.cfg.HideTips = !m.cfg.HideTips
			_ = config.SaveConfig(m.cfg)
			return m, nil
		case 5: // Desktop Notifications
			m.cfg.DisableNotifications = !m.cfg.DisableNotifications
			_ = config.SaveConfig(m.cfg)
			return m, nil
		case 6: // Mailing Lists
			m.state = SettingsMailingLists
			m.cursor = 0
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
	case "e":
		// Edit selected account
		if m.cursor < len(m.cfg.Accounts) {
			acc := m.cfg.Accounts[m.cursor]
			return m, func() tea.Msg {
				return GoToEditAccountMsg{
					AccountID:    acc.ID,
					Provider:     acc.ServiceProvider,
					Name:         acc.Name,
					Email:        acc.Email,
					FetchEmail:   acc.FetchEmail,
					IMAPServer:   acc.IMAPServer,
					IMAPPort:     acc.IMAPPort,
					SMTPServer:   acc.SMTPServer,
					SMTPPort:     acc.SMTPPort,
					Protocol:     acc.Protocol,
					JMAPEndpoint: acc.JMAPEndpoint,
					POP3Server:   acc.POP3Server,
					POP3Port:     acc.POP3Port,
				}
			}
		}
	case "enter":
		// If cursor is on "Add Account"
		if m.cursor == len(m.cfg.Accounts) {
			return m, func() tea.Msg { return GoToAddAccountMsg{} }
		} else if m.cursor < len(m.cfg.Accounts) {
			m.editingAccountIdx = m.cursor
			m.state = SettingsSMIMEConfig
			m.smimeCertInput.SetValue(m.cfg.Accounts[m.cursor].SMIMECert)
			m.smimeKeyInput.SetValue(m.cfg.Accounts[m.cursor].SMIMEKey)
			m.pgpPublicKeyInput.SetValue(m.cfg.Accounts[m.cursor].PGPPublicKey)
			m.pgpPrivateKeyInput.SetValue(m.cfg.Accounts[m.cursor].PGPPrivateKey)
			// Initialize PGP key source
			if m.cfg.Accounts[m.cursor].PGPKeySource == "" {
				m.pgpKeySource = "file"
			} else {
				m.pgpKeySource = m.cfg.Accounts[m.cursor].PGPKeySource
			}
			m.pgpPINInput.SetValue(m.cfg.Accounts[m.cursor].PGPPIN)
			m.focusIndex = 0
			m.smimeCertInput.Focus()
			m.smimeKeyInput.Blur()
			m.pgpPublicKeyInput.Blur()
			m.pgpPrivateKeyInput.Blur()
			m.pgpPINInput.Blur()
			return m, textinput.Blink
		}
	case "esc":
		m.state = SettingsMain
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

// Focus indices for the crypto config screen:
// 0: S/MIME Certificate Path
// 1: S/MIME Private Key Path
// 2: S/MIME Sign By Default toggle
// 3: PGP Public Key Path
// 4: PGP Private Key Path
// 5: PGP Key Source toggle (file/yubikey)
// 6: PGP PIN input (only shown if yubikey)
// 7: PGP Sign By Default toggle
// 8: PGP Encrypt By Default toggle
// 9: Save button
// 10: Cancel button
const cryptoConfigMaxFocus = 10

func (m *Settings) updateSMIMEConfig(msg tea.KeyPressMsg) (*Settings, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg.String() {
	case "esc":
		m.state = SettingsAccounts
		return m, nil
	case "tab", "shift+tab", "up", "down":
		if msg.String() == "shift+tab" || msg.String() == "up" {
			m.focusIndex--
			if m.focusIndex < 0 {
				m.focusIndex = cryptoConfigMaxFocus
			}
		} else {
			m.focusIndex++
			if m.focusIndex > cryptoConfigMaxFocus {
				m.focusIndex = 0
			}
		}

		m.smimeCertInput.Blur()
		m.smimeKeyInput.Blur()
		m.pgpPublicKeyInput.Blur()
		m.pgpPrivateKeyInput.Blur()
		m.pgpPINInput.Blur()

		switch m.focusIndex {
		case 0:
			cmds = append(cmds, m.smimeCertInput.Focus())
		case 1:
			cmds = append(cmds, m.smimeKeyInput.Focus())
		case 3:
			cmds = append(cmds, m.pgpPublicKeyInput.Focus())
		case 4:
			cmds = append(cmds, m.pgpPrivateKeyInput.Focus())
		case 6:
			cmds = append(cmds, m.pgpPINInput.Focus())
		}
		return m, tea.Batch(cmds...)
	case "enter", " ":
		switch m.focusIndex {
		case 0: // S/MIME cert - enter advances to next field
			if msg.String() == "enter" {
				m.focusIndex = 1
				m.smimeCertInput.Blur()
				cmds = append(cmds, m.smimeKeyInput.Focus())
				return m, tea.Batch(cmds...)
			}
		case 1: // S/MIME key - enter advances
			if msg.String() == "enter" {
				m.focusIndex = 2
				m.smimeKeyInput.Blur()
				return m, nil
			}
		case 2: // S/MIME sign toggle
			if msg.String() == "enter" || msg.String() == " " {
				m.cfg.Accounts[m.editingAccountIdx].SMIMESignByDefault = !m.cfg.Accounts[m.editingAccountIdx].SMIMESignByDefault
			}
			return m, nil
		case 3: // PGP public key - enter advances
			if msg.String() == "enter" {
				m.focusIndex = 4
				m.pgpPublicKeyInput.Blur()
				cmds = append(cmds, m.pgpPrivateKeyInput.Focus())
				return m, tea.Batch(cmds...)
			}
		case 4: // PGP private key - enter advances
			if msg.String() == "enter" {
				m.focusIndex = 5
				m.pgpPrivateKeyInput.Blur()
				return m, nil
			}
		case 5: // PGP key source toggle (file/yubikey)
			if msg.String() == "enter" || msg.String() == " " {
				if m.pgpKeySource == "file" {
					m.pgpKeySource = "yubikey"
				} else {
					m.pgpKeySource = "file"
				}
			}
			return m, nil
		case 6: // PGP PIN input - enter advances
			if msg.String() == "enter" {
				m.focusIndex = 7
				m.pgpPINInput.Blur()
				return m, nil
			}
		case 7: // PGP sign toggle
			if msg.String() == "enter" || msg.String() == " " {
				m.cfg.Accounts[m.editingAccountIdx].PGPSignByDefault = !m.cfg.Accounts[m.editingAccountIdx].PGPSignByDefault
			}
			return m, nil
		case 8: // PGP encrypt toggle
			if msg.String() == "enter" || msg.String() == " " {
				m.cfg.Accounts[m.editingAccountIdx].PGPEncryptByDefault = !m.cfg.Accounts[m.editingAccountIdx].PGPEncryptByDefault
			}
			return m, nil
		case 9: // Save
			if msg.String() == "enter" {
				m.cfg.Accounts[m.editingAccountIdx].SMIMECert = m.smimeCertInput.Value()
				m.cfg.Accounts[m.editingAccountIdx].SMIMEKey = m.smimeKeyInput.Value()
				m.cfg.Accounts[m.editingAccountIdx].PGPPublicKey = m.pgpPublicKeyInput.Value()
				m.cfg.Accounts[m.editingAccountIdx].PGPPrivateKey = m.pgpPrivateKeyInput.Value()
				m.cfg.Accounts[m.editingAccountIdx].PGPKeySource = m.pgpKeySource
				m.cfg.Accounts[m.editingAccountIdx].PGPPIN = m.pgpPINInput.Value()
				_ = config.SaveConfig(m.cfg)
				m.state = SettingsAccounts
				return m, nil
			}
		case 10: // Cancel
			if msg.String() == "enter" {
				m.state = SettingsAccounts
				return m, nil
			}
		}
	}

	switch m.focusIndex {
	case 0:
		m.smimeCertInput, cmd = m.smimeCertInput.Update(msg)
		cmds = append(cmds, cmd)
	case 1:
		m.smimeKeyInput, cmd = m.smimeKeyInput.Update(msg)
		cmds = append(cmds, cmd)
	case 3:
		m.pgpPublicKeyInput, cmd = m.pgpPublicKeyInput.Update(msg)
		cmds = append(cmds, cmd)
	case 4:
		m.pgpPrivateKeyInput, cmd = m.pgpPrivateKeyInput.Update(msg)
		cmds = append(cmds, cmd)
	case 6:
		m.pgpPINInput, cmd = m.pgpPINInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Settings) updateMailingLists(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.confirmingDelete {
		switch msg.String() {
		case "y", "Y":
			if m.cursor < len(m.cfg.MailingLists) {
				m.cfg.MailingLists = append(m.cfg.MailingLists[:m.cursor], m.cfg.MailingLists[m.cursor+1:]...)
				_ = config.SaveConfig(m.cfg)
				if m.cursor >= len(m.cfg.MailingLists) && m.cursor > 0 {
					m.cursor--
				}
				m.confirmingDelete = false
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
		if m.cursor < len(m.cfg.MailingLists) {
			m.cursor++
		}
	case "d":
		if m.cursor < len(m.cfg.MailingLists) && len(m.cfg.MailingLists) > 0 {
			m.confirmingDelete = true
		}
	case "e":
		// Edit selected mailing list
		if m.cursor < len(m.cfg.MailingLists) {
			list := m.cfg.MailingLists[m.cursor]
			idx := m.cursor
			return m, func() tea.Msg {
				return GoToEditMailingListMsg{
					Index:     idx,
					Name:      list.Name,
					Addresses: strings.Join(list.Addresses, ", "),
				}
			}
		}
	case "enter":
		if m.cursor == len(m.cfg.MailingLists) {
			return m, func() tea.Msg { return GoToAddMailingListMsg{} }
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
	} else if m.state == SettingsTheme {
		return tea.NewView(m.viewTheme())
	} else if m.state == SettingsMailingLists {
		return tea.NewView(m.viewMailingLists())
	} else if m.state == SettingsSMIMEConfig {
		return tea.NewView(m.viewSMIMEConfig())
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

	// Option 1: Theme
	themeText := fmt.Sprintf("Theme: %s", theme.ActiveTheme.Name)
	if m.cursor == 1 {
		b.WriteString(selectedAccountItemStyle.Render("> " + themeText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + themeText))
	}
	b.WriteString("\n")

	// Option 2: Image Display
	status := "ON"
	if m.cfg.DisableImages {
		status = "OFF"
	}
	text := fmt.Sprintf("Image Display: %s", status)
	if m.cursor == 2 {
		b.WriteString(selectedAccountItemStyle.Render("> " + text))
	} else {
		b.WriteString(accountItemStyle.Render("  " + text))
	}
	b.WriteString("\n")

	// Option 3: Edit Signature
	sigText := "Edit Signature"
	if config.HasSignature() {
		sigText = "Edit Signature (configured)"
	}
	if m.cursor == 3 {
		b.WriteString(selectedAccountItemStyle.Render("> " + sigText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + sigText))
	}
	b.WriteString("\n")

	// Option 4: Contextual Tips
	tipsStatus := "ON"
	if m.cfg.HideTips {
		tipsStatus = "OFF"
	}
	tipsText := fmt.Sprintf("Contextual Tips: %s", tipsStatus)
	if m.cursor == 4 {
		b.WriteString(selectedAccountItemStyle.Render("> " + tipsText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + tipsText))
	}
	b.WriteString("\n")

	// Option 5: Desktop Notifications
	notifStatus := "ON"
	if m.cfg.DisableNotifications {
		notifStatus = "OFF"
	}
	notifText := fmt.Sprintf("Desktop Notifications: %s", notifStatus)
	if m.cursor == 5 {
		b.WriteString(selectedAccountItemStyle.Render("> " + notifText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + notifText))
	}
	b.WriteString("\n")

	// Option 6: Mailing Lists
	mailingListsText := "Mailing Lists"
	if m.cursor == 6 {
		b.WriteString(selectedAccountItemStyle.Render("> " + mailingListsText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + mailingListsText))
	}
	b.WriteString("\n\n")

	if !m.cfg.HideTips {
		tip := ""
		switch m.cursor {
		case 0:
			tip = "Manage your connected email accounts."
		case 1:
			tip = "Choose a color theme for the application."
		case 2:
			tip = "Toggle displaying images in emails."
		case 3:
			tip = "Configure the signature appended to your outgoing emails."
		case 4:
			tip = "Toggle displaying helpful contextual tips like this one."
		case 5:
			tip = "Toggle desktop notifications when new mail arrives."
		case 6:
			tip = "Manage groups of email addresses to quickly send to multiple people."
		}
		if tip != "" {
			b.WriteString(TipStyle.Render("Tip: "+tip) + "\n\n")
		}
	}

	mainContent := b.String()
	helpView := helpStyle.Render("↑/↓: navigate • enter: select/toggle • esc: back")

	if m.height > 0 {
		currentHeight := lipgloss.Height(docStyle.Render(mainContent + helpView))
		gap := m.height - currentHeight
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	return docStyle.Render(mainContent + helpView)
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

		if account.SMIMECert != "" && account.SMIMEKey != "" {
			providerInfo += " [S/MIME Configured]"
		}
		if account.PGPPublicKey != "" && account.PGPPrivateKey != "" {
			providerInfo += " [PGP Configured]"
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
	b.WriteString("\n")

	mainContent := b.String()
	helpView := helpStyle.Render("↑/↓: navigate • enter: select • e: edit • d: delete • esc: back")

	if m.height > 0 {
		currentHeight := lipgloss.Height(docStyle.Render(mainContent + helpView))
		gap := m.height - currentHeight
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

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

	return docStyle.Render(mainContent + helpView)
}

func (m *Settings) viewSMIMEConfig() string {
	var b strings.Builder

	account := m.cfg.Accounts[m.editingAccountIdx]
	b.WriteString(titleStyle.Render(fmt.Sprintf("Crypto Configuration for %s", account.FetchEmail)))
	b.WriteString("\n\n")

	// --- S/MIME Section ---
	b.WriteString(settingsFocusedStyle.Render("S/MIME") + "\n")

	if m.focusIndex == 0 {
		b.WriteString(settingsFocusedStyle.Render("Certificate (PEM) Path:\n"))
	} else {
		b.WriteString(settingsBlurredStyle.Render("Certificate (PEM) Path:\n"))
	}
	b.WriteString(m.smimeCertInput.View() + "\n\n")

	if m.focusIndex == 1 {
		b.WriteString(settingsFocusedStyle.Render("Private Key (PEM) Path:\n"))
	} else {
		b.WriteString(settingsBlurredStyle.Render("Private Key (PEM) Path:\n"))
	}
	b.WriteString(m.smimeKeyInput.View() + "\n\n")

	smimeSignStatus := "OFF"
	if account.SMIMESignByDefault {
		smimeSignStatus = "ON"
	}
	if m.focusIndex == 2 {
		b.WriteString(settingsFocusedStyle.Render(fmt.Sprintf("> Sign By Default: %s\n\n", smimeSignStatus)))
	} else {
		b.WriteString(settingsBlurredStyle.Render(fmt.Sprintf("  Sign By Default: %s\n\n", smimeSignStatus)))
	}

	// --- PGP Section ---
	b.WriteString(settingsFocusedStyle.Render("PGP") + "\n")

	if m.focusIndex == 3 {
		b.WriteString(settingsFocusedStyle.Render("Public Key Path:\n"))
	} else {
		b.WriteString(settingsBlurredStyle.Render("Public Key Path:\n"))
	}
	b.WriteString(m.pgpPublicKeyInput.View() + "\n\n")

	if m.focusIndex == 4 {
		b.WriteString(settingsFocusedStyle.Render("Private Key Path:\n"))
	} else {
		b.WriteString(settingsBlurredStyle.Render("Private Key Path:\n"))
	}
	b.WriteString(m.pgpPrivateKeyInput.View() + "\n\n")

	// Key source toggle
	keySourceDisplay := "File"
	if m.pgpKeySource == "yubikey" {
		keySourceDisplay = "YubiKey"
	}
	if m.focusIndex == 5 {
		b.WriteString(settingsFocusedStyle.Render(fmt.Sprintf("> Key Source: %s\n\n", keySourceDisplay)))
	} else {
		b.WriteString(settingsBlurredStyle.Render(fmt.Sprintf("  Key Source: %s\n\n", keySourceDisplay)))
	}

	// PIN input (only shown if YubiKey is selected)
	if m.pgpKeySource == "yubikey" {
		if m.focusIndex == 6 {
			b.WriteString(settingsFocusedStyle.Render("YubiKey PIN:\n"))
		} else {
			b.WriteString(settingsBlurredStyle.Render("YubiKey PIN:\n"))
		}
		b.WriteString(m.pgpPINInput.View() + "\n\n")
	}

	pgpSignStatus := "OFF"
	if account.PGPSignByDefault {
		pgpSignStatus = "ON"
	}
	if m.focusIndex == 7 {
		b.WriteString(settingsFocusedStyle.Render(fmt.Sprintf("> Sign By Default: %s\n\n", pgpSignStatus)))
	} else {
		b.WriteString(settingsBlurredStyle.Render(fmt.Sprintf("  Sign By Default: %s\n\n", pgpSignStatus)))
	}

	pgpEncryptStatus := "OFF"
	if account.PGPEncryptByDefault {
		pgpEncryptStatus = "ON"
	}
	if m.focusIndex == 8 {
		b.WriteString(settingsFocusedStyle.Render(fmt.Sprintf("> Encrypt By Default: %s\n\n", pgpEncryptStatus)))
	} else {
		b.WriteString(settingsBlurredStyle.Render(fmt.Sprintf("  Encrypt By Default: %s\n\n", pgpEncryptStatus)))
	}

	// --- Buttons ---
	saveBtn := "[ Save ]"
	cancelBtn := "[ Cancel ]"

	if m.focusIndex == 9 {
		saveBtn = settingsFocusedStyle.Render(saveBtn)
	} else {
		saveBtn = settingsBlurredStyle.Render(saveBtn)
	}

	if m.focusIndex == 10 {
		cancelBtn = settingsFocusedStyle.Render(cancelBtn)
	} else {
		cancelBtn = settingsBlurredStyle.Render(cancelBtn)
	}

	b.WriteString(saveBtn + "  " + cancelBtn + "\n\n")

	mainContent := b.String()
	helpView := helpStyle.Render("tab/shift+tab: navigate • enter: save/next • space: toggle • esc: back")

	if m.height > 0 {
		currentHeight := lipgloss.Height(docStyle.Render(mainContent + helpView))
		gap := m.height - currentHeight
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	return docStyle.Render(mainContent + helpView)
}

func (m *Settings) viewMailingLists() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Mailing Lists"))
	b.WriteString("\n\n")

	if len(m.cfg.MailingLists) == 0 {
		b.WriteString(accountEmailStyle.Render("  No mailing lists configured.\n"))
		b.WriteString("\n")
	}

	for i, list := range m.cfg.MailingLists {
		displayName := list.Name

		addrCount := fmt.Sprintf("%d address", len(list.Addresses))
		if len(list.Addresses) != 1 {
			addrCount += "es"
		}

		line := fmt.Sprintf("%s - %s", displayName, accountEmailStyle.Render(addrCount))

		if m.cursor == i {
			b.WriteString(selectedAccountItemStyle.Render(fmt.Sprintf("> %s", line)))
		} else {
			b.WriteString(accountItemStyle.Render(fmt.Sprintf("  %s", line)))
		}
		b.WriteString("\n")
	}

	// Add Mailing List option
	addListText := "Add New Mailing List"
	if m.cursor == len(m.cfg.MailingLists) {
		b.WriteString(selectedAccountItemStyle.Render(fmt.Sprintf("> %s", addListText)))
	} else {
		b.WriteString(accountItemStyle.Render(fmt.Sprintf("  %s", addListText)))
	}
	b.WriteString("\n")

	helpView := helpStyle.Render("↑/↓: navigate • enter: select • e: edit • d: delete • esc: back")
	mainContent := b.String()

	if m.height > 0 {
		contentHeight := strings.Count(mainContent, "\n") + 1
		gap := m.height - contentHeight - 2 // -2 for margins
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	if m.confirmingDelete {
		listName := m.cfg.MailingLists[m.cursor].Name
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				dangerStyle.Render("Delete mailing list?"),
				accountEmailStyle.Render(listName),
				HelpStyle.Render("\n(y/n)"),
			),
		)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	return docStyle.Render(mainContent + helpView)
}

func (m *Settings) updateTheme(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	themes := theme.AllThemes()

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(themes)-1 {
			m.cursor++
		}
	case "enter":
		if m.cursor < len(themes) {
			selected := themes[m.cursor]
			theme.SetTheme(selected.Name)
			RebuildStyles()
			m.cfg.Theme = selected.Name
			_ = config.SaveConfig(m.cfg)
		}
		m.state = SettingsMain
		m.cursor = 1 // Return to Theme option
		return m, nil
	case "esc":
		m.state = SettingsMain
		m.cursor = 1 // Return to Theme option
		return m, nil
	}
	return m, nil
}

func (m *Settings) viewTheme() string {
	themes := theme.AllThemes()

	// Build left panel: theme list
	var left strings.Builder
	left.WriteString(titleStyle.Render("Theme") + "\n\n")

	for i, t := range themes {
		isActive := t.Name == theme.ActiveTheme.Name

		label := t.Name
		if isActive {
			label += " (active)"
		}

		if m.cursor == i {
			left.WriteString(selectedAccountItemStyle.Render(fmt.Sprintf("> %s", label)))
		} else {
			left.WriteString(accountItemStyle.Render(fmt.Sprintf("  %s", label)))
		}
		left.WriteString("\n")
	}

	left.WriteString("\n")
	if !m.cfg.HideTips {
		left.WriteString(TipStyle.Render("Tip: Custom themes can be added as\nJSON files in ~/.config/matcha/themes/") + "\n")
	}

	// Build right panel: theme preview
	var previewTheme theme.Theme
	if m.cursor < len(themes) {
		previewTheme = themes[m.cursor]
	} else {
		previewTheme = theme.ActiveTheme
	}
	preview := renderThemePreview(previewTheme, m.width)

	// Join panels side by side
	leftPanel := lipgloss.NewStyle().Width(30).Render(left.String())
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", preview)

	helpView := helpStyle.Render("↑/↓: navigate • enter: select • esc: back")

	if m.height > 0 {
		currentHeight := lipgloss.Height(docStyle.Render(content + "\n" + helpView))
		gap := m.height - currentHeight
		if gap > 0 {
			content += strings.Repeat("\n", gap)
		}
	} else {
		content += "\n\n"
	}

	return docStyle.Render(content + "\n" + helpView)
}

// renderThemePreview renders a small mockup showing how a theme looks.
func renderThemePreview(t theme.Theme, maxWidth int) string {
	previewWidth := maxWidth - 38 // 30 for left panel + padding/margins
	if previewWidth < 30 {
		previewWidth = 30
	}
	if previewWidth > 60 {
		previewWidth = 60
	}

	accent := lipgloss.NewStyle().Foreground(t.Accent)
	accentBold := lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	secondary := lipgloss.NewStyle().Foreground(t.Secondary)
	muted := lipgloss.NewStyle().Foreground(t.MutedText)
	dim := lipgloss.NewStyle().Foreground(t.DimText)
	danger := lipgloss.NewStyle().Foreground(t.Danger)
	warn := lipgloss.NewStyle().Foreground(t.Warning)
	tip := lipgloss.NewStyle().Foreground(t.Tip).Italic(true)
	link := lipgloss.NewStyle().Foreground(t.Link)
	title := lipgloss.NewStyle().Foreground(t.AccentText).Background(t.AccentDark).Padding(0, 1)
	activeTab := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Underline(true)
	activeFolder := lipgloss.NewStyle().Background(t.Accent).Foreground(t.Contrast).Bold(true).Padding(0, 1)

	var b strings.Builder

	b.WriteString(title.Render("Preview: "+t.Name) + "\n\n")

	// Fake inbox tabs
	b.WriteString(activeTab.Render("Inbox") + "  " + secondary.Render("Sent") + "  " + secondary.Render("Drafts") + "\n")
	b.WriteString(secondary.Render(strings.Repeat("─", previewWidth)) + "\n")

	// Fake email list
	b.WriteString(accentBold.Render("> ") + dim.Render("Alice  ") + accent.Render("Meeting tomorrow") + "  " + muted.Render("2m ago") + "\n")
	b.WriteString("  " + dim.Render("Bob    ") + secondary.Render("Re: Project update") + "  " + muted.Render("1h ago") + "\n")
	b.WriteString("  " + dim.Render("Carol  ") + secondary.Render("Quick question") + "    " + muted.Render("3h ago") + "\n\n")

	// Folder sidebar sample
	b.WriteString(accentBold.Render("Folders") + "\n")
	b.WriteString(activeFolder.Render(" INBOX ") + "  " + secondary.Render("Sent") + "  " + secondary.Render("Trash") + "\n\n")

	// Status indicators
	b.WriteString(accentBold.Render("Success: ") + accent.Render("Email sent!") + "\n")
	b.WriteString(danger.Render("Error: ") + danger.Render("Connection failed") + "\n")
	b.WriteString(warn.Render("Update available: v2.0") + "\n")
	b.WriteString(tip.Render("Tip: Press ? for help") + "\n")
	b.WriteString(link.Render("https://example.com") + "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.AccentDark).
		Padding(1, 2).
		Width(previewWidth).
		Render(b.String())

	return box
}

// UpdateConfig updates the configuration (used when accounts are deleted).
func (m *Settings) UpdateConfig(cfg *config.Config) {
	m.cfg = cfg
	if m.state == SettingsAccounts && m.cursor >= len(cfg.Accounts) {
		m.cursor = len(cfg.Accounts)
	}
}
