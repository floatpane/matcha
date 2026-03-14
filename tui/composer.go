package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/google/uuid"
)

var (
	suggestionStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectedSuggestionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	suggestionBoxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("245")).Padding(0, 1)
)

// Styles for the UI
var (
	focusedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	blurredStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	noStyle             = lipgloss.NewStyle()
	helpStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	focusedButton       = focusedStyle.Copy().Render("[ Send ]")
	blurredButton       = blurredStyle.Copy().Render("[ Send ]")
	emailRecipientStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	attachmentStyle     = lipgloss.NewStyle().PaddingLeft(4).Foreground(lipgloss.Color("245"))
	fromSelectorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	smimeToggleStyle    = lipgloss.NewStyle().PaddingLeft(4).Foreground(lipgloss.Color("245"))
)

const (
	focusFrom = iota
	focusTo
	focusCc
	focusBcc
	focusSubject
	focusBody
	focusSignature
	focusAttachment
	focusEncryptSMIME
	focusSend
)

// Composer model holds the state of the email composition UI.
type Composer struct {
	focusIndex      int
	toInput         textinput.Model
	ccInput         textinput.Model
	bccInput        textinput.Model
	subjectInput    textinput.Model
	bodyInput       textarea.Model
	signatureInput  textarea.Model
	attachmentPaths []string
	encryptSMIME    bool
	width           int
	height          int
	confirmingExit  bool
	hideTips        bool

	// Multi-account support
	accounts           []config.Account
	selectedAccountIdx int
	showAccountPicker  bool

	// Contact suggestions
	suggestions        []config.Contact
	selectedSuggestion int
	showSuggestions    bool
	lastToValue        string

	// Draft persistence
	draftID string

	// Reply context
	inReplyTo  string
	references []string

	// Hidden quoted text (appended to body when sending, but not shown in editor)
	quotedText string
}

// NewComposer initializes a new composer model.
func NewComposer(from, to, subject, body string, hideTips bool) *Composer {
	m := &Composer{
		draftID:  uuid.New().String(),
		hideTips: hideTips,
	}

	m.toInput = textinput.New()
	m.toInput.Placeholder = "To"
	m.toInput.SetValue(to)
	m.toInput.Prompt = "> "
	m.toInput.CharLimit = 256

	m.ccInput = textinput.New()
	m.ccInput.Placeholder = "Cc"
	m.ccInput.Prompt = "> "
	m.ccInput.CharLimit = 256

	m.bccInput = textinput.New()
	m.bccInput.Placeholder = "Bcc"
	m.bccInput.Prompt = "> "
	m.bccInput.CharLimit = 256

	m.subjectInput = textinput.New()
	m.subjectInput.Placeholder = "Subject"
	m.subjectInput.SetValue(subject)
	m.subjectInput.Prompt = "> "
	m.subjectInput.CharLimit = 256

	m.bodyInput = textarea.New()
	m.bodyInput.Placeholder = "Body (Markdown supported)..."
	m.bodyInput.SetValue(body)
	m.bodyInput.Prompt = "> "
	m.bodyInput.SetHeight(10)

	m.signatureInput = textarea.New()
	m.signatureInput.Placeholder = "Signature (optional)..."
	m.signatureInput.Prompt = "> "
	m.signatureInput.SetHeight(3)
	// Load default signature
	if sig, err := config.LoadSignature(); err == nil && sig != "" {
		m.signatureInput.SetValue(sig)
	}

	// Start focus on To field (From is selectable but not a text input)
	m.focusIndex = focusTo
	m.toInput.Focus()

	return m
}

// NewComposerWithAccounts initializes a composer with multiple account support.
func NewComposerWithAccounts(accounts []config.Account, selectedAccountID string, to, subject, body string, hideTips bool) *Composer {
	m := NewComposer("", to, subject, body, hideTips)
	m.accounts = accounts

	// Find the selected account index
	for i, acc := range accounts {
		if acc.ID == selectedAccountID {
			m.selectedAccountIdx = i
			break
		}
	}

	return m
}

// ResetConfirmation ensures a restored draft isnt stuck in the exit prompt.
func (m *Composer) ResetConfirmation() {
	m.confirmingExit = false
}

func (m *Composer) Init() tea.Cmd {
	return textinput.Blink
}

func (m *Composer) getFromAddress() string {
	if len(m.accounts) > 0 && m.selectedAccountIdx < len(m.accounts) {
		acc := m.accounts[m.selectedAccountIdx]
		if acc.Name != "" {
			return fmt.Sprintf("%s <%s>", acc.Name, acc.FetchEmail)
		}
		return acc.FetchEmail
	}
	return ""
}

func (m *Composer) getSelectedAccount() *config.Account {
	if len(m.accounts) > 0 && m.selectedAccountIdx < len(m.accounts) {
		return &m.accounts[m.selectedAccountIdx]
	}
	return nil
}

func (m *Composer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputWidth := msg.Width - 6
		m.toInput.SetWidth(inputWidth)
		m.ccInput.SetWidth(inputWidth)
		m.bccInput.SetWidth(inputWidth)
		m.subjectInput.SetWidth(inputWidth)
		m.bodyInput.SetWidth(inputWidth)
		m.signatureInput.SetWidth(inputWidth)
		if msg.Height > 0 {
			// Fixed rows: title, from, to, cc, bcc, subject, sig label,
			// attachment, smime, button, blank, tip, help = 13
			const fixedRows = 13
			available := msg.Height - fixedRows
			if available < 6 {
				available = 6
			}
			bodyHeight := (available * 55) / 100
			sigHeight := (available * 15) / 100
			if bodyHeight < 3 {
				bodyHeight = 3
			}
			if sigHeight < 2 {
				sigHeight = 2
			}
			m.bodyInput.SetHeight(bodyHeight)
			m.signatureInput.SetHeight(sigHeight)
		}

	case FileSelectedMsg:
		// Avoid duplicates
		for _, p := range m.attachmentPaths {
			if p == msg.Path {
				return m, nil
			}
		}
		m.attachmentPaths = append(m.attachmentPaths, msg.Path)
		return m, nil

	case tea.KeyPressMsg:
		// Handle contact suggestions mode
		if m.showSuggestions && len(m.suggestions) > 0 {
			switch msg.String() {
			case "up", "ctrl+p":
				if m.selectedSuggestion > 0 {
					m.selectedSuggestion--
				}
				return m, nil
			case "down", "ctrl+n":
				if m.selectedSuggestion < len(m.suggestions)-1 {
					m.selectedSuggestion++
				}
				return m, nil
			case "tab", "enter":
				// Select the suggestion
				selected := m.suggestions[m.selectedSuggestion]

				var newEmail string
				if strings.Contains(selected.Email, ",") {
					// It's a mailing list: insert just the addresses to maintain valid email formatting
					newEmail = selected.Email
				} else if selected.Name != "" && selected.Name != selected.Email {
					newEmail = fmt.Sprintf("%s <%s>", selected.Name, selected.Email)
				} else {
					newEmail = selected.Email
				}

				parts := strings.Split(m.toInput.Value(), ",")
				if len(parts) > 0 {
					if len(parts) == 1 {
						parts[0] = newEmail
					} else {
						parts[len(parts)-1] = " " + newEmail
					}
				} else {
					parts = []string{newEmail}
				}

				finalValue := strings.Join(parts, ",")
				if !strings.HasSuffix(finalValue, ", ") {
					finalValue += ", "
				}

				m.toInput.SetValue(finalValue)
				m.toInput.SetCursor(len(finalValue))
				m.lastToValue = m.toInput.Value()
				m.showSuggestions = false
				m.suggestions = nil
				return m, nil
			case "esc":
				m.showSuggestions = false
				m.suggestions = nil
				return m, nil
			}
			// For shift+tab, close suggestions and let it fall through to normal handling
			if msg.String() == "shift+tab" {
				m.showSuggestions = false
				m.suggestions = nil
			}
		}

		// Handle account picker mode
		if m.showAccountPicker {
			switch msg.String() {
			case "up", "k":
				if m.selectedAccountIdx > 0 {
					m.selectedAccountIdx--
				}
			case "down", "j":
				if m.selectedAccountIdx < len(m.accounts)-1 {
					m.selectedAccountIdx++
				}
			case "enter":
				m.showAccountPicker = false
			case "esc":
				m.showAccountPicker = false
			}
			return m, nil
		}

		if m.confirmingExit {
			switch msg.String() {
			case "y", "Y":
				return m, func() tea.Msg { return DiscardDraftMsg{ComposerState: m} }
			case "n", "N", "esc":
				m.confirmingExit = false
				return m, nil
			default:
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.confirmingExit = true
			return m, nil

		case "tab", "shift+tab":
			if msg.String() == "shift+tab" {
				m.focusIndex--
			} else {
				m.focusIndex++
			}

			maxFocus := focusSend
			minFocus := focusFrom
			// Skip From field if only one account (nothing to switch)
			if len(m.accounts) <= 1 {
				minFocus = focusTo
			}

			if m.focusIndex > maxFocus {
				m.focusIndex = minFocus
			} else if m.focusIndex < minFocus {
				m.focusIndex = maxFocus
			}

			m.toInput.Blur()
			m.ccInput.Blur()
			m.bccInput.Blur()
			m.subjectInput.Blur()
			m.bodyInput.Blur()
			m.signatureInput.Blur()

			switch m.focusIndex {
			case focusTo:
				cmds = append(cmds, m.toInput.Focus())
			case focusCc:
				cmds = append(cmds, m.ccInput.Focus())
			case focusBcc:
				cmds = append(cmds, m.bccInput.Focus())
			case focusSubject:
				cmds = append(cmds, m.subjectInput.Focus())
			case focusBody:
				cmds = append(cmds, m.bodyInput.Focus())
			case focusSignature:
				cmds = append(cmds, m.signatureInput.Focus())
			}
			return m, tea.Batch(cmds...)

		case "backspace", "delete", "d":
			if m.focusIndex == focusAttachment && len(m.attachmentPaths) > 0 {
				m.attachmentPaths = m.attachmentPaths[:len(m.attachmentPaths)-1]
				return m, nil
			}

		case "enter", " ":
			switch m.focusIndex {
			case focusFrom:
				if len(m.accounts) > 1 && msg.String() == "enter" {
					m.showAccountPicker = true
				}
				return m, nil
			case focusAttachment:
				if msg.String() == "enter" {
					return m, func() tea.Msg { return GoToFilePickerMsg{} }
				}
			case focusEncryptSMIME:
				if msg.String() == "enter" || msg.String() == " " {
					m.encryptSMIME = !m.encryptSMIME
				}
				return m, nil
			case focusSend:
				if msg.String() == "enter" {
					acc := m.getSelectedAccount()
					accountID := ""
					if acc != nil {
						accountID = acc.ID
					}
					return m, func() tea.Msg {
						return SendEmailMsg{
							To:              m.toInput.Value(),
							Cc:              m.ccInput.Value(),
							Bcc:             m.bccInput.Value(),
							Subject:         m.subjectInput.Value(),
							Body:            m.bodyInput.Value(),
							AttachmentPaths: m.attachmentPaths,
							AccountID:       accountID,
							QuotedText:      m.quotedText,
							InReplyTo:       m.inReplyTo,
							References:      m.references,
							Signature:       m.signatureInput.Value(),
							SignSMIME:       acc != nil && acc.SMIMESignByDefault,
							EncryptSMIME:    m.encryptSMIME,
						}
					}
				}
			}
		}
	}

	switch m.focusIndex {
	case focusTo:
		m.toInput, cmd = m.toInput.Update(msg)
		cmds = append(cmds, cmd)

		// Check if To field value changed and update suggestions
		currentValue := m.toInput.Value()
		if currentValue != m.lastToValue {
			m.lastToValue = currentValue

			// Extract the last comma-separated part for searching
			parts := strings.Split(currentValue, ",")
			lastPart := strings.TrimSpace(parts[len(parts)-1])

			if len(lastPart) >= 2 {
				m.suggestions = config.SearchContacts(lastPart)
				m.showSuggestions = len(m.suggestions) > 0
				m.selectedSuggestion = 0
			} else {
				m.showSuggestions = false
				m.suggestions = nil
			}
		}
	case focusCc:
		m.ccInput, cmd = m.ccInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusBcc:
		m.bccInput, cmd = m.bccInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusSubject:
		m.subjectInput, cmd = m.subjectInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusBody:
		m.bodyInput, cmd = m.bodyInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusSignature:
		m.signatureInput, cmd = m.signatureInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Composer) View() tea.View {
	var composerView strings.Builder
	var button string

	if m.focusIndex == focusSend {
		button = focusedButton
	} else {
		button = blurredButton
	}

	// From field with account selector
	fromAddr := m.getFromAddress()
	var fromField string
	if len(m.accounts) > 1 {
		if m.focusIndex == focusFrom {
			fromField = focusedStyle.Render(fmt.Sprintf("> From: %s [Enter to switch]", fromAddr))
		} else {
			fromField = blurredStyle.Render(fmt.Sprintf("  From: %s [switchable]", fromAddr))
		}
	} else if fromAddr != "" {
		fromField = "  From: " + emailRecipientStyle.Render(fromAddr)
	} else {
		fromField = blurredStyle.Render("  From: (no account configured)")
	}

	var attachmentField string
	if len(m.attachmentPaths) == 0 {
		attachmentText := "None (Enter to add)"
		if m.focusIndex == focusAttachment {
			attachmentField = focusedStyle.Render(fmt.Sprintf("> Attachments: %s", attachmentText))
		} else {
			attachmentField = blurredStyle.Render(fmt.Sprintf("  Attachments: %s", attachmentText))
		}
	} else {
		var names []string
		for _, p := range m.attachmentPaths {
			names = append(names, filepath.Base(p))
		}
		attachmentText := strings.Join(names, ", ")
		if m.focusIndex == focusAttachment {
			attachmentField = focusedStyle.Render(fmt.Sprintf("> Attachments (%d): %s", len(m.attachmentPaths), attachmentText))
		} else {
			attachmentField = blurredStyle.Render(fmt.Sprintf("  Attachments (%d): %s", len(m.attachmentPaths), attachmentText))
		}
	}

	encToggle := "[ ]"
	if m.encryptSMIME {
		encToggle = "[x]"
	}
	encField := blurredStyle.Render(fmt.Sprintf("  Encrypt Email (S/MIME): %s", encToggle))
	if m.focusIndex == focusEncryptSMIME {
		encField = focusedStyle.Render(fmt.Sprintf("> Encrypt Email (S/MIME): %s", encToggle))
	}

	// Build To field with suggestions
	toFieldView := m.toInput.View()
	if m.showSuggestions && len(m.suggestions) > 0 {
		var suggestionsBuilder strings.Builder
		for i, s := range m.suggestions {
			display := s.Email
			if s.Name != "" && s.Name != s.Email {
				display = fmt.Sprintf("%s <%s>", s.Name, s.Email)
			}
			if i == m.selectedSuggestion {
				suggestionsBuilder.WriteString(selectedSuggestionStyle.Render("> "+display) + "\n")
			} else {
				suggestionsBuilder.WriteString(suggestionStyle.Render("  "+display) + "\n")
			}
		}
		toFieldView = toFieldView + "\n" + suggestionBoxStyle.Render(strings.TrimSuffix(suggestionsBuilder.String(), "\n"))
	}

	// Signature field label
	var signatureLabel string
	if m.focusIndex == focusSignature {
		signatureLabel = focusedStyle.Render("Signature:")
	} else {
		signatureLabel = blurredStyle.Render("Signature:")
	}

	tip := ""
	switch m.focusIndex {
	case focusFrom:
		tip = "Select the account to send from."
	case focusTo:
		tip = "Enter recipient email addresses."
	case focusCc:
		tip = "Carbon copy recipients."
	case focusBcc:
		tip = "Blind carbon copy recipients."
	case focusSubject:
		tip = "The subject line of your email."
	case focusBody:
		tip = "The main content of your email. Markdown and HTML are supported."
	case focusSignature:
		tip = "Your email signature. This will be appended to the end of the email."
	case focusAttachment:
		tip = "Enter: add file • backspace/d: remove last attachment"
	case focusEncryptSMIME:
		tip = "Press Space or Enter to toggle S/MIME encryption on or off."
	case focusSend:
		tip = "Press Enter to send the email."
	}

	composerViewElements := []string{
		"Compose New Email",
		fromField,
		toFieldView,
		m.ccInput.View(),
		m.bccInput.View(),
		m.subjectInput.View(),
		m.bodyInput.View(),
		signatureLabel,
		m.signatureInput.View(),
		attachmentStyle.Render(attachmentField),
		smimeToggleStyle.Render(encField),
		button,
		"",
	}

	if !m.hideTips && tip != "" {
		composerViewElements = append(composerViewElements, TipStyle.Render("Tip: "+tip))
	}

	mainContent := lipgloss.JoinVertical(lipgloss.Left, composerViewElements...)
	helpView := helpStyle.Render("Markdown/HTML • tab/shift+tab: navigate • esc: save draft & exit")

	if m.height > 0 {
		currentHeight := lipgloss.Height(mainContent) + lipgloss.Height(helpView)
		gap := m.height - currentHeight
		if gap >= 0 {
			mainContent += strings.Repeat("\n", gap+1)
		} else {
			mainContent += "\n"
		}
	} else {
		mainContent += "\n\n"
	}

	composerView.WriteString(mainContent)
	composerView.WriteString(helpView)

	// Account picker overlay
	if m.showAccountPicker {
		var accountList strings.Builder
		accountList.WriteString("Select Account:\n\n")
		for i, acc := range m.accounts {
			display := acc.FetchEmail
			if acc.Name != "" {
				display = fmt.Sprintf("%s (%s)", acc.Name, acc.FetchEmail)
			}
			if i == m.selectedAccountIdx {
				accountList.WriteString(selectedItemStyle.Render(fmt.Sprintf("> %s", display)))
			} else {
				accountList.WriteString(itemStyle.Render(fmt.Sprintf("  %s", display)))
			}
			accountList.WriteString("\n")
		}
		accountList.WriteString("\n")
		accountList.WriteString(HelpStyle.Render("↑/↓: navigate • enter: select • esc: cancel"))

		dialog := DialogBoxStyle.Render(accountList.String())
		return tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog))
	}

	if m.confirmingExit {
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				"Discard draft?",
				HelpStyle.Render("\n(y/n)"),
			),
		)
		return tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog))
	}

	return tea.NewView(composerView.String())
}

// SetAccounts sets the available accounts for sending.
func (m *Composer) SetAccounts(accounts []config.Account) {
	m.accounts = accounts
	if m.selectedAccountIdx >= len(accounts) {
		m.selectedAccountIdx = 0
	}
}

// SetSelectedAccount sets the selected account by ID.
func (m *Composer) SetSelectedAccount(accountID string) {
	for i, acc := range m.accounts {
		if acc.ID == accountID {
			m.selectedAccountIdx = i
			return
		}
	}
}

// GetSelectedAccountID returns the ID of the currently selected account.
func (m *Composer) GetSelectedAccountID() string {
	if len(m.accounts) > 0 && m.selectedAccountIdx < len(m.accounts) {
		return m.accounts[m.selectedAccountIdx].ID
	}
	return ""
}

// GetDraftID returns the draft ID for this composer.
func (m *Composer) GetDraftID() string {
	return m.draftID
}

// SetDraftID sets the draft ID (for loading existing drafts).
func (m *Composer) SetDraftID(id string) {
	m.draftID = id
}

// GetTo returns the current To field value.
func (m *Composer) GetTo() string {
	return m.toInput.Value()
}

// GetSubject returns the current Subject field value.
func (m *Composer) GetSubject() string {
	return m.subjectInput.Value()
}

// GetBody returns the current Body field value.
func (m *Composer) GetBody() string {
	return m.bodyInput.Value()
}

// GetAttachmentPaths returns the current attachment paths.
func (m *Composer) GetAttachmentPaths() []string {
	return m.attachmentPaths
}

// GetSignature returns the current signature value.
func (m *Composer) GetSignature() string {
	return m.signatureInput.Value()
}

// SetReplyContext sets the reply context for the draft.
func (m *Composer) SetReplyContext(inReplyTo string, references []string) {
	m.inReplyTo = inReplyTo
	m.references = references
}

// SetQuotedText sets the hidden quoted text that will be appended when sending.
func (m *Composer) SetQuotedText(text string) {
	m.quotedText = text
}

// GetQuotedText returns the hidden quoted text.
func (m *Composer) GetQuotedText() string {
	return m.quotedText
}

// GetInReplyTo returns the In-Reply-To header value.
func (m *Composer) GetInReplyTo() string {
	return m.inReplyTo
}

// GetReferences returns the References header values.
func (m *Composer) GetReferences() []string {
	return m.references
}

// ToDraft converts the composer state to a Draft for saving.
func (m *Composer) ToDraft() config.Draft {
	return config.Draft{
		ID:              m.draftID,
		To:              m.toInput.Value(),
		Cc:              m.ccInput.Value(),
		Bcc:             m.bccInput.Value(),
		Subject:         m.subjectInput.Value(),
		Body:            m.bodyInput.Value(),
		AttachmentPaths: m.attachmentPaths,
		AccountID:       m.GetSelectedAccountID(),
		InReplyTo:       m.inReplyTo,
		References:      m.references,
		QuotedText:      m.quotedText,
	}
}

// NewComposerFromDraft creates a composer from an existing draft.
func NewComposerFromDraft(draft config.Draft, accounts []config.Account, hideTips bool) *Composer {
	m := NewComposerWithAccounts(accounts, draft.AccountID, draft.To, draft.Subject, draft.Body, hideTips)
	m.ccInput.SetValue(draft.Cc)
	m.bccInput.SetValue(draft.Bcc)
	m.draftID = draft.ID
	m.attachmentPaths = draft.AttachmentPaths
	m.inReplyTo = draft.InReplyTo
	m.references = draft.References
	m.quotedText = draft.QuotedText
	return m
}
