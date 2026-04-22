package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/theme"
)

var (
	accountItemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedAccountItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42")).Bold(true)
	accountEmailStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dangerStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	settingsFocusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	settingsBlurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type SettingsPane int

const (
	PaneMenu SettingsPane = iota
	PaneContent
)

type SettingsCategory int

const (
	CategoryGeneral SettingsCategory = iota
	CategoryAccounts
	CategoryTheme
	CategoryMailingLists
	CategoryEncryption
)

type Settings struct {
	cfg    *config.Config
	width  int
	height int

	activePane     SettingsPane
	activeCategory SettingsCategory

	// Menu state
	menuCursor int

	// Sub-components states
	generalCursor    int
	accountsCursor   int
	themeCursor      int
	listsCursor      int
	confirmingDelete bool

	// S/MIME Config fields
	isCryptoConfig     bool
	editingAccountIdx  int
	cryptoFocusIndex   int
	smimeCertInput     textinput.Model
	smimeKeyInput      textinput.Model
	pgpPublicKeyInput  textinput.Model
	pgpPrivateKeyInput textinput.Model
	pgpKeySource       string // "file" or "yubikey"
	pgpPINInput        textinput.Model

	// Encryption fields
	encPasswordInput  textinput.Model
	encConfirmInput   textinput.Model
	encFocusIndex     int
	encError          string
	encEnabling       bool
	confirmingDisable bool
}

func NewSettings(cfg *config.Config) *Settings {
	if cfg == nil {
		cfg = &config.Config{}
	}

	tiStyles := ThemedTextInputStyles()

	newInput := func(placeholder, prompt string, isPassword bool) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.Prompt = prompt
		t.CharLimit = 256
		t.SetStyles(tiStyles)
		if isPassword {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '*'
		}
		return t
	}

	return &Settings{
		cfg:                cfg,
		activePane:         PaneMenu,
		activeCategory:     CategoryGeneral,
		smimeCertInput:     newInput("/path/to/cert.pem", "> ", false),
		smimeKeyInput:      newInput("/path/to/private_key.pem", "> ", false),
		pgpPublicKeyInput:  newInput("/path/to/public_key.asc", "> ", false),
		pgpPrivateKeyInput: newInput("/path/to/private_key.asc", "> ", false),
		pgpPINInput:        newInput("YubiKey PIN (6-8 digits)", "> ", true),
		pgpKeySource:       "file",
		encPasswordInput:   newInput("Password", "> ", true),
		encConfirmInput:    newInput("Confirm Password", "> ", true),
	}
}

func (m *Settings) Init() tea.Cmd {
	return textinput.Blink
}

func (m *Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputWidth := (m.width - 30) - 6 // left pane is 30
		if inputWidth < 20 {
			inputWidth = 20
		}
		m.smimeCertInput.SetWidth(inputWidth)
		m.smimeKeyInput.SetWidth(inputWidth)
		m.pgpPublicKeyInput.SetWidth(inputWidth)
		m.pgpPrivateKeyInput.SetWidth(inputWidth)
		m.pgpPINInput.SetWidth(inputWidth)
		return m, nil

	case tea.KeyPressMsg:
		// Global shortcut to return to menu from content pane
		if m.activePane == PaneContent && msg.String() == "esc" {
			// unless we are in crypto config or encryption editing which have their own esc logic
			if !(m.activeCategory == CategoryAccounts && m.isCryptoConfig) &&
				!(m.activeCategory == CategoryEncryption && m.encFocusIndex > -1) {
				m.activePane = PaneMenu
				return m, nil
			}
		}

		if m.activePane == PaneMenu {
			return m.updateMenu(msg)
		} else {
			switch m.activeCategory {
			case CategoryGeneral:
				return m.updateGeneral(msg)
			case CategoryAccounts:
				return m.updateAccounts(msg)
			case CategoryTheme:
				return m.updateTheme(msg)
			case CategoryMailingLists:
				return m.updateMailingLists(msg)
			case CategoryEncryption:
				return m.updateEncryption(msg)
			}
		}

	case SecureModeEnabledMsg:
		m.encEnabling = false
		if msg.Err != nil {
			m.encError = msg.Err.Error()
			return m, nil
		}
		m.activePane = PaneMenu
		return m, nil

	case SecureModeDisabledMsg:
		if msg.Err != nil {
			m.encError = msg.Err.Error()
			return m, nil
		}
		m.confirmingDisable = false
		m.activePane = PaneMenu
		return m, nil
	}

	// Update text inputs if active
	if m.activePane == PaneContent {
		if m.activeCategory == CategoryEncryption {
			m.encPasswordInput, cmd = m.encPasswordInput.Update(msg)
			cmds = append(cmds, cmd)
			m.encConfirmInput, cmd = m.encConfirmInput.Update(msg)
			cmds = append(cmds, cmd)
		} else if m.activeCategory == CategoryAccounts && m.isCryptoConfig {
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
	}

	return m, tea.Batch(cmds...)
}

func (m *Settings) updateMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < 4 {
			m.menuCursor++
		}
	case "right", "l", "enter":
		m.activeCategory = SettingsCategory(m.menuCursor)
		m.activePane = PaneContent

		// Reset states
		m.confirmingDelete = false
		if m.activeCategory == CategoryTheme {
			// Find current theme index
			themes := theme.AllThemes()
			for i, t := range themes {
				if t.Name == theme.ActiveTheme.Name {
					m.themeCursor = i
					break
				}
			}
		} else if m.activeCategory == CategoryEncryption {
			m.encError = ""
			m.encPasswordInput.SetValue("")
			m.encConfirmInput.SetValue("")
			m.encFocusIndex = 0
			m.confirmingDisable = false
			m.encEnabling = false
			if !config.IsSecureModeEnabled() {
				m.encPasswordInput.Focus()
				m.encConfirmInput.Blur()
			}
		}

		return m, textinput.Blink
	case "esc":
		return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
	}
	m.activeCategory = SettingsCategory(m.menuCursor)
	return m, nil
}

func (m *Settings) View() tea.View {
	// Left pane
	var left strings.Builder
	left.WriteString(titleStyle.Render(t("settings.title")) + "\n\n")

	categories := []string{
		t("settings.category_general"),
		t("settings.category_accounts"),
		t("settings.category_theme"),
		t("settings.category_mailing_lists"),
		t("settings.category_encryption"),
	}
	for i, c := range categories {
		cursor := "  "
		if m.menuCursor == i {
			if m.activePane == PaneMenu {
				cursor = "> "
			} else {
				cursor = "• "
			}
		}

		style := accountItemStyle
		if m.menuCursor == i {
			style = selectedAccountItemStyle
		}

		left.WriteString(style.Render(cursor+c) + "\n")
	}

	leftPanel := lipgloss.NewStyle().
		Width(30).
		PaddingRight(2).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(theme.ActiveTheme.Secondary).
		Render(left.String())

	// Right pane
	var right string
	switch m.activeCategory {
	case CategoryGeneral:
		right = m.viewGeneral()
	case CategoryAccounts:
		right = m.viewAccounts()
	case CategoryTheme:
		right = m.viewTheme()
	case CategoryMailingLists:
		right = m.viewMailingLists()
	case CategoryEncryption:
		right = m.viewEncryption()
	}

	rightPanel := lipgloss.NewStyle().
		PaddingLeft(2).
		Width(m.width - 34). // 30 (left) + 2 (border) + 2 (padding)
		Render(right)

	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	helpText := t("settings.help_content")
	if m.activePane == PaneMenu {
		helpText = t("settings.help_menu")
	}
	helpView := helpStyle.Render(helpText)

	if m.height > 0 {
		currentHeight := lipgloss.Height(content + "\n\n" + helpView)
		gap := m.height - currentHeight
		if gap > 0 {
			content += strings.Repeat("\n", gap)
		}
	} else {
		content += "\n\n"
	}

	return tea.NewView(docStyle.Render(content + helpView))
}

func (m *Settings) UpdateConfig(cfg *config.Config) {
	m.cfg = cfg
	if m.activeCategory == CategoryAccounts && m.accountsCursor >= len(cfg.Accounts) {
		m.accountsCursor = len(cfg.Accounts)
	}
}
