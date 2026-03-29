package tui

import (
	"strconv"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Login holds the state for the login/add account form.
type Login struct {
	focusIndex int
	inputs     []textinput.Model
	showCustom bool // Show custom server fields
	useOAuth2  bool // Use OAuth2 instead of password (for gmail)
	isEditMode bool // Whether we're editing an existing account
	accountID  string
	hideTips   bool
	width      int
	height     int
}

const (
	inputProvider = iota
	inputName
	inputEmail
	inputFetchEmail
	inputAuthMethod // "password" or "oauth2" (shown for gmail)
	inputPassword
	inputIMAPServer
	inputIMAPPort
	inputSMTPServer
	inputSMTPPort
	inputCount
)

// NewLogin creates a new login model for adding accounts.
func NewLogin(hideTips bool) *Login {
	m := &Login{
		inputs:   make([]textinput.Model, inputCount),
		hideTips: hideTips,
	}

	tiStyles := ThemedTextInputStyles()
	var t textinput.Model
	for i := range m.inputs {
		t = textinput.New()
		t.CharLimit = 128
		t.SetStyles(tiStyles)

		switch i {
		case inputProvider:
			t.Placeholder = "Provider (gmail, icloud, or custom)"
			t.Focus()
			t.Prompt = "🏢 > "
		case inputName:
			t.Placeholder = "Display Name"
			t.Prompt = "👤 > "
		case inputEmail:
			t.Placeholder = "Username"
			t.Prompt = "🏠 > "
		case inputFetchEmail:
			t.Placeholder = "Email Address (comma-separated for multiple)"
			t.Prompt = "📧 > "
		case inputAuthMethod:
			t.Placeholder = "Auth Method (password or oauth2)"
			t.Prompt = "🔐 > "
		case inputPassword:
			t.Placeholder = "Password / App Password"
			t.EchoMode = textinput.EchoPassword
			t.Prompt = "🔑 > "
		case inputIMAPServer:
			t.Placeholder = "IMAP Server (e.g., imap.example.com)"
			t.Prompt = "📥 > "
		case inputIMAPPort:
			t.Placeholder = "IMAP Port (default: 993)"
			t.Prompt = "🔢 > "
		case inputSMTPServer:
			t.Placeholder = "SMTP Server (e.g., smtp.example.com)"
			t.Prompt = "📤 > "
		case inputSMTPPort:
			t.Placeholder = "SMTP Port (default: 587)"
			t.Prompt = "🔢 > "
		}
		m.inputs[i] = t
	}

	return m
}

// Init initializes the login model.
func (m *Login) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the login model.
func (m *Login) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		for i := range m.inputs {
			m.inputs[i].SetWidth(msg.Width - 6)
		}

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return GoToChoiceMenuMsg{} }

		case "enter":
			// Check if provider is "custom" to show/hide custom fields
			provider := m.inputs[inputProvider].Value()
			m.showCustom = provider == "custom"
			m.useOAuth2 = m.inputs[inputAuthMethod].Value() == "oauth2"

			lastFieldIndex := inputPassword
			if m.useOAuth2 {
				// OAuth2: last field before submit is the auth method field
				lastFieldIndex = inputAuthMethod
			}
			if m.showCustom {
				lastFieldIndex = inputSMTPPort
			}

			if m.focusIndex == lastFieldIndex {
				// Submit the form
				imapPort := 993
				smtpPort := 587
				if m.inputs[inputIMAPPort].Value() != "" {
					if p, err := strconv.Atoi(m.inputs[inputIMAPPort].Value()); err == nil {
						imapPort = p
					}
				}
				if m.inputs[inputSMTPPort].Value() != "" {
					if p, err := strconv.Atoi(m.inputs[inputSMTPPort].Value()); err == nil {
						smtpPort = p
					}
				}

				authMethod := "password"
				if m.useOAuth2 {
					authMethod = "oauth2"
				}

				return m, func() tea.Msg {
					return Credentials{
						Provider:   m.inputs[inputProvider].Value(),
						Name:       m.inputs[inputName].Value(),
						Host:       m.inputs[inputEmail].Value(),
						FetchEmail: m.inputs[inputFetchEmail].Value(),
						Password:   m.inputs[inputPassword].Value(),
						IMAPServer: m.inputs[inputIMAPServer].Value(),
						IMAPPort:   imapPort,
						SMTPServer: m.inputs[inputSMTPServer].Value(),
						SMTPPort:   smtpPort,
						AuthMethod: authMethod,
					}
				}
			}
			fallthrough

		case "tab", "shift+tab", "up", "down":
			s := msg.String()

			// Check provider to update showCustom and useOAuth2
			provider := m.inputs[inputProvider].Value()
			m.showCustom = provider == "custom"
			m.useOAuth2 = m.inputs[inputAuthMethod].Value() == "oauth2"

			maxIndex := inputPassword
			if m.useOAuth2 {
				maxIndex = inputAuthMethod
			}
			if m.showCustom {
				maxIndex = inputSMTPPort
			}

			if s == "up" || s == "shift+tab" {
				m.focusIndex--
			} else {
				m.focusIndex++
			}

			if m.focusIndex > maxIndex {
				m.focusIndex = 0
			} else if m.focusIndex < 0 {
				m.focusIndex = maxIndex
			}

			// Skip password field when using OAuth2
			if m.useOAuth2 && m.focusIndex == inputPassword {
				if s == "up" || s == "shift+tab" {
					m.focusIndex = inputAuthMethod
				} else {
					m.focusIndex = 0
				}
			}

			// Skip auth method field when not Gmail (only Gmail supports OAuth2)
			if provider != "gmail" && m.focusIndex == inputAuthMethod {
				if s == "up" || s == "shift+tab" {
					m.focusIndex = inputFetchEmail
				} else {
					m.focusIndex = inputPassword
				}
			}

			// Skip custom fields if not showing them
			if !m.showCustom && m.focusIndex > inputPassword {
				if s == "up" || s == "shift+tab" {
					if m.useOAuth2 {
						m.focusIndex = inputAuthMethod
					} else {
						m.focusIndex = inputPassword
					}
				} else {
					m.focusIndex = 0
				}
			}

			cmds := make([]tea.Cmd, len(m.inputs))
			for i := 0; i < len(m.inputs); i++ {
				if i == m.focusIndex {
					cmds[i] = m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			return m, tea.Batch(cmds...)
		}
	}

	// Update the focused input field
	var cmds = make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}

	// Check if provider changed
	provider := m.inputs[inputProvider].Value()
	m.showCustom = provider == "custom"
	m.useOAuth2 = m.inputs[inputAuthMethod].Value() == "oauth2"

	return m, tea.Batch(cmds...)
}

// View renders the login form.
func (m *Login) View() tea.View {
	title := "Add Account"
	if m.isEditMode {
		title = "Edit Account"
	}

	customHint := ""
	if m.inputs[inputProvider].Value() == "custom" || m.showCustom {
		customHint = "\n" + accountEmailStyle.Render("Custom provider selected - configure server settings below")
	}

	tip := ""
	switch m.focusIndex {
	case inputProvider:
		tip = "Enter your email provider (e.g., gmail, icloud) or 'custom'."
	case inputName:
		tip = "The name that will appear on emails you send."
	case inputEmail:
		tip = "Your full email address used to log in."
	case inputFetchEmail:
		tip = "The email address to fetch messages for (comma-separated for multiple, e.g. me@icloud.com,me@mac.com)."
	case inputAuthMethod:
		tip = "Type 'oauth2' for Gmail OAuth2 or 'password' for app password."
	case inputPassword:
		tip = "Your password or an app-specific password if using 2FA."
	case inputIMAPServer:
		tip = "The server address for receiving emails."
	case inputIMAPPort:
		tip = "The port for the IMAP server (usually 993 for SSL)."
	case inputSMTPServer:
		tip = "The server address for sending emails."
	case inputSMTPPort:
		tip = "The port for the SMTP server (usually 587 for TLS)."
	}

	isGmail := m.inputs[inputProvider].Value() == "gmail"

	views := []string{
		titleStyle.Render(title),
		"Enter your email account credentials.",
		customHint,
		m.inputs[inputProvider].View(),
		m.inputs[inputName].View(),
		m.inputs[inputEmail].View(),
		m.inputs[inputFetchEmail].View(),
	}

	// Show auth method selector for Gmail
	if isGmail {
		views = append(views, m.inputs[inputAuthMethod].View())
	}

	// Hide password field when using OAuth2
	if !m.useOAuth2 {
		views = append(views, m.inputs[inputPassword].View())
	} else {
		views = append(views, accountEmailStyle.Render("OAuth2 selected — browser authorization will open after submit"))
	}

	if m.showCustom {
		views = append(views,
			"",
			listHeader.Render("Custom Server Settings:"),
			m.inputs[inputIMAPServer].View(),
			m.inputs[inputIMAPPort].View(),
			m.inputs[inputSMTPServer].View(),
			m.inputs[inputSMTPPort].View(),
		)
	}

	views = append(views, "")
	if !m.hideTips && tip != "" {
		views = append(views, TipStyle.Render("Tip: "+tip))
	}
	views = append(views, helpStyle.Render("\nenter: save • tab: next field • esc: back to menu"))

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, views...))
}

// SetEditMode sets the login form to edit an existing account.
func (m *Login) SetEditMode(accountID, provider, name, email, fetchEmail, imapServer string, imapPort int, smtpServer string, smtpPort int) {
	m.isEditMode = true
	m.accountID = accountID
	m.inputs[inputProvider].SetValue(provider)
	m.inputs[inputName].SetValue(name)
	m.inputs[inputEmail].SetValue(email)
	m.inputs[inputFetchEmail].SetValue(fetchEmail)
	m.showCustom = provider == "custom"

	if m.showCustom {
		m.inputs[inputIMAPServer].SetValue(imapServer)
		if imapPort != 0 {
			m.inputs[inputIMAPPort].SetValue(strconv.Itoa(imapPort))
		}
		m.inputs[inputSMTPServer].SetValue(smtpServer)
		if smtpPort != 0 {
			m.inputs[inputSMTPPort].SetValue(strconv.Itoa(smtpPort))
		}
	}
}

// GetAccountID returns the account ID being edited (if in edit mode).
func (m *Login) GetAccountID() string {
	return m.accountID
}

// IsEditMode returns whether the form is in edit mode.
func (m *Login) IsEditMode() bool {
	return m.isEditMode
}
