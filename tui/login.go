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
	inputProtocol = iota // "imap", "jmap", or "pop3"
	inputProvider        // "gmail", "icloud", or "custom"
	inputName
	inputEmail
	inputFetchEmail
	inputSendAsEmail
	inputAuthMethod // "password" or "oauth2" (shown for gmail)
	inputPassword
	inputIMAPServer
	inputIMAPPort
	inputSMTPServer
	inputSMTPPort
	inputInsecure
	inputJMAPEndpoint // JMAP session URL
	inputPOP3Server
	inputPOP3Port
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
		case inputProtocol:
			t.Placeholder = "Protocol (imap, jmap, or pop3)"
			t.Focus()
			t.Prompt = "🌐 > "
		case inputProvider:
			t.Placeholder = "Provider (gmail, outlook, icloud, or custom)"
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
		case inputSendAsEmail:
			t.Placeholder = "Send As Email (optional From header override)"
			t.Prompt = "✉️ > "
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
		case inputInsecure:
			t.Placeholder = "Insecure (true/false) - Skip TLS verification"
			t.Prompt = "🔓 > "
		case inputJMAPEndpoint:
			t.Placeholder = "JMAP Session URL (e.g., https://api.fastmail.com/jmap/session)"
			t.Prompt = "🔗 > "
		case inputPOP3Server:
			t.Placeholder = "POP3 Server (e.g., pop.example.com)"
			t.Prompt = "📥 > "
		case inputPOP3Port:
			t.Placeholder = "POP3 Port (default: 995)"
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

// protocol returns the currently selected protocol (defaults to "imap").
func (m *Login) protocol() string {
	p := m.inputs[inputProtocol].Value()
	if p == "" {
		return "imap"
	}
	return p
}

// visibleFields returns the ordered list of input indices the user should see
// for the current protocol/provider/auth combination.
func (m *Login) visibleFields() []int {
	proto := m.protocol()
	provider := m.inputs[inputProvider].Value()
	hasOAuth := provider == "gmail" || provider == "outlook"

	fields := []int{inputProtocol}

	switch proto {
	case "jmap":
		// JMAP: no provider selector, just endpoint + common fields
		fields = append(fields, inputName, inputEmail, inputFetchEmail, inputSendAsEmail, inputPassword, inputJMAPEndpoint)
	case "pop3":
		// POP3: custom server fields + SMTP for sending
		fields = append(fields, inputName, inputEmail, inputFetchEmail, inputSendAsEmail, inputPassword,
			inputPOP3Server, inputPOP3Port, inputSMTPServer, inputSMTPPort, inputInsecure)
	default:
		// IMAP (default): existing flow
		fields = append(fields, inputProvider, inputName, inputEmail, inputFetchEmail, inputSendAsEmail)
		if hasOAuth {
			fields = append(fields, inputAuthMethod)
		}
		if !m.useOAuth2 {
			fields = append(fields, inputPassword)
		}
		if m.showCustom {
			fields = append(fields, inputIMAPServer, inputIMAPPort, inputSMTPServer, inputSMTPPort, inputInsecure)
		}
	}

	return fields
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
			m.updateFlags()
			visible := m.visibleFields()
			lastField := visible[len(visible)-1]

			if m.focusIndex == lastField {
				return m, m.submitForm()
			}
			fallthrough

		case "tab", "shift+tab", "up", "down":
			s := msg.String()
			m.updateFlags()
			visible := m.visibleFields()

			// Find current position in visible fields
			curPos := 0
			for i, f := range visible {
				if f == m.focusIndex {
					curPos = i
					break
				}
			}

			if s == "up" || s == "shift+tab" {
				curPos--
			} else {
				curPos++
			}

			if curPos >= len(visible) {
				curPos = 0
			} else if curPos < 0 {
				curPos = len(visible) - 1
			}

			m.focusIndex = visible[curPos]

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

	m.updateFlags()

	return m, tea.Batch(cmds...)
}

// updateFlags recalculates showCustom and useOAuth2 from current inputs.
func (m *Login) updateFlags() {
	provider := m.inputs[inputProvider].Value()
	m.showCustom = provider == "custom"
	m.useOAuth2 = m.inputs[inputAuthMethod].Value() == "oauth2"
}

// submitForm builds and returns a Credentials message from the current inputs.
func (m *Login) submitForm() func() tea.Msg {
	imapPort := 993
	smtpPort := 587
	pop3Port := 995
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
	if m.inputs[inputPOP3Port].Value() != "" {
		if p, err := strconv.Atoi(m.inputs[inputPOP3Port].Value()); err == nil {
			pop3Port = p
		}
	}

	authMethod := "password"
	if m.useOAuth2 {
		authMethod = "oauth2"
	}

	proto := m.protocol()

	insecure := m.inputs[inputInsecure].Value() == "true"

	return func() tea.Msg {
		return Credentials{
			Protocol:     proto,
			Provider:     m.inputs[inputProvider].Value(),
			Name:         m.inputs[inputName].Value(),
			Host:         m.inputs[inputEmail].Value(),
			FetchEmail:   m.inputs[inputFetchEmail].Value(),
			SendAsEmail:  m.inputs[inputSendAsEmail].Value(),
			Password:     m.inputs[inputPassword].Value(),
			IMAPServer:   m.inputs[inputIMAPServer].Value(),
			IMAPPort:     imapPort,
			SMTPServer:   m.inputs[inputSMTPServer].Value(),
			SMTPPort:     smtpPort,
			Insecure:     insecure,
			AuthMethod:   authMethod,
			JMAPEndpoint: m.inputs[inputJMAPEndpoint].Value(),
			POP3Server:   m.inputs[inputPOP3Server].Value(),
			POP3Port:     pop3Port,
		}
	}
}

// View renders the login form.
func (m *Login) View() tea.View {
	title := "Add Account"
	if m.isEditMode {
		title = "Edit Account"
	}

	proto := m.protocol()

	tip := ""
	switch m.focusIndex {
	case inputProtocol:
		tip = "Choose the protocol: imap (default), jmap, or pop3."
	case inputProvider:
		tip = "Enter your email provider (e.g., gmail, outlook, icloud) or 'custom'."
	case inputName:
		tip = "The name that will appear on emails you send."
	case inputEmail:
		tip = "Your full email address used to log in."
	case inputFetchEmail:
		tip = "The email address to fetch messages for (comma-separated for multiple, e.g. me@icloud.com,me@mac.com)."
	case inputSendAsEmail:
		tip = "Optional From header override for outgoing email. Leave blank to send as the fetched address."
	case inputAuthMethod:
		tip = "Type 'oauth2' for OAuth2 or 'password' for app password."
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
	case inputInsecure:
		tip = "Type 'true' to disable TLS certificate verification (not recommended)."
	case inputJMAPEndpoint:
		tip = "The JMAP session resource URL (e.g., https://api.fastmail.com/jmap/session)."
	case inputPOP3Server:
		tip = "The POP3 server address for receiving emails."
	case inputPOP3Port:
		tip = "The port for the POP3 server (usually 995 for SSL)."
	}

	views := []string{
		titleStyle.Render(title),
		"Enter your email account credentials.",
		"",
		m.inputs[inputProtocol].View(),
	}

	switch proto {
	case "jmap":
		views = append(views,
			m.inputs[inputName].View(),
			m.inputs[inputEmail].View(),
			m.inputs[inputFetchEmail].View(),
			m.inputs[inputSendAsEmail].View(),
			m.inputs[inputPassword].View(),
			"",
			listHeader.Render("JMAP Settings:"),
			m.inputs[inputJMAPEndpoint].View(),
		)
	case "pop3":
		views = append(views,
			m.inputs[inputName].View(),
			m.inputs[inputEmail].View(),
			m.inputs[inputFetchEmail].View(),
			m.inputs[inputSendAsEmail].View(),
			m.inputs[inputPassword].View(),
			"",
			listHeader.Render("POP3 Server Settings:"),
			m.inputs[inputPOP3Server].View(),
			m.inputs[inputPOP3Port].View(),
			"",
			listHeader.Render("SMTP Settings (for sending):"),
			m.inputs[inputSMTPServer].View(),
			m.inputs[inputSMTPPort].View(),
			m.inputs[inputInsecure].View(),
		)
	default:
		// IMAP flow
		provider := m.inputs[inputProvider].Value()
		hasOAuth := provider == "gmail" || provider == "outlook"
		views = append(views,
			m.inputs[inputProvider].View(),
			m.inputs[inputName].View(),
			m.inputs[inputEmail].View(),
			m.inputs[inputFetchEmail].View(),
			m.inputs[inputSendAsEmail].View(),
		)

		if hasOAuth {
			views = append(views, m.inputs[inputAuthMethod].View())
		}

		if !m.useOAuth2 {
			views = append(views, m.inputs[inputPassword].View())
		} else {
			views = append(views, accountEmailStyle.Render("OAuth2 selected — browser authorization will open after submit"))
		}

		if m.showCustom {
			customHint := accountEmailStyle.Render("Custom provider selected - configure server settings below")
			views = append(views,
				"",
				customHint,
				m.inputs[inputIMAPServer].View(),
				m.inputs[inputIMAPPort].View(),
				m.inputs[inputSMTPServer].View(),
				m.inputs[inputSMTPPort].View(),
				m.inputs[inputInsecure].View(),
			)
		}
	}

	views = append(views, "")
	if !m.hideTips && tip != "" {
		views = append(views, TipStyle.Render("Tip: "+tip))
	}
	views = append(views, helpStyle.Render("\nenter: save • tab: next field • esc: back to menu"))

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, views...))
}

// SetEditMode sets the login form to edit an existing account.
func (m *Login) SetEditMode(accountID, protocol, provider, name, email, fetchEmail, sendAsEmail, imapServer string, imapPort int, smtpServer string, smtpPort int, insecure bool, jmapEndpoint, pop3Server string, pop3Port int) {
	m.isEditMode = true
	m.accountID = accountID

	if protocol == "" {
		protocol = "imap"
	}
	m.inputs[inputProtocol].SetValue(protocol)
	m.inputs[inputProvider].SetValue(provider)
	m.inputs[inputName].SetValue(name)
	m.inputs[inputEmail].SetValue(email)
	m.inputs[inputFetchEmail].SetValue(fetchEmail)
	m.inputs[inputSendAsEmail].SetValue(sendAsEmail)
	m.showCustom = provider == "custom"

	if m.showCustom {
		m.inputs[inputIMAPServer].SetValue(imapServer)
		if insecure {
			m.inputs[inputInsecure].SetValue("true")
		} else {
			m.inputs[inputInsecure].SetValue("false")
		}
		if imapPort != 0 {
			m.inputs[inputIMAPPort].SetValue(strconv.Itoa(imapPort))
		}
		m.inputs[inputSMTPServer].SetValue(smtpServer)
		if smtpPort != 0 {
			m.inputs[inputSMTPPort].SetValue(strconv.Itoa(smtpPort))
		}
	}

	if jmapEndpoint != "" {
		m.inputs[inputJMAPEndpoint].SetValue(jmapEndpoint)
	}
	if pop3Server != "" {
		m.inputs[inputPOP3Server].SetValue(pop3Server)
	}
	if pop3Port != 0 {
		m.inputs[inputPOP3Port].SetValue(strconv.Itoa(pop3Port))
	}
	// Also set SMTP for POP3
	if protocol == "pop3" {
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
