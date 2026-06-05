package tui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/theme"
)

// Supported account protocols.
const (
	protocolIMAP    = "imap"
	protocolJMAP    = "jmap"
	protocolPOP3    = "pop3"
	protocolMaildir = "maildir"
)

// loginProtocols are the selectable protocols shown in the protocol combobox,
// in cycle order.
var loginProtocols = []string{protocolIMAP, protocolJMAP, protocolPOP3, protocolMaildir}

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
	inputCatchAll     // "true/false" — show all inbox messages regardless of To address
	inputJMAPEndpoint // JMAP session URL
	inputPOP3Server
	inputPOP3Port
	inputMaildirPath // Local Maildir root path
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
			// Rendered as a combobox (see viewProtocolCombobox); the textinput
			// is only used to hold the selected value. Seed a default so a
			// protocol is always selected.
			t.SetValue(loginProtocols[0])
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
			t.Placeholder = "Auth Method (password, oauth2, or token)"
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
		case inputCatchAll:
			t.Placeholder = "Catch-All (true/false) - Show all inbox messages"
			t.Prompt = "📬 > "
		case inputJMAPEndpoint:
			t.Placeholder = "JMAP Session URL (e.g., https://api.fastmail.com/jmap/session)"
			t.Prompt = "🔗 > "
		case inputPOP3Server:
			t.Placeholder = "POP3 Server (e.g., pop.example.com)"
			t.Prompt = "📥 > "
		case inputPOP3Port:
			t.Placeholder = "POP3 Port (default: 995)"
			t.Prompt = "🔢 > "
		case inputMaildirPath:
			t.Placeholder = "Maildir Path (e.g., ~/Mail or /var/mail/user)"
			t.Prompt = "📁 > "
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
		return protocolIMAP
	}
	return p
}

// cycleProtocol moves the protocol selection by delta (+1 next, -1 previous),
// wrapping around the loginProtocols list.
func (m *Login) cycleProtocol(delta int) {
	cur := m.protocol()
	idx := 0
	for i, p := range loginProtocols {
		if p == cur {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(loginProtocols)) % len(loginProtocols)
	m.inputs[inputProtocol].SetValue(loginProtocols[idx])
}

// visibleFields returns the ordered list of input indices the user should see
// for the current protocol/provider/auth combination.
func (m *Login) visibleFields() []int {
	proto := m.protocol()
	provider := m.inputs[inputProvider].Value()
	hasOAuth := provider == "gmail" || provider == "outlook"

	fields := []int{inputProtocol}

	switch proto {
	case protocolJMAP:
		// JMAP: no provider selector, just endpoint + common fields
		fields = append(fields, inputName, inputEmail, inputFetchEmail, inputSendAsEmail, inputCatchAll, inputAuthMethod, inputPassword, inputJMAPEndpoint)
	case protocolPOP3:
		// POP3: custom server fields + SMTP for sending
		fields = append(fields, inputName, inputEmail, inputFetchEmail, inputSendAsEmail, inputCatchAll, inputPassword,
			inputPOP3Server, inputPOP3Port, inputSMTPServer, inputSMTPPort, inputInsecure)
	case protocolMaildir:
		// Maildir: local filesystem only — no auth, no network.
		fields = append(fields, inputName, inputEmail, inputFetchEmail, inputSendAsEmail, inputCatchAll, inputMaildirPath)
	default:
		// IMAP (default): existing flow
		fields = append(fields, inputProvider, inputName, inputEmail, inputFetchEmail, inputSendAsEmail, inputCatchAll)
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

		case keyLeft, keyRight, "h", "l", "space":
			// On the protocol combobox, left/right (or h/l, space) cycle the
			// selection instead of editing text. Elsewhere, fall through so the
			// focused textinput handles the key normally.
			if m.focusIndex == inputProtocol {
				if msg.String() == keyLeft || msg.String() == "h" {
					m.cycleProtocol(-1)
				} else {
					m.cycleProtocol(1)
				}
				return m, nil
			}

		case "ctrl+v":
			// Toggle password visibility while focused on the password field,
			// so typos in app-passwords are catchable without retyping.
			if m.focusIndex == inputPassword {
				if m.inputs[inputPassword].EchoMode == textinput.EchoPassword {
					m.inputs[inputPassword].EchoMode = textinput.EchoNormal
				} else {
					m.inputs[inputPassword].EchoMode = textinput.EchoPassword
				}
				return m, nil
			}

		case keyEnter:
			m.updateFlags()
			visible := m.visibleFields()
			lastField := visible[len(visible)-1]

			if m.focusIndex == lastField {
				return m, m.submitForm()
			}
			fallthrough

		case "tab", keyShiftTab, "up", keyDown:
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

			if s == "up" || s == keyShiftTab {
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

	// Update the focused input field. The protocol field is a combobox, not a
	// text field, so it never consumes raw input here.
	var cmds = make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		if i == inputProtocol {
			continue
		}
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

	authMethod := m.inputs[inputAuthMethod].Value()
	if m.protocol() == protocolJMAP && (authMethod == "token" || authMethod == "") {
		m.inputs[inputPassword].Placeholder = "API Token"
	} else {
		m.inputs[inputPassword].Placeholder = "Password / App Password"
	}
}

// validPort parses a port string and returns the integer value if it is within
// the valid TCP/UDP port range (1-65535). Returns the fallback if the string is
// empty or invalid.
func validPort(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return fallback
	}
	return p
}

// submitForm builds and returns a Credentials message from the current inputs.
func (m *Login) submitForm() func() tea.Msg {
	imapPort := validPort(m.inputs[inputIMAPPort].Value(), 993)
	smtpPort := validPort(m.inputs[inputSMTPPort].Value(), 587)
	pop3Port := validPort(m.inputs[inputPOP3Port].Value(), 995)

	proto := m.protocol()

	var authMethod string
	switch {
	case proto == protocolJMAP:
		authMethod = m.inputs[inputAuthMethod].Value()
		if authMethod == "" {
			authMethod = "token"
		}
	case m.useOAuth2:
		authMethod = "oauth2"
	default:
		authMethod = "password"
	}

	insecure := m.inputs[inputInsecure].Value() == "true"
	catchAll := m.inputs[inputCatchAll].Value() == "true"

	return func() tea.Msg {
		return Credentials{
			Protocol:     proto,
			Provider:     m.inputs[inputProvider].Value(),
			Name:         m.inputs[inputName].Value(),
			Host:         m.inputs[inputEmail].Value(),
			FetchEmail:   m.inputs[inputFetchEmail].Value(),
			SendAsEmail:  m.inputs[inputSendAsEmail].Value(),
			CatchAll:     catchAll,
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
			MaildirPath:  m.inputs[inputMaildirPath].Value(),
		}
	}
}

// viewProtocolCombobox renders the protocol selector as a segmented combobox,
// highlighting the current selection and dimming the alternatives.
func (m *Login) viewProtocolCombobox() string {
	th := theme.ActiveTheme
	focused := m.focusIndex == inputProtocol
	cur := m.protocol()

	promptColor := th.MutedText
	if focused {
		promptColor = th.AccentText
	}
	prompt := lipgloss.NewStyle().Foreground(promptColor).Render("🌐 > ")

	selStyle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	optStyle := lipgloss.NewStyle().Foreground(th.DimText)

	parts := make([]string, len(loginProtocols))
	for i, p := range loginProtocols {
		switch {
		case p == cur && focused:
			parts[i] = selStyle.Render("‹ " + p + " ›")
		case p == cur:
			parts[i] = selStyle.Render("  " + p + "  ")
		default:
			parts[i] = optStyle.Render("  " + p + "  ")
		}
	}

	return prompt + strings.Join(parts, "")
}

// protocolFieldViews returns the rendered input fields specific to the given
// protocol, in display order.
func (m *Login) protocolFieldViews(proto string) []string {
	common := []string{
		m.inputs[inputName].View(),
		m.inputs[inputEmail].View(),
		m.inputs[inputFetchEmail].View(),
		m.inputs[inputSendAsEmail].View(),
		m.inputs[inputCatchAll].View(),
	}

	switch proto {
	case protocolJMAP:
		return append(common,
			m.inputs[inputAuthMethod].View(),
			m.inputs[inputPassword].View(),
			"",
			listHeader.Render("JMAP Settings:"),
			m.inputs[inputJMAPEndpoint].View(),
		)
	case protocolPOP3:
		return append(common,
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
	case protocolMaildir:
		return append(common,
			"",
			listHeader.Render("Maildir Settings:"),
			m.inputs[inputMaildirPath].View(),
		)
	default:
		return m.imapFieldViews(common)
	}
}

// imapFieldViews renders the IMAP-specific fields (provider selector, optional
// OAuth2/password, and custom server settings) appended after the common fields.
func (m *Login) imapFieldViews(common []string) []string {
	provider := m.inputs[inputProvider].Value()
	hasOAuth := provider == "gmail" || provider == "outlook"

	views := append([]string{m.inputs[inputProvider].View()}, common...)

	if hasOAuth {
		views = append(views, m.inputs[inputAuthMethod].View())
	}

	if !m.useOAuth2 {
		views = append(views, m.inputs[inputPassword].View())
	} else {
		views = append(views, accountEmailStyle.Render("OAuth2 selected — browser authorization will open after submit"))
	}

	if m.showCustom {
		views = append(views,
			"",
			accountEmailStyle.Render("Custom provider selected - configure server settings below"),
			m.inputs[inputIMAPServer].View(),
			m.inputs[inputIMAPPort].View(),
			m.inputs[inputSMTPServer].View(),
			m.inputs[inputSMTPPort].View(),
			m.inputs[inputInsecure].View(),
		)
	}

	return views
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
		tip = "Use ←/→ to choose the protocol: imap (default), jmap, pop3, or maildir."
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
		if m.protocol() == protocolJMAP {
			tip = "Type 'token' for API token (Bearer auth, default) or 'password' for HTTP Basic auth."
		} else {
			tip = "Type 'oauth2' for OAuth2 or 'password' for app password."
		}
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
	case inputCatchAll:
		tip = "Type 'true' to show all inbox messages regardless of To address (useful for catch-all domains)."
	case inputJMAPEndpoint:
		tip = "The JMAP session resource URL (e.g., https://api.fastmail.com/jmap/session)."
	case inputPOP3Server:
		tip = "The POP3 server address for receiving emails."
	case inputPOP3Port:
		tip = "The port for the POP3 server (usually 995 for SSL)."
	case inputMaildirPath:
		tip = "Local path to a Maildir directory (cur/new/tmp). Subfolders use .Foldername (Maildir++)."
	}

	views := []string{
		titleStyle.Render(title),
		"Enter your email account credentials.",
		"",
		m.viewProtocolCombobox(),
	}

	views = append(views, m.protocolFieldViews(proto)...)

	views = append(views, "")
	if !m.hideTips && tip != "" {
		views = append(views, TipStyle.Render("Tip: "+tip))
	}
	helpLine := "enter: save • tab: next field • esc: back to menu"
	if m.focusIndex == inputProtocol {
		helpLine += " • ←/→: change protocol"
	}
	if m.focusIndex == inputPassword {
		helpLine += " • ctrl+v: toggle password visibility"
	}
	views = append(views, helpStyle.Render("\n"+helpLine))

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, views...))
}

// SetEditMode sets the login form to edit an existing account.
func (m *Login) SetEditMode(accountID, protocol, provider, name, email, fetchEmail, sendAsEmail, imapServer string, imapPort int, smtpServer string, smtpPort int, insecure bool, jmapEndpoint, pop3Server string, pop3Port int, catchAll bool, maildirPath string) {
	m.isEditMode = true
	m.accountID = accountID

	if protocol == "" {
		protocol = protocolIMAP
	}
	m.inputs[inputProtocol].SetValue(protocol)
	m.inputs[inputProvider].SetValue(provider)
	m.inputs[inputName].SetValue(name)
	m.inputs[inputEmail].SetValue(email)
	m.inputs[inputFetchEmail].SetValue(fetchEmail)
	m.inputs[inputSendAsEmail].SetValue(sendAsEmail)
	if catchAll {
		m.inputs[inputCatchAll].SetValue("true")
	} else {
		m.inputs[inputCatchAll].SetValue("false")
	}
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
	if protocol == protocolPOP3 {
		m.inputs[inputSMTPServer].SetValue(smtpServer)
		if smtpPort != 0 {
			m.inputs[inputSMTPPort].SetValue(strconv.Itoa(smtpPort))
		}
	}
	if maildirPath != "" {
		m.inputs[inputMaildirPath].SetValue(maildirPath)
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
