package tui

import (
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/spellcheck"
	"github.com/google/uuid"
)

// spellcheckReadyMsg is delivered when the background spellcheck loader
// finishes (either downloading the default dictionary or loading an
// already-installed one).
type spellcheckReadyMsg struct {
	checker *spellcheck.Checker
}

var (
	suggestionStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectedSuggestionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	suggestionBoxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("245")).Padding(0, 1)
)

// Styles for the UI
var (
	focusedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	blurredStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	emailRecipientStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	attachmentStyle     = lipgloss.NewStyle().PaddingLeft(4).Foreground(lipgloss.Color("245"))
	smimeToggleStyle    = lipgloss.NewStyle().PaddingLeft(4).Foreground(lipgloss.Color("245"))
	composerErrorStyle  = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("196"))
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

type hideComposerNoticeMsg struct{}

// Composer model holds the state of the email composition UI.
type Composer struct {
	focusIndex       int
	toInput          textinput.Model
	ccInput          textinput.Model
	bccInput         textinput.Model
	fromError        string
	toError          string
	ccError          string
	bccError         string
	subjectInput     textinput.Model
	bodyInput        textarea.Model
	signatureInput   textarea.Model
	attachmentPaths  []string
	attachmentNames  map[string]string
	attachmentCursor int
	encryptSMIME     bool
	width            int
	height           int
	confirmingExit   bool
	showNotice       bool
	noticeText       string
	hideTips         bool

	// Multi-account support
	accounts           []config.Account
	selectedAccountIdx int
	showAccountPicker  bool
	fromInput          textinput.Model // editable From when account is catch-all

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

	// Plugin status text shown in the help bar
	pluginStatus      string
	pluginKeyBindings []PluginKeyBinding

	// Plugin prompt overlay
	showPluginPrompt        bool
	pluginPromptInput       textinput.Model
	pluginPromptPlaceholder string

	// Spellcheck (loaded asynchronously; nil until ready).
	spellChecker            *spellcheck.Checker
	spellSuggestions        []string
	spellSelected           int
	spellShow               bool
	spellWordOnLine         int    // index of the logical line containing the word
	spellWordLineStart      int    // byte offset of the word within its logical line
	spellWordLineEnd        int    // byte offset of the word's end within its logical line
	spellWord               string // the misspelled word (as currently in body)
	spellLastBody           string // last body value we computed suggestions for
	spellLastCursorRow      int
	spellLastCursorCol      int
	disableSpellcheck       bool
	disableSpellSuggestions bool
}

// NewComposer initializes a new composer model.
func NewComposer(from, to, subject, body string, hideTips bool) *Composer {
	m := &Composer{
		draftID:         uuid.New().String(),
		hideTips:        hideTips,
		attachmentNames: make(map[string]string),
	}

	tiStyles := ThemedTextInputStyles()
	taStyles := ThemedTextAreaStyles()

	m.toInput = textinput.New()
	m.toInput.Placeholder = t("composer.to_placeholder")
	m.toInput.SetValue(to)
	m.toInput.Prompt = "> "
	m.toInput.CharLimit = 256
	m.toInput.SetStyles(tiStyles)

	m.ccInput = textinput.New()
	m.ccInput.Placeholder = t("composer.cc_placeholder")
	m.ccInput.Prompt = "> "
	m.ccInput.CharLimit = 256
	m.ccInput.SetStyles(tiStyles)

	m.bccInput = textinput.New()
	m.bccInput.Placeholder = t("composer.bcc_placeholder")
	m.bccInput.Prompt = "> "
	m.bccInput.CharLimit = 256
	m.bccInput.SetStyles(tiStyles)

	m.subjectInput = textinput.New()
	m.subjectInput.Placeholder = t("composer.subject_placeholder")
	m.subjectInput.SetValue(subject)
	m.subjectInput.Prompt = "> "
	m.subjectInput.CharLimit = 256
	m.subjectInput.SetStyles(tiStyles)

	m.bodyInput = textarea.New()
	m.bodyInput.Placeholder = t("composer.body_placeholder")
	m.bodyInput.SetValue(body)
	m.bodyInput.Prompt = "> "
	m.bodyInput.SetHeight(10)
	m.bodyInput.SetStyles(taStyles)

	m.signatureInput = textarea.New()
	m.signatureInput.Placeholder = t("composer.signature_placeholder")
	m.signatureInput.Prompt = "> "
	m.signatureInput.SetHeight(3)
	m.signatureInput.SetStyles(taStyles)
	m.updateSignature()

	m.fromInput = textinput.New()
	m.fromInput.Placeholder = t("composer.from_placeholder")
	m.fromInput.Prompt = "> "
	m.fromInput.CharLimit = 256
	m.fromInput.SetStyles(tiStyles)

	// Start focus on To field (From is selectable but not a text input)
	m.focusIndex = focusTo
	m.toInput.Focus()

	return m
}

func normalizeEmailList(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}

	parts := strings.Split(value, ",")
	addresses := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		addr, err := mail.ParseAddress(part)
		if err != nil || addr.Address == "" {
			return value, false
		}
		addresses = append(addresses, addr.Address)
	}
	if len(addresses) == 0 {
		return "", true
	}
	return strings.Join(addresses, ", "), true
}

func (m *Composer) hasAnyRecipient() bool {
	return strings.TrimSpace(m.toInput.Value()) != "" ||
		strings.TrimSpace(m.ccInput.Value()) != "" ||
		strings.TrimSpace(m.bccInput.Value()) != ""
}

func (m *Composer) showComposerNotice(message string) tea.Cmd {
	m.noticeText = message
	m.showNotice = true
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return hideComposerNoticeMsg{}
	})
}

func (m *Composer) hideComposerNotice() {
	m.showNotice = false
	m.noticeText = ""
}

func (m *Composer) validateFromField() bool { //nolint:unparam
	if !m.isCatchAllAccount() {
		m.fromError = ""
		return true
	}
	value := strings.TrimSpace(m.fromInput.Value())
	addr, err := mail.ParseAddress(value)
	if value == "" || err != nil || addr.Address == "" {
		m.fromError = t("composer.invalid_email")
		return false
	}
	m.fromError = ""
	return true
}

func (m *Composer) validateEmailField(focus int) bool { //nolint:unparam
	var input *textinput.Model
	var setError func(string)
	switch focus {
	case focusTo:
		input = &m.toInput
		setError = func(err string) { m.toError = err }
	case focusCc:
		input = &m.ccInput
		setError = func(err string) { m.ccError = err }
	case focusBcc:
		input = &m.bccInput
		setError = func(err string) { m.bccError = err }
	default:
		return true
	}

	normalized, ok := normalizeEmailList(input.Value())
	if !ok {
		setError(t("composer.invalid_email"))
		return false
	}
	input.SetValue(normalized)
	setError("")
	return true
}

func (m *Composer) canSendEmail() bool {
	m.validateFromField()
	m.validateEmailField(focusTo)
	m.validateEmailField(focusCc)
	m.validateEmailField(focusBcc)
	return m.fromError == "" && m.toError == "" && m.ccError == "" && m.bccError == ""
}

// updateSignature updates the signature input based on the current selected account.
func (m *Composer) updateSignature() {
	if len(m.accounts) > 0 && m.selectedAccountIdx < len(m.accounts) {
		acc := &m.accounts[m.selectedAccountIdx]
		if sig, err := config.LoadSignatureForAccount(acc); err == nil && sig != "" {
			m.signatureInput.SetValue(sig)
		} else if sig, err := config.LoadSignature(); err == nil && sig != "" {
			m.signatureInput.SetValue(sig)
		} else {
			m.signatureInput.SetValue("")
		}
		// Seed the editable From address for catch-all accounts.
		m.fromInput.SetValue(acc.FormatFromHeader())
		return
	}

	if sig, err := config.LoadSignature(); err == nil && sig != "" {
		m.signatureInput.SetValue(sig)
	} else {
		m.signatureInput.SetValue("")
	}
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
	m.updateSignature()

	return m
}

// ResetConfirmation ensures a restored draft isnt stuck in the exit prompt.
func (m *Composer) ResetConfirmation() {
	m.confirmingExit = false
}

// SetFromOverride pre-fills the editable From field (used for catch-all replies).
func (m *Composer) SetFromOverride(addr string) {
	m.fromInput.SetValue(addr)
}

// SetSpellcheckOptions toggles spellcheck features for this composer. Pass
// disableCheck=true to skip dictionary download/highlighting entirely;
// disableSuggestions=true keeps inline underlines but suppresses the popup.
func (m *Composer) SetSpellcheckOptions(disableCheck, disableSuggestions bool) {
	m.disableSpellcheck = disableCheck
	m.disableSpellSuggestions = disableSuggestions
	if disableCheck {
		m.spellChecker = nil
		m.spellShow = false
		m.spellSuggestions = nil
	}
}

func (m *Composer) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if !m.disableSpellcheck {
		cmds = append(cmds, loadSpellcheckCmd())
	}
	return tea.Batch(cmds...)
}

// loadSpellcheckCmd ensures the default dictionary is downloaded and
// loaded into a new Checker. Network errors are swallowed: spellcheck is a
// non-essential overlay, so the composer continues to work normally.
func loadSpellcheckCmd() tea.Cmd {
	return func() tea.Msg {
		lang, err := spellcheck.EnsureDefault()
		if err != nil {
			return spellcheckReadyMsg{checker: nil}
		}
		c := spellcheck.NewChecker()
		if err := c.LoadLang(lang); err != nil {
			return spellcheckReadyMsg{checker: nil}
		}
		return spellcheckReadyMsg{checker: c}
	}
}

// updateSpellSuggestions inspects the body cursor position and refreshes
// the suggestion popup. It only fires when the cursor sits at the end of
// a misspelled word.
func (m *Composer) updateSpellSuggestions() {
	m.spellShow = false
	m.spellSuggestions = nil
	m.spellWord = ""

	if m.disableSpellcheck || m.disableSpellSuggestions {
		return
	}
	if m.spellChecker == nil || !m.spellChecker.Loaded() {
		return
	}
	if m.focusIndex != focusBody {
		return
	}

	value := m.bodyInput.Value()
	row := m.bodyInput.Line()
	col := m.bodyInput.Column()
	lines := strings.Split(value, "\n")
	if row < 0 || row >= len(lines) {
		return
	}
	line := lines[row]
	lineRunes := []rune(line)
	if col > len(lineRunes) {
		col = len(lineRunes)
	}

	// Walk back from cursor while we have letters or internal connectors.
	end := col
	start := col
	for start > 0 {
		r := lineRunes[start-1]
		if isWordContinuation(r) {
			start--
			continue
		}
		break
	}
	// Trim leading connectors so the word starts on a letter.
	for start < end && !isLetter(lineRunes[start]) {
		start++
	}
	// Trim trailing connectors so we don't suggest replacements while the
	// user is still mid-apostrophe.
	for end > start && !isLetter(lineRunes[end-1]) {
		end--
	}
	if end-start < 2 {
		return
	}

	word := string(lineRunes[start:end])
	if !spellcheck.IsCheckable(word) {
		return
	}
	if m.spellChecker.Check(word) {
		return
	}

	suggestions := m.spellChecker.Suggest(word, 5)
	if len(suggestions) == 0 {
		return
	}

	m.spellSuggestions = suggestions
	m.spellSelected = 0
	m.spellShow = true
	m.spellWord = word

	// Byte offsets within the current line, needed by the accept handler.
	m.spellWordLineStart = len(string(lineRunes[:start]))
	m.spellWordLineEnd = len(string(lineRunes[:end]))
	m.spellWordOnLine = row

	// Cache cursor position so a no-op key (e.g. arrow without movement)
	// doesn't redundantly recompute suggestions.
	m.spellLastBody = value
	m.spellLastCursorRow = row
	m.spellLastCursorCol = col
}

// acceptSpellSuggestion replaces the misspelled word currently under the
// cursor with the selected suggestion. It works by sending backspace key
// events to the textarea (so the textarea's own bookkeeping stays in
// sync) and then inserting the replacement text.
func (m *Composer) acceptSpellSuggestion() {
	if !m.spellShow || len(m.spellSuggestions) == 0 {
		return
	}
	if m.spellSelected < 0 || m.spellSelected >= len(m.spellSuggestions) {
		return
	}
	suggestion := m.spellSuggestions[m.spellSelected]

	// Only replace when the cursor is still at the end of the word we
	// recorded — otherwise the user moved and the popup is stale.
	row := m.bodyInput.Line()
	col := m.bodyInput.Column()
	lines := strings.Split(m.bodyInput.Value(), "\n")
	if row != m.spellWordOnLine || row >= len(lines) {
		m.spellShow = false
		m.spellSuggestions = nil
		return
	}
	endRunes := len([]rune(lines[row][:m.spellWordLineEnd]))
	if col != endRunes {
		m.spellShow = false
		m.spellSuggestions = nil
		return
	}

	wordRuneLen := len([]rune(m.spellWord))
	for i := 0; i < wordRuneLen; i++ {
		m.bodyInput, _ = m.bodyInput.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m.bodyInput.InsertString(suggestion)

	m.spellShow = false
	m.spellSuggestions = nil
	m.spellWord = ""
}

func isWordContinuation(r rune) bool {
	return isLetter(r) || r == '\'' || r == '’' || r == '-'
}

func isLetter(r rune) bool {
	if r < 0x80 {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.IsLetter(r)
}

func (m *Composer) getFromAddress() string {
	if len(m.accounts) > 0 && m.selectedAccountIdx < len(m.accounts) {
		return m.accounts[m.selectedAccountIdx].FormatFromHeader()
	}
	return ""
}

func (m *Composer) isCatchAllAccount() bool {
	if len(m.accounts) > 0 && m.selectedAccountIdx < len(m.accounts) {
		return m.accounts[m.selectedAccountIdx].CatchAll
	}
	return false
}

func (m *Composer) getSelectedAccount() *config.Account {
	if len(m.accounts) > 0 && m.selectedAccountIdx < len(m.accounts) {
		return &m.accounts[m.selectedAccountIdx]
	}
	return nil
}

func formatAttachmentName(path string) string {
	name := filepath.Base(path)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, tfs(info.Size()))
}

func (m *Composer) attachmentDisplayName(path string) string {
	if name, ok := m.attachmentNames[path]; ok {
		return name
	}
	return filepath.Base(path)
}

func (m *Composer) clampAttachmentCursor() {
	if len(m.attachmentPaths) == 0 {
		m.attachmentCursor = 0
		return
	}
	if m.attachmentCursor < 0 {
		m.attachmentCursor = 0
	}
	if m.attachmentCursor >= len(m.attachmentPaths) {
		m.attachmentCursor = len(m.attachmentPaths) - 1
	}
}

func (m *Composer) removeSelectedAttachment() {
	if len(m.attachmentPaths) == 0 {
		return
	}

	m.clampAttachmentCursor()
	idx := m.attachmentCursor
	delete(m.attachmentNames, m.attachmentPaths[idx])
	m.attachmentPaths = append(m.attachmentPaths[:idx], m.attachmentPaths[idx+1:]...)
	m.clampAttachmentCursor()
}

func suggestionDisplay(s config.Contact, suggestionWidth int) string {
	display := s.Email
	if len(s.Addresses) > 0 {
		display = fmt.Sprintf("%s (%s)", s.Name, strings.Join(s.Addresses, ", "))
		return truncateSuggestionDisplay(display, suggestionWidth)
	} else if s.Name != "" && s.Name != s.Email {
		display = fmt.Sprintf("%s <%s>", s.Name, s.Email)
	}
	return display
}

func suggestionDisplayWidth(width int) int {
	if width > 12 {
		return width - 6
	}
	return 40
}

func truncateSuggestionDisplay(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 0 {
		return ""
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func (m *Composer) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocyclo
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

	case hideComposerNoticeMsg:
		m.hideComposerNotice()
		return m, nil

	case spellcheckReadyMsg:
		if msg.checker != nil {
			m.spellChecker = msg.checker
			m.updateSpellSuggestions()
		}
		return m, nil

	case FileSelectedMsg:
		// Avoid duplicates and add all selected paths
		for _, newPath := range msg.Paths {
			exists := false
			for _, p := range m.attachmentPaths {
				if p == newPath {
					exists = true
					break
				}
			}
			if !exists {
				m.attachmentPaths = append(m.attachmentPaths, newPath)
				m.attachmentNames[newPath] = formatAttachmentName(newPath)
			}
		}
		m.clampAttachmentCursor()
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
			case keyDown, "ctrl+n":
				if m.selectedSuggestion < len(m.suggestions)-1 {
					m.selectedSuggestion++
				}
				return m, nil
			case "tab", keyEnter:
				// Select the suggestion
				selected := m.suggestions[m.selectedSuggestion]

				var newEmail string
				switch {
				case len(selected.Addresses) > 0:
					// Mailing list: emit just the addresses to maintain valid email formatting
					newEmail = strings.Join(selected.Addresses, ", ")
				case selected.Name != "" && selected.Name != selected.Email:
					newEmail = fmt.Sprintf("%s <%s>", selected.Name, selected.Email)
				default:
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
				m.toError = ""
				m.lastToValue = m.toInput.Value()
				m.showSuggestions = false
				m.suggestions = nil
				return m, nil
			case "esc":
				m.showSuggestions = false
				m.suggestions = nil
				return m, nil
			}
			// For prev-field key, close suggestions and let it fall through to normal handling
			if msg.String() == config.Keybinds.Composer.PrevField {
				m.showSuggestions = false
				m.suggestions = nil
			}
		}

		// Handle plugin prompt overlay
		if m.showPluginPrompt {
			switch msg.String() {
			case keyEnter:
				value := m.pluginPromptInput.Value()
				m.showPluginPrompt = false
				return m, func() tea.Msg { return PluginPromptSubmitMsg{Value: value} }
			case "esc":
				m.showPluginPrompt = false
				return m, func() tea.Msg { return PluginPromptCancelMsg{} }
			default:
				m.pluginPromptInput, cmd = m.pluginPromptInput.Update(msg)
				return m, cmd
			}
		}

		// Handle account picker mode
		if m.showAccountPicker {
			switch msg.String() {
			case "up", "k":
				if m.selectedAccountIdx > 0 {
					m.selectedAccountIdx--
					m.updateSignature()
				}
			case keyDown, "j":
				if m.selectedAccountIdx < len(m.accounts)-1 {
					m.selectedAccountIdx++
					m.updateSignature()
				}
			case keyEnter:
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

		if m.showNotice {
			switch msg.String() {
			case keyEnter, "esc", " ":
				m.hideComposerNotice()
			}
			return m, nil
		}

		// Spellcheck suggestion popup (only while body is focused).
		if m.focusIndex == focusBody && m.spellShow && len(m.spellSuggestions) > 0 {
			sk := config.Keybinds.Composer
			switch msg.String() {
			case sk.SpellPrev:
				if m.spellSelected > 0 {
					m.spellSelected--
				}
				return m, nil
			case sk.SpellNext:
				if m.spellSelected < len(m.spellSuggestions)-1 {
					m.spellSelected++
				}
				return m, nil
			case sk.SpellAccept:
				m.acceptSpellSuggestion()
				return m, nil
			case sk.SpellDismiss:
				m.spellShow = false
				m.spellSuggestions = nil
				return m, nil
			}
		}

		kb := config.Keybinds
		attachmentPathSize := len(m.attachmentPaths)
		if m.focusIndex == focusAttachment && attachmentPathSize > 0 {
			switch msg.String() {
			case "up", kb.Global.NavUp:
				m.attachmentCursor = (m.attachmentCursor - 1 + attachmentPathSize) % attachmentPathSize
				return m, nil
			case keyDown, kb.Global.NavDown:
				m.attachmentCursor = (m.attachmentCursor + 1) % attachmentPathSize
				return m, nil
			}
		}

		switch msg.String() {
		case kb.Global.Quit:
			return m, tea.Quit
		case kb.Composer.ExternalEditor:
			return m, func() tea.Msg { return OpenEditorMsg{} }
		case kb.Global.Cancel:
			m.confirmingExit = true
			return m, nil

		case kb.Composer.NextField, kb.Composer.PrevField:
			previousFocus := m.focusIndex
			if msg.String() == kb.Composer.PrevField {
				m.focusIndex--
			} else {
				m.focusIndex++
			}

			maxFocus := focusSend
			minFocus := focusFrom
			// Skip From field if only one non-catch-all account (nothing to switch or edit)
			if len(m.accounts) <= 1 && !m.isCatchAllAccount() {
				minFocus = focusTo
			}

			if m.focusIndex > maxFocus {
				m.focusIndex = minFocus
			} else if m.focusIndex < minFocus {
				m.focusIndex = maxFocus
			}

			if previousFocus == focusFrom {
				m.validateFromField()
			} else if previousFocus != m.focusIndex {
				m.validateEmailField(previousFocus)
			}

			m.fromInput.Blur()
			m.toInput.Blur()
			m.ccInput.Blur()
			m.bccInput.Blur()
			m.subjectInput.Blur()
			m.bodyInput.Blur()
			m.signatureInput.Blur()
			m.spellShow = false
			m.spellSuggestions = nil

			switch m.focusIndex {
			case focusFrom:
				if m.isCatchAllAccount() {
					cmds = append(cmds, m.fromInput.Focus())
				}
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

		case kb.Composer.Delete:
			if m.focusIndex == focusAttachment && len(m.attachmentPaths) > 0 {
				m.removeSelectedAttachment()
				return m, nil
			}

		case keyEnter, " ":
			switch m.focusIndex {
			case focusFrom:
				if msg.String() == keyEnter && len(m.accounts) > 1 {
					m.showAccountPicker = true
					return m, nil
				}
				if m.isCatchAllAccount() && msg.String() == " " {
					break
				}
				return m, nil
			case focusAttachment:
				if msg.String() == keyEnter {
					return m, func() tea.Msg { return GoToFilePickerMsg{} }
				}
			case focusEncryptSMIME:
				if msg.String() == keyEnter || msg.String() == " " {
					m.encryptSMIME = !m.encryptSMIME
				}
				return m, nil

			case focusSend:
				if msg.String() == keyEnter {
					if !m.canSendEmail() {
						return m, m.showComposerNotice(t("composer.invalid_email_fields"))
					}
					if !m.hasAnyRecipient() {
						return m, m.showComposerNotice(t("composer.recipient_required"))
					}
					acc := m.getSelectedAccount()
					accountID := ""
					if acc != nil {
						accountID = acc.ID
					}
					fromOverride := ""
					if m.isCatchAllAccount() {
						fromOverride = m.fromInput.Value()
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
							FromOverride:    fromOverride,
							QuotedText:      m.quotedText,
							InReplyTo:       m.inReplyTo,
							References:      m.references,
							Signature:       m.signatureInput.Value(),
							SignSMIME:       acc != nil && acc.SMIMESignByDefault,
							EncryptSMIME:    m.encryptSMIME,
							SignPGP:         acc != nil && acc.PGPSignByDefault,
						}
					}
				}
			}
		}
	}

	switch m.focusIndex {
	case focusFrom:
		if m.isCatchAllAccount() {
			previousFromValue := m.fromInput.Value()
			m.fromInput, cmd = m.fromInput.Update(msg)
			cmds = append(cmds, cmd)
			if m.fromInput.Value() != previousFromValue {
				m.fromError = ""
			}
		}
	case focusTo:
		previousToValue := m.toInput.Value()
		m.toInput, cmd = m.toInput.Update(msg)
		cmds = append(cmds, cmd)

		// Check if To field value changed and update suggestions
		currentValue := m.toInput.Value()
		if currentValue != m.lastToValue {
			if currentValue != previousToValue {
				m.toError = ""
			}
			m.lastToValue = currentValue

			// Extract the last comma-separated part for searching
			parts := strings.Split(currentValue, ",")
			lastPart := strings.TrimSpace(parts[len(parts)-1])

			if len(lastPart) >= 2 {
				m.suggestions = config.SearchContactsForAccount(lastPart, m.GetSelectedAccountID())
				m.showSuggestions = len(m.suggestions) > 0
				m.selectedSuggestion = 0
			} else {
				m.showSuggestions = false
				m.suggestions = nil
			}
		}
	case focusCc:
		previousCcValue := m.ccInput.Value()
		m.ccInput, cmd = m.ccInput.Update(msg)
		cmds = append(cmds, cmd)
		if m.ccInput.Value() != previousCcValue {
			m.ccError = ""
		}
	case focusBcc:
		previousBccValue := m.bccInput.Value()
		m.bccInput, cmd = m.bccInput.Update(msg)
		cmds = append(cmds, cmd)
		if m.bccInput.Value() != previousBccValue {
			m.bccError = ""
		}
	case focusSubject:
		m.subjectInput, cmd = m.subjectInput.Update(msg)
		cmds = append(cmds, cmd)
	case focusBody:
		prevBody := m.bodyInput.Value()
		prevRow := m.bodyInput.Line()
		prevCol := m.bodyInput.Column()
		m.bodyInput, cmd = m.bodyInput.Update(msg)
		cmds = append(cmds, cmd)
		// Only recompute suggestions when the body state actually changes.
		// Cursor-blink ticks otherwise reset spellSelected to 0 every blink.
		if m.bodyInput.Value() != prevBody ||
			m.bodyInput.Line() != prevRow ||
			m.bodyInput.Column() != prevCol {
			m.updateSpellSuggestions()
		}
	case focusSignature:
		m.signatureInput, cmd = m.signatureInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Composer) View() tea.View { //nolint:gocyclo
	var composerView strings.Builder
	var button string
	ck := config.Keybinds.Composer

	if m.focusIndex == focusSend {
		button = focusedStyle.Render("[ " + t("composer.send") + " ]")
	} else {
		button = blurredStyle.Render("[ " + t("composer.send") + " ]")
	}

	// From field with account selector
	fromAddr := m.getFromAddress()
	var fromField string
	if m.isCatchAllAccount() { //nolint:gocritic
		fromAddrView := m.fromInput.View()
		if len(m.accounts) > 1 {
			if m.focusIndex == focusFrom {
				fromField = focusedStyle.Render(fmt.Sprintf("> %s ", t("composer.from"))) + fromAddrView + " " + blurredStyle.Render("["+t("composer.enter_to_switch")+"]")
			} else {
				fromField = blurredStyle.Render(fmt.Sprintf("  %s ", t("composer.from"))) + fromAddrView + " " + blurredStyle.Render("["+t("composer.switchable")+"]")
			}
		} else {
			fromField = "  " + t("composer.from") + " " + fromAddrView
		}
		if m.fromError != "" {
			fromField += "\n" + composerErrorStyle.Render(m.fromError)
		}
	} else if len(m.accounts) > 1 {
		if m.focusIndex == focusFrom {
			fromField = focusedStyle.Render(fmt.Sprintf("> %s %s [%s]", t("composer.from"), fromAddr, t("composer.enter_to_switch")))
		} else {
			fromField = blurredStyle.Render(fmt.Sprintf("  %s %s [%s]", t("composer.from"), fromAddr, t("composer.switchable")))
		}
	} else if fromAddr != "" {
		fromField = "  " + t("composer.from") + " " + emailRecipientStyle.Render(fromAddr)
	} else {
		fromField = blurredStyle.Render(fmt.Sprintf("  %s (%s)", t("composer.from"), t("composer.no_account")))
	}

	var attachmentField string
	if len(m.attachmentPaths) == 0 {
		attachmentText := fmt.Sprintf("%s (%s)", t("composer.attachments_none"), t("composer.enter_to_add"))
		if m.focusIndex == focusAttachment {
			attachmentField = focusedStyle.Render(fmt.Sprintf("> %s %s", t("composer.attachments"), attachmentText))
		} else {
			attachmentField = blurredStyle.Render(fmt.Sprintf("  %s %s", t("composer.attachments"), attachmentText))
		}
	} else {
		var b strings.Builder
		headerPrefix := "  "
		headerStyle := blurredStyle
		if m.focusIndex == focusAttachment {
			headerPrefix = "> "
			headerStyle = focusedStyle
		}
		b.WriteString(headerStyle.Render(fmt.Sprintf("%s%s (%d):", headerPrefix, t("composer.attachments"), len(m.attachmentPaths))))
		for i, p := range m.attachmentPaths {
			cursor := "    "
			style := blurredStyle
			if m.focusIndex == focusAttachment && i == m.attachmentCursor {
				cursor = "  > "
				style = focusedStyle
			}
			b.WriteString("\n")
			b.WriteString(style.Render(fmt.Sprintf("%s%s", cursor, m.attachmentDisplayName(p))))
		}
		attachmentField = b.String()
	}

	encToggle := "[ ]"
	if m.encryptSMIME {
		encToggle = "[x]"
	}
	encField := blurredStyle.Render(fmt.Sprintf("  %s %s", t("composer.encrypt_smime"), encToggle))
	if m.focusIndex == focusEncryptSMIME {
		encField = focusedStyle.Render(fmt.Sprintf("> %s %s", t("composer.encrypt_smime"), encToggle))
	}

	// Build To field with suggestions
	toFieldView := m.toInput.View()
	if m.toError != "" {
		toFieldView += "\n" + composerErrorStyle.Render(m.toError)
	}
	if m.showSuggestions && len(m.suggestions) > 0 {
		var suggestionsBuilder strings.Builder
		suggestionWidth := suggestionDisplayWidth(m.width)
		for i, s := range m.suggestions {
			display := suggestionDisplay(s, suggestionWidth)
			if i == m.selectedSuggestion {
				suggestionsBuilder.WriteString(selectedSuggestionStyle.Render("> "+display) + "\n")
			} else {
				suggestionsBuilder.WriteString(suggestionStyle.Render("  "+display) + "\n")
			}
		}
		toFieldView = toFieldView + "\n" + suggestionBoxStyle.Render(strings.TrimSuffix(suggestionsBuilder.String(), "\n"))
	}

	ccFieldView := m.ccInput.View()
	if m.ccError != "" {
		ccFieldView += "\n" + composerErrorStyle.Render(m.ccError)
	}

	bccFieldView := m.bccInput.View()
	if m.bccError != "" {
		bccFieldView += "\n" + composerErrorStyle.Render(m.bccError)
	}

	// Signature field label
	var signatureLabel string
	if m.focusIndex == focusSignature {
		signatureLabel = focusedStyle.Render(t("composer.signature") + ":")
	} else {
		signatureLabel = blurredStyle.Render(t("composer.signature") + ":")
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
		if m.spellShow && len(m.spellSuggestions) > 0 {
			sk := config.Keybinds.Composer
			tip = fmt.Sprintf("Spelling: %s accept • %s/%s navigate • %s dismiss",
				sk.SpellAccept, sk.SpellNext, sk.SpellPrev, sk.SpellDismiss)
		} else {
			tip = "The main content of your email. Markdown and HTML are supported."
		}
	case focusSignature:
		tip = "Your email signature. This will be appended to the end of the email."
	case focusAttachment:
		tip = fmt.Sprintf("Enter: add file • up/down: select attachment • %s: remove selected", ck.Delete)
	case focusEncryptSMIME:
		tip = "Press Space or Enter to toggle S/MIME encryption on or off."
	case focusSend:
		tip = "Press Enter to send the email."
	}

	bodyView := m.bodyInput.View()
	if !m.disableSpellcheck && m.spellChecker != nil && m.spellChecker.Loaded() {
		bodyView = spellcheck.Highlight(bodyView, m.spellChecker, -1)
	}

	composerViewElements := []string{
		t("composer.title"),
		fromField,
		toFieldView,
		ccFieldView,
		bccFieldView,
		m.subjectInput.View(),
		bodyView,
		signatureLabel,
		m.signatureInput.View(),
		attachmentStyle.Render(attachmentField),
	}
	if len(m.attachmentPaths) > 0 {
		composerViewElements = append(composerViewElements, "")
	}
	composerViewElements = append(composerViewElements,
		smimeToggleStyle.Render(encField),
		button,
		"",
	)

	if !m.hideTips && tip != "" {
		composerViewElements = append(composerViewElements, TipStyle.Render("Tip: "+tip))
	}

	mainContent := lipgloss.JoinVertical(lipgloss.Left, composerViewElements...)
	helpText := t("composer.help")
	for _, pk := range m.pluginKeyBindings {
		helpText += " • " + pk.Key + ": " + pk.Description
	}
	if m.pluginStatus != "" {
		helpText += " • " + m.pluginStatus
	}
	helpView := helpStyle.Render(helpText)

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

	// Plugin prompt overlay
	if m.showPluginPrompt {
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Left,
				m.pluginPromptPlaceholder,
				"",
				m.pluginPromptInput.View(),
				"",
				HelpStyle.Render("enter: submit • esc: cancel"),
			),
		)
		return tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog))
	}

	// Account picker overlay
	if m.showAccountPicker {
		var accountList strings.Builder
		accountList.WriteString("Select Account:\n\n")
		for i, acc := range m.accounts {
			display := acc.GetSendAsEmail()
			if acc.Name != "" {
				display = fmt.Sprintf("%s (%s)", acc.Name, acc.GetSendAsEmail())
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
				t("composer.exit_confirm"),
				HelpStyle.Render("\n(y/n)"),
			),
		)
		return tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog))
	}

	if m.showNotice {
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				dangerStyle.Render(m.noticeText),
				HelpStyle.Render("\nenter/esc: close"),
			),
		)
		return tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog))
	}

	out := composerView.String()
	if m.spellShow && len(m.spellSuggestions) > 0 && m.focusIndex == focusBody {
		out = m.overlaySpellPopup(out, composerViewElements)
	}
	return tea.NewView(out)
}

// overlaySpellPopup floats the suggestion box at the body cursor position
// in the rendered composer view. It returns the view unchanged when the
// cursor can't be located.
func (m *Composer) overlaySpellPopup(view string, elementsBeforeBody []string) string {
	// Body is the 7th element (index 6) of composerViewElements: title,
	// from, to, cc, bcc, subject, body, ...
	const bodyIdx = 6
	if bodyIdx > len(elementsBeforeBody) {
		return view
	}
	bodyStartRow := 0
	for i := 0; i < bodyIdx; i++ {
		bodyStartRow += lipgloss.Height(elementsBeforeBody[i])
	}

	li := m.bodyInput.LineInfo()
	const promptWidth = 2 // "> "
	cursorRow := bodyStartRow + li.RowOffset
	cursorCol := li.CharOffset + promptWidth

	popup := m.renderSpellPopupLines()
	if len(popup) == 0 {
		return view
	}

	// Anchor below cursor. If popup would clip the bottom, raise it above
	// the cursor row instead.
	anchorRow := cursorRow + 1
	if m.height > 0 && anchorRow+len(popup) > m.height-1 && cursorRow-len(popup) >= 0 {
		anchorRow = cursorRow - len(popup)
	}
	anchorCol := cursorCol
	popupWidth := lipgloss.Width(popup[0])
	if m.width > 0 && anchorCol+popupWidth > m.width {
		anchorCol = max(0, m.width-popupWidth)
	}

	return overlayBlock(view, popup, anchorRow, anchorCol)
}

// renderSpellPopupLines builds the styled, bordered suggestion box and
// returns its rendered lines. Each row carries an "abc" badge to mirror
// the language-server look familiar from VSCode.
func (m *Composer) renderSpellPopupLines() []string {
	if !m.spellShow || len(m.spellSuggestions) == 0 {
		return nil
	}
	maxWidth := 0
	for _, s := range m.spellSuggestions {
		if w := len(s); w > maxWidth {
			maxWidth = w
		}
	}
	rowWidth := maxWidth + 6 // " abc " badge + word + trailing space

	iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("24")).Foreground(lipgloss.Color("231"))

	var rows []string
	for i, s := range m.spellSuggestions {
		text := " " + iconStyle.Render("abc") + " " + s
		pad := rowWidth - lipgloss.Width(text)
		if pad < 0 {
			pad = 0
		}
		text += strings.Repeat(" ", pad)
		if i == m.spellSelected {
			rows = append(rows, selStyle.Render(text))
		} else {
			rows = append(rows, rowStyle.Render(text))
		}
	}
	box := suggestionBoxStyle.Render(strings.Join(rows, "\n"))
	return strings.Split(box, "\n")
}

// SetAccounts sets the available accounts for sending.
func (m *Composer) SetAccounts(accounts []config.Account) {
	m.accounts = accounts
	if m.selectedAccountIdx >= len(accounts) {
		m.selectedAccountIdx = 0
	}
	m.updateSignature()
}

// SetSelectedAccount sets the selected account by ID.
func (m *Composer) SetSelectedAccount(accountID string) {
	for i, acc := range m.accounts {
		if acc.ID == accountID {
			m.selectedAccountIdx = i
			m.updateSignature()
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

// SetTo updates the To field with new content.
func (m *Composer) SetTo(to string) {
	m.toInput.SetValue(to)
}

// GetCc returns the current Cc field value.
func (m *Composer) GetCc() string {
	return m.ccInput.Value()
}

// SetCc updates the Cc field with new content.
func (m *Composer) SetCc(cc string) {
	m.ccInput.SetValue(cc)
}

// GetBcc returns the current Bcc field value.
func (m *Composer) GetBcc() string {
	return m.bccInput.Value()
}

// SetBcc updates the Bcc field with new content.
func (m *Composer) SetBcc(bcc string) {
	m.bccInput.SetValue(bcc)
}

// GetSubject returns the current Subject field value.
func (m *Composer) GetSubject() string {
	return m.subjectInput.Value()
}

// SetSubject updates the Subject field with new content.
func (m *Composer) SetSubject(subject string) {
	m.subjectInput.SetValue(subject)
}

// GetBody returns the current Body field value.
func (m *Composer) GetBody() string {
	return m.bodyInput.Value()
}

// SetBody updates the Body field with new content.
func (m *Composer) SetBody(body string) {
	m.bodyInput.SetValue(body)
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

// SetPluginStatus sets a persistent status string from plugins, shown in the help bar.
func (m *Composer) SetPluginStatus(status string) {
	m.pluginStatus = status
}

// SetPluginKeyBindings sets the plugin-registered key bindings for display in the help bar.
func (m *Composer) SetPluginKeyBindings(bindings []PluginKeyBinding) {
	m.pluginKeyBindings = bindings
}

// ShowPluginPrompt activates the plugin prompt overlay with the given placeholder text.
func (m *Composer) ShowPluginPrompt(placeholder string) {
	m.pluginPromptPlaceholder = placeholder
	m.pluginPromptInput = textinput.New()
	m.pluginPromptInput.Placeholder = placeholder
	m.pluginPromptInput.Prompt = "> "
	m.pluginPromptInput.CharLimit = 256
	m.pluginPromptInput.Focus()
	m.showPluginPrompt = true
}

// HidePluginPrompt deactivates the plugin prompt overlay.
func (m *Composer) HidePluginPrompt() {
	m.showPluginPrompt = false
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
		FromOverride:    m.fromInput.Value(),
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
	m.attachmentNames = make(map[string]string, len(m.attachmentPaths))
	for _, path := range m.attachmentPaths {
		m.attachmentNames[path] = formatAttachmentName(path)
	}
	m.clampAttachmentCursor()
	if m.isCatchAllAccount() && draft.FromOverride != "" {
		m.fromInput.SetValue(draft.FromOverride)
	}
	m.inReplyTo = draft.InReplyTo
	m.references = draft.References
	m.quotedText = draft.QuotedText
	return m
}
