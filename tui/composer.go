package tui

import (
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	overlay "github.com/floatpane/bubble-overlay"
	shortcode "github.com/floatpane/go-emoji-shortcode"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/pgp"
	"github.com/floatpane/matcha/spellcheck"
	"github.com/google/uuid"
)

// spellcheckReadyMsg is delivered when the background spellcheck loader
// finishes (either downloading the default dictionary or loading an
// already-installed one).
type spellcheckReadyMsg struct {
	checker *spellcheck.Checker
}

// ToggleCcBccMsg is sent when the user toggles CC/BCC field visibility.
type ToggleCcBccMsg struct{}

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
	focusSignSMIME
	focusSignPGP
	focusEncryptPGP
	focusSend
)

// keySpaceBar is how Bubble Tea v2 reports the space bar in KeyPressMsg.String().
const keySpaceBar = "space"

// composerHit describes what a mouse click on part of one rendered row
// targets. Hits are recorded while rendering, so hit-testing always matches
// the frame that is actually on screen.
type composerHit struct {
	minX, maxX int // clickable column span [minX, maxX)
	focus      int // focus target, or -1 when the span is inert
	attachment int // attachment index, or -1
	suggestion int // contact suggestion index, or -1
	account    int // account picker index, or -1
}

// composerBlock is one rendered piece of the composer form together with the
// focus target its rows map to.
type composerBlock struct {
	text       string
	focus      int // -1 when the rows are inert
	attachment int // attachment index for attachment rows, -1 otherwise
}

func composerInert(text string) composerBlock {
	return composerBlock{text: text, focus: -1, attachment: -1}
}

func composerField(text string, focus int) composerBlock {
	return composerBlock{text: text, focus: focus, attachment: -1}
}

// composerFrame accumulates rendered rows together with their mouse targets.
type composerFrame struct {
	lines []string
	hits  [][]composerHit
}

func (f *composerFrame) add(b composerBlock) {
	for _, line := range strings.Split(b.text, "\n") {
		f.lines = append(f.lines, line)
		f.hits = append(f.hits, []composerHit{{
			maxX: lipgloss.Width(line), focus: b.focus, attachment: b.attachment,
			suggestion: -1, account: -1,
		}})
	}
}

// composerFit tracks which optional rows of the form are shown and how tall
// the body and signature editors are while the layout is squeezed to fit.
type composerFit struct {
	bodyH, sigH                            int
	showSig, showTip, attachGap, buttonGap bool
	keepSig                                bool // the signature is focused, so it must stay visible
}

// rows returns the total rendered height for the given number of rows that
// are always shown.
func (f composerFit) rows(fixed int) int {
	n := fixed + f.bodyH
	for _, on := range []bool{f.showTip, f.attachGap, f.buttonGap} {
		if on {
			n++
		}
	}
	if f.showSig {
		n += 1 + f.sigH // label + editor
	}
	return n
}

// shrink frees one row, returning false when nothing is left to give up.
func (f *composerFit) shrink() bool {
	switch {
	case f.bodyH > 3:
		f.bodyH--
	case f.showSig && f.sigH > 1:
		f.sigH--
	case f.showTip:
		f.showTip = false
	case f.buttonGap:
		f.buttonGap = false
	case f.attachGap:
		f.attachGap = false
	case f.showSig && !f.keepSig:
		f.showSig = false
	case f.bodyH > 1:
		f.bodyH--
	default:
		return false
	}
	return true
}

// splitComposerHeights returns the preferred body and signature heights for
// the rows left over by the default layout.
func splitComposerHeights(avail int) (bodyH, sigH int) {
	return max(avail*55/100, 3), max(avail*15/100, 2)
}

// truncateLines clips every line of s to at most w cells (no-op when w <= 0).
func truncateLines(s string, w int) string {
	if w <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = ansi.Truncate(l, w, "…")
		}
	}
	return strings.Join(lines, "\n")
}

// clampRendered guarantees a rendered view never exceeds w columns or h rows
// (a non-positive limit is ignored).
func clampRendered(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if h > 0 && len(lines) > h {
		lines = lines[:h]
	}
	if w > 0 {
		for i, l := range lines {
			if lipgloss.Width(l) > w {
				lines[i] = ansi.Truncate(l, w, "")
			}
		}
	}
	return strings.Join(lines, "\n")
}

// centerInArea centers block in a w×h area without ever growing the result
// beyond it, and returns the block's top-left offset within the area.
func centerInArea(block string, w, h int) (view string, top, left int) {
	if w <= 0 || h <= 0 {
		return block, 0, 0
	}
	lines := strings.Split(block, "\n")
	bw := 0
	for _, l := range lines {
		bw = max(bw, lipgloss.Width(l))
	}
	top = max(0, (h-len(lines))/2)
	left = max(0, (w-bw)/2)
	pad := strings.Repeat(" ", left)
	out := make([]string, 0, h)
	for range top {
		out = append(out, "")
	}
	for _, l := range lines {
		out = append(out, pad+l)
	}
	for len(out) < h {
		out = append(out, "")
	}
	return clampRendered(strings.Join(out, "\n"), w, h), top, left
}

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
	signSMIME        bool
	signPGP          bool
	encryptPGP       bool
	width            int
	height           int
	confirmingExit   bool
	hideTips         bool

	// Multi-account support
	accounts            []config.Account
	selectedAccountIdx  int
	showAccountPicker   bool
	accountPickerCursor int             // highlighted picker row; applied on enter/click, dropped on esc
	fromInput           textinput.Model // editable From when account is catch-all

	// Mouse hit map of the last rendered frame (one entry per row, relative to
	// the composer's top-left corner) and the row the body editor starts on.
	rowHits [][]composerHit
	bodyRow int

	// Contact suggestions
	suggestions        []config.Contact
	selectedSuggestion int
	showSuggestions    bool
	lastToValue        string

	// Draft persistence
	draftID string

	// CC/BCC visibility
	showCcBcc bool

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

	// Emoji shortcode suggestions (e.g. :smile: -> 😄).
	emojiSuggestions    []string
	emojiSelected       int
	emojiShow           bool
	emojiShortcode      string
	emojiShortcodeStart int // byte offset of the leading ':' in the current line
	emojiShortcodeEnd   int // byte offset of the end of the shortcode in the current line
	emojiShortcodeLine  int // index of the logical line containing the shortcode

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

	// WKD key download confirmation
	wkdConfirmRecipients  []string // recipients missing local keys, awaiting confirmation
	enablePGPAfterInstall bool     // turn PGP encryption on once the confirmed keys are installed

	// PGP keys discovered for recipients (local or WKD). Used to decide when to
	// show the PGP encryption toggle.
	pgpKeysAvailable map[string]bool
}

// NewComposer initializes a new composer model.
func NewComposer(from, to, subject, body string, hideTips bool) *Composer {
	m := &Composer{
		draftID:          uuid.New().String(),
		hideTips:         hideTips,
		attachmentNames:  make(map[string]string),
		pgpKeysAvailable: make(map[string]bool),
		showCcBcc:        true, // Default to showing CC/BCC for backward compatibility
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

// updatePGPDefaults seeds signPGP from the selected account's PGPSignByDefault.
func (m *Composer) updatePGPDefaults() {
	if acc := m.getSelectedAccount(); acc != nil {
		m.signPGP = acc.PGPSignByDefault
	}
}

func (m *Composer) updateSMIMEDefaults() {
	if acc := m.getSelectedAccount(); acc != nil {
		m.signSMIME = acc.SMIMESignByDefault
	}
}

func (m *Composer) signSMIMEEnabled() bool {
	acc := m.getSelectedAccount()
	if !pgp.HasSMIMESetup(acc) {
		return false
	}
	return m.signSMIME
}

func (m *Composer) pgpRecipients() []string {
	var recipients []string
	for _, s := range []string{m.toInput.Value(), m.ccInput.Value(), m.bccInput.Value()} {
		for _, r := range strings.Split(s, ",") {
			if email := strings.TrimSpace(r); email != "" {
				recipients = append(recipients, email)
			}
		}
	}
	return recipients
}

func (m *Composer) hasPGPKeyAvailable() bool {
	for _, email := range m.pgpRecipients() {
		if m.pgpKeysAvailable[email] {
			return true
		}
	}
	return false
}

func (m *Composer) missingRecipientKeys() []string {
	var missing []string
	for _, email := range m.pgpRecipients() {
		if !m.pgpKeysAvailable[email] {
			missing = append(missing, email)
		}
	}
	return missing
}

func (m *Composer) hasSMIMECertForAnyRecipient() bool {
	acc := m.getSelectedAccount()
	if !pgp.HasSMIMESetup(acc) {
		return false
	}
	for _, email := range m.pgpRecipients() {
		if email != "" && pgp.HasSMIMECertForRecipient(email, acc) {
			return true
		}
	}
	return false
}

func (m *Composer) refreshPGPKeyAvailability() tea.Cmd {
	var cmds []tea.Cmd
	for _, email := range m.pgpRecipients() {
		if !m.pgpKeysAvailable[email] {
			cmds = append(cmds, m.lookupPGPKeyCmd(email))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Composer) lookupPGPKeyCmd(email string) tea.Cmd {
	return func() tea.Msg {
		found, err := pgp.HasLocalKeyForRecipient(email)
		if err == nil && found {
			return pgpKeyFoundMsg{email: email, source: "local"}
		}
		entity, err := pgp.LookupWKD(email)
		if err != nil {
			return nil
		}
		return pgpKeyFoundMsg{email: email, source: "wkd", entity: entity}
	}
}

type pgpKeyFoundMsg struct {
	email  string
	source string
	entity interface{}
}

// pgpKeyInstalledMsg reports the outcome of a confirmed WKD key download.
type pgpKeyInstalledMsg struct {
	email  string
	ok     bool
	notice tea.Msg // notification to surface to the user
}

func (m *Composer) installPGPKeyCmd(email string) tea.Cmd {
	return func() tea.Msg {
		cfgDir, err := config.GetConfigDir()
		if err != nil {
			return pgpKeyInstalledMsg{email: email, notice: NotifyMsg{Message: "PGP: could not determine config directory"}}
		}
		pgpDir := cfgDir + "/pgp"
		entity, err := pgp.LookupWKD(email)
		if err != nil {
			return pgpKeyInstalledMsg{email: email, notice: NotifyMsg{Message: fmt.Sprintf("PGP: could not download key for %s", email)}}
		}
		if err := pgp.CacheWKDKey(pgpDir, email, entity); err != nil {
			return pgpKeyInstalledMsg{email: email, notice: NotifyMsg{Message: fmt.Sprintf("PGP: could not save key for %s", email)}}
		}
		return pgpKeyInstalledMsg{email: email, ok: true, notice: InfoNotifyMsg{Message: fmt.Sprintf("PGP key downloaded for %s", email), Duration: 2}}
	}
}

// visibleCryptoToggles lists the crypto toggles shown for the current
// account and recipients, in display order. An enabled encryption toggle stays
// visible even when it no longer applies (e.g. the recipient with a key was
// removed), so the user can always see it and turn it off.
func (m *Composer) visibleCryptoToggles() []int {
	selAcc := m.getSelectedAccount()
	hasSMIME := pgp.HasSMIMESetup(selAcc)
	hasPGP := pgp.HasPGPSetup(selAcc)

	var toggles []int
	if m.encryptSMIME || (hasSMIME && m.hasSMIMECertForAnyRecipient()) {
		toggles = append(toggles, focusEncryptSMIME)
	}
	if hasSMIME {
		toggles = append(toggles, focusSignSMIME)
	}
	if hasPGP {
		toggles = append(toggles, focusSignPGP)
	}
	if m.encryptPGP || (hasPGP && m.hasPGPKeyAvailable()) {
		toggles = append(toggles, focusEncryptPGP)
	}
	return toggles
}

// ccBccVisible reports whether the Cc/Bcc rows are shown. They are always
// shown when they hold recipients, so nobody is ever mailed invisibly.
func (m *Composer) ccBccVisible() bool {
	return m.showCcBcc ||
		strings.TrimSpace(m.ccInput.Value()) != "" ||
		strings.TrimSpace(m.bccInput.Value()) != ""
}

func (m *Composer) orderedFocusIndices() []int {
	minFocus := focusFrom
	// Skip From field if only one non-catch-all account (nothing to switch or edit)
	if len(m.accounts) <= 1 && !m.isCatchAllAccount() {
		minFocus = focusTo
	}

	indices := make([]int, 0, 8+len(m.visibleCryptoToggles())+1)
	indices = append(indices,
		focusFrom,
		focusTo,
	)
	if m.ccBccVisible() {
		indices = append(indices, focusCc, focusBcc)
	}
	indices = append(indices,
		focusSubject,
		focusBody,
		focusSignature,
		focusAttachment,
	)
	indices = append(indices, m.visibleCryptoToggles()...)
	indices = append(indices, focusSend)

	result := make([]int, 0, len(indices))
	for _, idx := range indices {
		if idx >= minFocus {
			result = append(result, idx)
		}
	}
	return result
}

func (m *Composer) advanceFocus(focusList []int) {
	m.cycleFocus(focusList, 1)
}

func (m *Composer) retreatFocus(focusList []int) {
	m.cycleFocus(focusList, -1)
}

func (m *Composer) cycleFocus(focusList []int, delta int) {
	if len(focusList) == 0 {
		return
	}
	pos := -1
	for i, f := range focusList {
		if f == m.focusIndex {
			pos = i
			break
		}
	}
	if pos == -1 {
		if delta > 0 {
			pos = -1
		} else {
			pos = len(focusList)
		}
	}
	pos += delta
	pos = ((pos % len(focusList)) + len(focusList)) % len(focusList)
	m.focusIndex = focusList[pos]
}

// NewComposerWithAccounts initializes a composer with multiple account support.
func NewComposerWithAccounts(accounts []config.Account, selectedAccountID string, to, subject, body string, hideTips bool) *Composer {
	m := NewComposer("", to, subject, body, hideTips)
	m.accounts = accounts
	m.showCcBcc = true // Will be updated by caller if needed

	// Find the selected account index
	for i, acc := range accounts {
		if acc.ID == selectedAccountID {
			m.selectedAccountIdx = i
			break
		}
	}
	m.updateSignature()
	m.updatePGPDefaults()
	m.updateSMIMEDefaults()

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
	// If the word is preceded by a colon, the user is typing an emoji shortcode;
	// skip spellcheck so the emoji picker can handle it.
	if start > 0 && lineRunes[start-1] == ':' {
		return
	}
	// Trim leading connectors so the word starts on a letter.
	for start < end && !isLetter(lineRunes[start]) {
		start++
	}
	// Don't suggest replacements while the user is still mid-apostrophe or
	// mid-hyphen: the word isn't finished, and accepting requires the cursor
	// to sit right at the end of the word.
	if end > start && !isLetter(lineRunes[end-1]) {
		return
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

// updateEmojiSuggestions inspects the body cursor position and refreshes the
// emoji shortcode suggestion popup. It triggers when the user has typed a ':'
// followed by at least two word characters before the cursor on the same line.
func (m *Composer) updateEmojiSuggestions() {
	m.emojiShow = false
	m.emojiSuggestions = nil
	m.emojiShortcode = ""

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

	// Walk back from cursor while we have valid shortcode characters.
	end := col
	start := col
	for start > 0 {
		r := lineRunes[start-1]
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			start--
			continue
		}
		break
	}
	if start == 0 || start > len(lineRunes) || lineRunes[start-1] != ':' {
		return
	}
	if end-start < 2 {
		return
	}
	code := string(lineRunes[start:end])
	if code == "" {
		return
	}

	matches := shortcode.Suggest(code)
	if len(matches) == 0 {
		return
	}

	// Render each suggestion as "shortcode :emoji".
	suggestions := make([]string, 0, len(matches))
	for _, match := range matches {
		suggestions = append(suggestions, match.Code+" "+match.Emoji)
	}
	m.emojiSuggestions = suggestions
	m.emojiSelected = 0
	m.emojiShow = true
	m.emojiShortcode = code

	m.emojiShortcodeLine = row
	m.emojiShortcodeStart = len(string(lineRunes[:start-1])) // start of ':'
	m.emojiShortcodeEnd = len(string(lineRunes[:end]))
}

// acceptEmojiSuggestion replaces the typed shortcode (including the leading
// colon) with the selected emoji character.
func (m *Composer) acceptEmojiSuggestion() {
	if !m.emojiShow || len(m.emojiSuggestions) == 0 {
		return
	}
	if m.emojiSelected < 0 || m.emojiSelected >= len(m.emojiSuggestions) {
		return
	}
	// Only accept if the cursor is still right after the typed shortcode.
	row := m.bodyInput.Line()
	col := m.bodyInput.Column()
	lines := strings.Split(m.bodyInput.Value(), "\n")
	// The offsets were recorded when the popup opened; if the body changed
	// since (e.g. via the external editor) they may no longer fit the line.
	if row != m.emojiShortcodeLine || row >= len(lines) || m.emojiShortcodeEnd > len(lines[row]) {
		m.emojiShow = false
		m.emojiSuggestions = nil
		return
	}
	endRunes := len([]rune(lines[row][:m.emojiShortcodeEnd]))
	if col != endRunes {
		m.emojiShow = false
		m.emojiSuggestions = nil
		return
	}

	selected := m.emojiSuggestions[m.emojiSelected]
	parts := strings.SplitN(selected, " ", 2)
	if len(parts) != 2 {
		return
	}
	emoji := strings.TrimSpace(parts[1])
	if emoji == "" {
		return
	}

	// Delete the colon plus the typed shortcode characters.
	codeRuneLen := len([]rune(m.emojiShortcode)) + 1
	for i := 0; i < codeRuneLen; i++ {
		m.bodyInput, _ = m.bodyInput.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m.bodyInput.InsertString(emoji)

	m.emojiShow = false
	m.emojiSuggestions = nil
	m.emojiShortcode = ""
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
	// The offsets were recorded when the popup opened; if the body changed
	// since (e.g. via the external editor) they may no longer fit the line.
	if row != m.spellWordOnLine || row >= len(lines) || m.spellWordLineEnd > len(lines[row]) {
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

// suggestionDisplayWidth is the text width available to a contact suggestion
// inside the suggestion box (border, padding and "> " marker take 6 cells).
func suggestionDisplayWidth(width int) int {
	if width <= 0 {
		return 40
	}
	return max(width-6, 1)
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

// setFocusTo blurs all inputs, sets focusIndex to the given constant, and
// focuses the corresponding input widget.
func (m *Composer) setFocusTo(focus int) tea.Cmd {
	m.fromInput.Blur()
	m.toInput.Blur()
	m.ccInput.Blur()
	m.bccInput.Blur()
	m.subjectInput.Blur()
	m.bodyInput.Blur()
	m.signatureInput.Blur()
	m.spellShow = false
	m.spellSuggestions = nil
	if focus != focusTo {
		// Contact suggestions belong to the To field; leaving it must not
		// leave them armed (Enter would otherwise rewrite To).
		m.showSuggestions = false
		m.suggestions = nil
	}
	m.focusIndex = focus
	switch focus {
	case focusFrom:
		if m.isCatchAllAccount() {
			return m.fromInput.Focus()
		}
	case focusTo:
		return m.toInput.Focus()
	case focusCc:
		return m.ccInput.Focus()
	case focusBcc:
		return m.bccInput.Focus()
	case focusSubject:
		return m.subjectInput.Focus()
	case focusBody:
		return m.bodyInput.Focus()
	case focusSignature:
		return m.signatureInput.Focus()
	}
	return nil
}

func (m *Composer) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocyclo
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case ToggleCcBccMsg:
		return m, m.toggleCcBcc()

	case tea.MouseClickMsg:
		return m, m.handleMouseClick(msg)

	case tea.MouseWheelMsg:
		m.handleMouseWheel(msg)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Never hand the inputs a negative width: bubbles' textinput panics
		// rendering its placeholder on tiny terminals otherwise.
		inputWidth := max(msg.Width-6, 1)
		m.toInput.SetWidth(inputWidth)
		m.ccInput.SetWidth(inputWidth)
		m.bccInput.SetWidth(inputWidth)
		m.subjectInput.SetWidth(inputWidth)
		m.bodyInput.SetWidth(inputWidth)
		m.signatureInput.SetWidth(inputWidth)
		// Size the body and signature editors to the rows the rest of the
		// form leaves free.
		_ = m.renderForm()
		return m, nil

	case spellcheckReadyMsg:
		if msg.checker != nil {
			m.spellChecker = msg.checker
			m.updateSpellSuggestions()
		}
		return m, nil

	case pgpKeyFoundMsg:
		if msg.email != "" {
			m.pgpKeysAvailable[msg.email] = true
		}
		if msg.source == "wkd" {
			return m, func() tea.Msg {
				return InfoNotifyMsg{Message: fmt.Sprintf("PGP key found for %s", msg.email), Duration: 2}
			}
		}
		return m, nil

	case pgpKeyInstalledMsg:
		return m, m.handleKeyInstalled(msg)

	case FileSelectedMsg:
		m.addAttachments(msg.Paths)
		return m, nil

	case tea.KeyPressMsg:
		if handled, cmd := m.handleKeyPress(msg); handled {
			return m, cmd
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

			if pgp.HasPGPSetup(m.getSelectedAccount()) && currentValue != previousToValue {
				if cmd := m.refreshPGPKeyAvailability(); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
	case focusCc:
		previousCcValue := m.ccInput.Value()
		m.ccInput, cmd = m.ccInput.Update(msg)
		cmds = append(cmds, cmd)
		if m.ccInput.Value() != previousCcValue {
			m.ccError = ""
			if pgp.HasPGPSetup(m.getSelectedAccount()) {
				if cmd := m.refreshPGPKeyAvailability(); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
	case focusBcc:
		previousBccValue := m.bccInput.Value()
		m.bccInput, cmd = m.bccInput.Update(msg)
		cmds = append(cmds, cmd)
		if m.bccInput.Value() != previousBccValue {
			m.bccError = ""
			if pgp.HasPGPSetup(m.getSelectedAccount()) {
				if cmd := m.refreshPGPKeyAvailability(); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
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
			m.updateEmojiSuggestions()
		}
	case focusSignature:
		m.signatureInput, cmd = m.signatureInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// addAttachments appends newly picked files, skipping ones already attached.
func (m *Composer) addAttachments(paths []string) {
	for _, newPath := range paths {
		if slices.Contains(m.attachmentPaths, newPath) {
			continue
		}
		m.attachmentPaths = append(m.attachmentPaths, newPath)
		m.attachmentNames[newPath] = formatAttachmentName(newPath)
	}
	m.clampAttachmentCursor()
}

// handleKeyInstalled records a key downloaded after the WKD confirmation and
// turns encryption on once every confirmed recipient has a key.
func (m *Composer) handleKeyInstalled(msg pgpKeyInstalledMsg) tea.Cmd {
	if msg.ok {
		m.pgpKeysAvailable[msg.email] = true
	} else {
		m.enablePGPAfterInstall = false
	}
	if m.enablePGPAfterInstall && len(m.missingRecipientKeys()) == 0 {
		m.encryptPGP = true
		m.enablePGPAfterInstall = false
	}
	if msg.notice == nil {
		return nil
	}
	notice := msg.notice
	return func() tea.Msg { return notice }
}

// handleKeyPress processes keys for dialogs, popups and form navigation. It
// reports false when the key should go to the focused input instead.
func (m *Composer) handleKeyPress(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if handled, cmd := m.handleDialogKey(msg); handled {
		return true, cmd
	}
	if m.handlePopupKey(msg) {
		return true, nil
	}
	return m.handleFormKey(msg)
}

// handleDialogKey routes keys to whichever modal dialog is open.
func (m *Composer) handleDialogKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch {
	case m.showPluginPrompt:
		switch msg.String() {
		case keyEnter:
			value := m.pluginPromptInput.Value()
			m.showPluginPrompt = false
			return true, func() tea.Msg { return PluginPromptSubmitMsg{Value: value} }
		case "esc":
			m.showPluginPrompt = false
			return true, func() tea.Msg { return PluginPromptCancelMsg{} }
		}
		var cmd tea.Cmd
		m.pluginPromptInput, cmd = m.pluginPromptInput.Update(msg)
		return true, cmd

	case m.showAccountPicker:
		switch msg.String() {
		case "up", "k":
			if m.accountPickerCursor > 0 {
				m.accountPickerCursor--
			}
		case keyDown, "j":
			if m.accountPickerCursor < len(m.accounts)-1 {
				m.accountPickerCursor++
			}
		case keyEnter:
			m.selectAccount(m.accountPickerCursor)
			m.showAccountPicker = false
		case "esc":
			// Cancel: the selection is only applied on enter.
			m.showAccountPicker = false
		}
		return true, nil

	case m.confirmingExit:
		switch msg.String() {
		case "y", "Y":
			return true, func() tea.Msg { return DiscardDraftMsg{ComposerState: m} }
		case "n", "N", "esc":
			m.confirmingExit = false
		}
		return true, nil

	case len(m.wkdConfirmRecipients) > 0:
		switch msg.String() {
		case "y", "Y":
			recipients := m.wkdConfirmRecipients
			m.wkdConfirmRecipients = nil
			m.enablePGPAfterInstall = true
			cmds := make([]tea.Cmd, 0, len(recipients))
			for _, r := range recipients {
				cmds = append(cmds, m.installPGPKeyCmd(r))
			}
			return true, tea.Batch(cmds...)
		case "n", "N", "esc":
			m.wkdConfirmRecipients = nil
		}
		return true, nil
	}
	return false, nil
}

// handlePopupKey handles the contact, spelling and emoji suggestion popups.
// Each popup only reacts while the field it belongs to has focus.
func (m *Composer) handlePopupKey(msg tea.KeyPressMsg) bool {
	sk := config.Keybinds.Composer
	if m.focusIndex == focusTo && m.showSuggestions && len(m.suggestions) > 0 {
		switch msg.String() {
		case "up", "ctrl+p":
			if m.selectedSuggestion > 0 {
				m.selectedSuggestion--
			}
			return true
		case keyDown, "ctrl+n":
			if m.selectedSuggestion < len(m.suggestions)-1 {
				m.selectedSuggestion++
			}
			return true
		case "tab", keyEnter:
			m.acceptContactSuggestion(m.selectedSuggestion)
			return true
		case "esc":
			m.showSuggestions = false
			m.suggestions = nil
			return true
		}
		// The prev-field key closes the suggestions and then moves focus.
		if msg.String() == sk.PrevField {
			m.showSuggestions = false
			m.suggestions = nil
		}
	}

	if m.focusIndex == focusBody && m.spellShow && len(m.spellSuggestions) > 0 {
		switch msg.String() {
		case sk.SpellPrev:
			if m.spellSelected > 0 {
				m.spellSelected--
			}
			return true
		case sk.SpellNext:
			if m.spellSelected < len(m.spellSuggestions)-1 {
				m.spellSelected++
			}
			return true
		case sk.SpellAccept:
			m.acceptSpellSuggestion()
			return true
		case sk.SpellDismiss, "esc":
			m.spellShow = false
			m.spellSuggestions = nil
			return true
		}
	}

	if m.focusIndex == focusBody && m.emojiShow && len(m.emojiSuggestions) > 0 {
		switch msg.String() {
		case sk.SpellPrev:
			if m.emojiSelected > 0 {
				m.emojiSelected--
			}
			return true
		case sk.SpellNext:
			if m.emojiSelected < len(m.emojiSuggestions)-1 {
				m.emojiSelected++
			}
			return true
		case sk.SpellAccept, "tab":
			m.acceptEmojiSuggestion()
			return true
		case sk.SpellDismiss, "esc":
			m.emojiShow = false
			m.emojiSuggestions = nil
			return true
		}
	}
	return false
}

// handleFormKey handles navigation and activation keys of the form itself.
func (m *Composer) handleFormKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	kb := config.Keybinds
	if n := len(m.attachmentPaths); m.focusIndex == focusAttachment && n > 0 {
		switch msg.String() {
		case "up", kb.Global.NavUp:
			m.attachmentCursor = (m.attachmentCursor - 1 + n) % n
			return true, nil
		case keyDown, kb.Global.NavDown:
			m.attachmentCursor = (m.attachmentCursor + 1) % n
			return true, nil
		}
	}

	switch msg.String() {
	case kb.Global.Quit:
		return true, tea.Quit
	case kb.Composer.ExternalEditor:
		return true, func() tea.Msg { return OpenEditorMsg{} }
	case kb.Global.Cancel:
		m.confirmingExit = true
		return true, nil
	case kb.Composer.NextField, kb.Composer.PrevField:
		return true, m.cycleFieldFocus(msg.String() == kb.Composer.PrevField)
	case kb.Composer.Delete:
		if m.focusIndex == focusAttachment && len(m.attachmentPaths) > 0 {
			m.removeSelectedAttachment()
			return true, nil
		}
	case keyEnter, keySpaceBar:
		return m.activateFocused(msg.String() == keyEnter)
	}
	return false, nil
}

// cycleFieldFocus moves focus to the next (or previous) field, validating the
// address field being left.
func (m *Composer) cycleFieldFocus(backwards bool) tea.Cmd {
	previousFocus := m.focusIndex
	focusList := m.orderedFocusIndices()
	if backwards {
		m.retreatFocus(focusList)
	} else {
		m.advanceFocus(focusList)
	}

	if previousFocus == focusFrom {
		m.validateFromField()
	} else if previousFocus != m.focusIndex {
		m.validateEmailField(previousFocus)
	}

	cmd := m.setFocusTo(m.focusIndex)
	if m.focusIndex == focusBody {
		m.updateSpellSuggestions()
		m.updateEmojiSuggestions()
	}
	return cmd
}

// activateFocused handles enter/space on the focused element. It reports
// false when the key should reach the focused input instead (e.g. a space
// typed into a text field).
func (m *Composer) activateFocused(isEnter bool) (bool, tea.Cmd) {
	switch m.focusIndex {
	case focusFrom:
		if isEnter && len(m.accounts) > 1 {
			m.accountPickerCursor = m.selectedAccountIdx
			m.showAccountPicker = true
			return true, nil
		}
		// A space belongs to the editable From field of catch-all accounts.
		return isEnter || !m.isCatchAllAccount(), nil
	case focusAttachment:
		if isEnter {
			return true, func() tea.Msg { return GoToFilePickerMsg{} }
		}
	case focusEncryptSMIME, focusSignSMIME, focusSignPGP, focusEncryptPGP:
		m.toggleCrypto(m.focusIndex)
		return true, nil
	case focusSend:
		if isEnter {
			return true, m.sendCmd()
		}
	}
	return false, nil
}

// toggleCrypto flips one of the S/MIME / PGP toggles. Turning PGP encryption
// on first asks to fetch any recipient keys that are missing.
func (m *Composer) toggleCrypto(focus int) {
	switch focus {
	case focusEncryptSMIME:
		m.encryptSMIME = !m.encryptSMIME
	case focusSignSMIME:
		m.signSMIME = !m.signSMIME
	case focusSignPGP:
		m.signPGP = !m.signPGP
	case focusEncryptPGP:
		if !m.encryptPGP {
			if missing := m.missingRecipientKeys(); len(missing) > 0 {
				m.wkdConfirmRecipients = missing
				return
			}
		}
		m.encryptPGP = !m.encryptPGP
	}
}

// sendCmd validates the form and returns the command that sends it.
func (m *Composer) sendCmd() tea.Cmd {
	if !m.canSendEmail() {
		msg := t("composer.invalid_email_fields")
		return func() tea.Msg { return NotifyMsg{Message: msg} }
	}
	if !m.hasAnyRecipient() {
		msg := t("composer.recipient_required")
		return func() tea.Msg { return NotifyMsg{Message: msg} }
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
	// Like S/MIME signing, PGP signing only applies when the account has a key.
	signPGP := m.signPGP && pgp.HasPGPSetup(acc)
	return func() tea.Msg {
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
			SignSMIME:       m.signSMIMEEnabled(),
			EncryptSMIME:    m.encryptSMIME,
			SignPGP:         signPGP,
			EncryptPGP:      m.encryptPGP,
		}
	}
}

// acceptContactSuggestion replaces the address being typed in To with the
// suggestion at index i.
func (m *Composer) acceptContactSuggestion(i int) {
	if i < 0 || i >= len(m.suggestions) {
		return
	}
	selected := m.suggestions[i]

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
	if len(parts) == 1 {
		parts[0] = newEmail
	} else {
		parts[len(parts)-1] = " " + newEmail
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
}

// selectAccount switches the sending account and reloads its defaults.
func (m *Composer) selectAccount(i int) {
	if i < 0 || i >= len(m.accounts) || i == m.selectedAccountIdx {
		return
	}
	m.selectedAccountIdx = i
	m.updateSignature()
	m.updatePGPDefaults()
	m.updateSMIMEDefaults()
}

// dialogOpen reports whether a modal dialog currently replaces the form.
func (m *Composer) dialogOpen() bool {
	return m.showPluginPrompt || m.showAccountPicker || m.confirmingExit || len(m.wkdConfirmRecipients) > 0
}

// hitAt returns the mouse target rendered at (x, y) in the last frame.
func (m *Composer) hitAt(x, y int) (composerHit, bool) {
	if y < 0 || y >= len(m.rowHits) {
		return composerHit{}, false
	}
	for _, h := range m.rowHits[y] {
		if x >= h.minX && x < h.maxX {
			return h, true
		}
	}
	return composerHit{}, false
}

// handleMouseClick maps a left click (relative to the composer's top-left
// corner) onto the element rendered there in the last frame.
func (m *Composer) handleMouseClick(msg tea.MouseClickMsg) tea.Cmd {
	if msg.Button != tea.MouseLeft {
		return nil
	}
	hit, ok := m.hitAt(msg.X, msg.Y)
	switch {
	case m.showAccountPicker:
		if ok && hit.account >= 0 {
			m.selectAccount(hit.account)
			m.showAccountPicker = false
		}
		return nil
	case m.dialogOpen(), !ok:
		// Other dialogs only take keyboard input.
		return nil
	case hit.suggestion >= 0:
		m.acceptContactSuggestion(hit.suggestion)
		return nil
	case hit.focus < 0 || !slices.Contains(m.orderedFocusIndices(), hit.focus):
		// Inert row, or a field that can't take focus (e.g. From with a
		// single account).
		return nil
	}

	prev := m.focusIndex
	if prev == focusFrom && hit.focus != focusFrom {
		m.validateFromField()
	} else if prev != hit.focus {
		m.validateEmailField(prev)
	}
	cmd := m.setFocusTo(hit.focus)
	switch hit.focus {
	case focusAttachment:
		if hit.attachment >= 0 {
			m.attachmentCursor = hit.attachment
		}
	case focusEncryptSMIME, focusSignSMIME, focusSignPGP, focusEncryptPGP:
		m.toggleCrypto(hit.focus)
	case focusBody:
		m.updateSpellSuggestions()
		m.updateEmojiSuggestions()
	}
	return cmd
}

// handleMouseWheel scrolls the body or signature editor under the pointer
// (falling back to the focused one) by moving its cursor, which keeps the
// editor's viewport in sync.
func (m *Composer) handleMouseWheel(msg tea.MouseWheelMsg) {
	if m.dialogOpen() {
		return
	}
	target := m.focusIndex
	if hit, ok := m.hitAt(msg.X, msg.Y); ok && (hit.focus == focusBody || hit.focus == focusSignature) {
		target = hit.focus
	}
	var ta *textarea.Model
	switch target {
	case focusBody:
		ta = &m.bodyInput
	case focusSignature:
		ta = &m.signatureInput
	default:
		return
	}
	var move func()
	switch msg.Button {
	case tea.MouseWheelUp:
		move = ta.CursorUp
	case tea.MouseWheelDown:
		move = ta.CursorDown
	default:
		return
	}
	const wheelLines = 3
	for range wheelLines {
		move()
	}
	if target == focusBody && m.focusIndex == focusBody {
		m.updateSpellSuggestions()
		m.updateEmojiSuggestions()
	}
}

// View renders the composer. Whenever the size is known it fits exactly
// inside the last tea.WindowSizeMsg, so it can also be embedded as a pane.
func (m *Composer) View() tea.View {
	var out string
	switch {
	case m.showPluginPrompt:
		out = m.renderPluginPrompt()
	case m.showAccountPicker:
		out = m.renderAccountPicker()
	case m.confirmingExit:
		out = m.renderConfirmExit()
	case len(m.wkdConfirmRecipients) > 0:
		out = m.renderWKDConfirm()
	default:
		out = m.renderForm()
	}
	v := tea.NewView(out)
	if config.MouseEnabled != nil && *config.MouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// baseFixedRows is the number of non-editor rows in the default layout (no
// errors, attachments or crypto toggles). The editors are sized from what
// remains, so transient rows eat into the spare gap before the editors shrink.
func (m *Composer) baseFixedRows() int {
	n := 9 // title, from, to, subject, signature label, attachments, send, blank, help
	if m.ccBccVisible() {
		n += 2
	}
	if !m.hideTips {
		n++
	}
	return n
}

func blocksHeight(blocks []composerBlock) int {
	n := 0
	for _, b := range blocks {
		n += lipgloss.Height(b.text)
	}
	return n
}

// renderForm renders the compose form and records the mouse hit map. When
// the window size is known, the result is exactly m.height rows of at most
// m.width cells: optional rows are dropped and the editors shrink as needed.
func (m *Composer) renderForm() string {
	w, h := m.width, m.height
	header, toEnd := m.headerBlocks()
	attachments := m.attachmentBlocks()
	toggles := make([]composerBlock, 0, 4)
	for _, f := range m.visibleCryptoToggles() {
		toggles = append(toggles, composerField(truncateLines(m.toggleView(f), w), f))
	}
	tip := m.tipView()
	help := m.helpView()

	fit := composerFit{
		bodyH:     m.bodyInput.Height(),
		sigH:      m.signatureInput.Height(),
		showSig:   true,
		showTip:   tip != "",
		attachGap: len(m.attachmentPaths) > 0,
		buttonGap: true,
		keepSig:   m.focusIndex == focusSignature,
	}
	if h > 0 {
		fixed := blocksHeight(header) + blocksHeight(attachments) + len(toggles) + 1 + lipgloss.Height(help)
		fit.bodyH, fit.sigH = splitComposerHeights(h - m.baseFixedRows())
		for fit.rows(fixed) > h {
			if !fit.shrink() {
				break
			}
		}
		m.bodyInput.SetHeight(fit.bodyH)
		if fit.showSig {
			m.signatureInput.SetHeight(fit.sigH)
		}
	}

	var fr composerFrame
	anchorRow := 0
	for i, b := range header {
		fr.add(b)
		if i == toEnd-1 {
			anchorRow = len(fr.lines)
		}
	}

	bodyView := m.bodyInput.View()
	if !m.disableSpellcheck && m.spellChecker != nil && m.spellChecker.Loaded() {
		bodyView = spellcheck.Highlight(bodyView, m.spellChecker, -1)
	}
	m.bodyRow = len(fr.lines)
	fr.add(composerField(truncateLines(bodyView, w), focusBody))
	if fit.showSig {
		fr.add(composerField(truncateLines(m.signatureLabelView(), w), focusSignature))
		fr.add(composerField(truncateLines(m.signatureInput.View(), w), focusSignature))
	}
	for _, b := range attachments {
		fr.add(b)
	}
	if fit.attachGap {
		fr.add(composerInert(""))
	}
	for _, b := range toggles {
		fr.add(b)
	}
	fr.add(composerField(m.sendButtonView(), focusSend))
	if fit.buttonGap {
		fr.add(composerInert(""))
	}
	if fit.showTip {
		fr.add(composerInert(tip))
	}
	if h > 0 {
		for len(fr.lines) < h-lipgloss.Height(help) {
			fr.add(composerInert(""))
		}
	} else {
		fr.add(composerInert(""))
	}
	fr.add(composerInert(help))

	m.rowHits = fr.hits
	out := strings.Join(fr.lines, "\n")
	if m.focusIndex == focusTo && m.showSuggestions && len(m.suggestions) > 0 {
		out = m.overlaySuggestions(out, anchorRow)
	}
	if m.focusIndex == focusBody && m.spellShow && len(m.spellSuggestions) > 0 {
		out = m.overlayBodyPopup(out, m.renderSpellPopupLines())
	}
	if m.focusIndex == focusBody && m.emojiShow && len(m.emojiSuggestions) > 0 {
		out = m.overlayBodyPopup(out, m.renderEmojiPopupLines())
	}
	return clampRendered(out, w, h)
}

// headerBlocks renders the address and subject rows. toEnd is the number of
// blocks up to and including the To field (and its error), i.e. where the
// contact suggestion dropdown attaches.
func (m *Composer) headerBlocks() (blocks []composerBlock, toEnd int) {
	w := m.width
	blocks = append(blocks,
		composerInert(truncateLines(t("composer.title"), w)),
		composerField(m.fromFieldView(), focusFrom),
	)
	if m.isCatchAllAccount() && m.fromError != "" {
		blocks = append(blocks, composerField(truncateLines(composerErrorStyle.Render(m.fromError), w), focusFrom))
	}
	blocks = append(blocks, m.inputBlocks(m.toInput.View(), m.toError, focusTo)...)
	toEnd = len(blocks)
	if m.ccBccVisible() {
		blocks = append(blocks, m.inputBlocks(m.ccInput.View(), m.ccError, focusCc)...)
		blocks = append(blocks, m.inputBlocks(m.bccInput.View(), m.bccError, focusBcc)...)
	}
	blocks = append(blocks, composerField(truncateLines(m.subjectInput.View(), w), focusSubject))
	return blocks, toEnd
}

// inputBlocks renders an address input plus its validation error, if any.
func (m *Composer) inputBlocks(view, errText string, focus int) []composerBlock {
	blocks := []composerBlock{composerField(truncateLines(view, m.width), focus)}
	if errText != "" {
		blocks = append(blocks, composerField(truncateLines(composerErrorStyle.Render(errText), m.width), focus))
	}
	return blocks
}

// fromFieldView renders the From row, shortening the address (not the hint)
// so the row fits the width.
func (m *Composer) fromFieldView() string {
	w := m.width
	from := t("composer.from")
	if m.isCatchAllAccount() {
		prefix := "  " + from + " "
		suffix := ""
		if len(m.accounts) > 1 {
			if m.focusIndex == focusFrom {
				prefix = focusedStyle.Render(fmt.Sprintf("> %s ", from))
				suffix = " " + blurredStyle.Render("["+t("composer.enter_to_switch")+"]")
			} else {
				prefix = blurredStyle.Render(fmt.Sprintf("  %s ", from))
				suffix = " " + blurredStyle.Render("["+t("composer.switchable")+"]")
			}
		}
		if w > 0 {
			// Let the editable address scroll inside the row.
			avail := w - lipgloss.Width(prefix) - lipgloss.Width(suffix) - lipgloss.Width(m.fromInput.Prompt) - 1
			m.fromInput.SetWidth(max(avail, 1))
		}
		return truncateLines(prefix+m.fromInput.View()+suffix, w)
	}

	addr := m.getFromAddress()
	shorten := func(reserved int) string {
		if w <= 0 {
			return addr
		}
		return ansi.Truncate(addr, max(w-reserved, 1), "…")
	}
	switch {
	case len(m.accounts) > 1:
		marker, style, hint := "  ", blurredStyle, t("composer.switchable")
		if m.focusIndex == focusFrom {
			marker, style, hint = "> ", focusedStyle, t("composer.enter_to_switch")
		}
		prefix := marker + from + " "
		suffix := " [" + hint + "]"
		return truncateLines(style.Render(prefix+shorten(lipgloss.Width(prefix)+lipgloss.Width(suffix))+suffix), w)
	case addr != "":
		prefix := "  " + from + " "
		return truncateLines(prefix+emailRecipientStyle.Render(shorten(lipgloss.Width(prefix))), w)
	default:
		return truncateLines(blurredStyle.Render(fmt.Sprintf("  %s (%s)", from, t("composer.no_account"))), w)
	}
}

// attachmentBlocks renders the attachment header and one row per attachment.
func (m *Composer) attachmentBlocks() []composerBlock {
	w := m.width
	focused := m.focusIndex == focusAttachment
	if len(m.attachmentPaths) == 0 {
		text := fmt.Sprintf("%s %s (%s)", t("composer.attachments"), t("composer.attachments_none"), t("composer.enter_to_add"))
		line := blurredStyle.Render("  " + text)
		if focused {
			line = focusedStyle.Render("> " + text)
		}
		return []composerBlock{composerField(truncateLines(attachmentStyle.Render(line), w), focusAttachment)}
	}

	headerPrefix, headerStyle := "  ", blurredStyle
	if focused {
		headerPrefix, headerStyle = "> ", focusedStyle
	}
	header := headerStyle.Render(fmt.Sprintf("%s%s (%d):", headerPrefix, t("composer.attachments"), len(m.attachmentPaths)))
	blocks := []composerBlock{composerField(truncateLines(attachmentStyle.Render(header), w), focusAttachment)}
	for i, p := range m.attachmentPaths {
		cursor, style := "    ", blurredStyle
		if focused && i == m.attachmentCursor {
			cursor, style = "  > ", focusedStyle
		}
		line := attachmentStyle.Render(style.Render(cursor + m.attachmentDisplayName(p)))
		blocks = append(blocks, composerBlock{text: truncateLines(line, w), focus: focusAttachment, attachment: i})
	}
	return blocks
}

// toggleView renders one crypto toggle row.
func (m *Composer) toggleView(focus int) string {
	var label string
	var on bool
	switch focus {
	case focusEncryptSMIME:
		label, on = t("composer.encrypt_smime"), m.encryptSMIME
	case focusSignSMIME:
		label, on = t("composer.sign_smime"), m.signSMIME
	case focusSignPGP:
		label, on = t("composer.sign_pgp"), m.signPGP
	case focusEncryptPGP:
		label, on = t("composer.encrypt_pgp"), m.encryptPGP
	}
	box := "[ ]"
	if on {
		box = "[x]"
	}
	if m.focusIndex == focus {
		return smimeToggleStyle.Render(focusedStyle.Render(fmt.Sprintf("> %s %s", label, box)))
	}
	return smimeToggleStyle.Render(blurredStyle.Render(fmt.Sprintf("  %s %s", label, box)))
}

func (m *Composer) sendButtonView() string {
	label := "[ " + t("composer.send") + " ]"
	if m.focusIndex == focusSend {
		return focusedStyle.Render(label)
	}
	return blurredStyle.Render(label)
}

func (m *Composer) signatureLabelView() string {
	if m.focusIndex == focusSignature {
		return focusedStyle.Render(t("composer.signature") + ":")
	}
	return blurredStyle.Render(t("composer.signature") + ":")
}

// tipText is the contextual tip for the focused element.
func (m *Composer) tipText() string {
	switch m.focusIndex {
	case focusFrom:
		return "Select the account to send from."
	case focusTo:
		return "Enter recipient email addresses."
	case focusCc:
		return "Carbon copy recipients."
	case focusBcc:
		return "Blind carbon copy recipients."
	case focusSubject:
		return "The subject line of your email."
	case focusBody:
		if m.spellShow && len(m.spellSuggestions) > 0 {
			sk := config.Keybinds.Composer
			return fmt.Sprintf("Spelling: %s accept • %s/%s navigate • %s dismiss",
				sk.SpellAccept, sk.SpellNext, sk.SpellPrev, sk.SpellDismiss)
		}
		return "The main content of your email. Markdown and HTML are supported."
	case focusSignature:
		return "Your email signature. This will be appended to the end of the email."
	case focusAttachment:
		return fmt.Sprintf("Enter: add file • up/down: select attachment • %s: remove selected", config.Keybinds.Composer.Delete)
	case focusEncryptSMIME:
		return "Press Space or Enter to toggle S/MIME encryption on or off."
	case focusSignSMIME:
		return "Press Space or Enter to toggle S/MIME signing on or off."
	case focusSignPGP:
		return "Press Space or Enter to toggle PGP signing on or off."
	case focusEncryptPGP:
		return "Press Space or Enter to toggle PGP encryption on or off."
	case focusSend:
		return "Press Enter to send the email."
	}
	return ""
}

// tipView renders the tip as a single row, or "" when tips are hidden.
func (m *Composer) tipView() string {
	tip := m.tipText()
	if m.hideTips || tip == "" {
		return ""
	}
	return truncateLines(TipStyle.Render("Tip: "+tip), m.width)
}

// helpView renders the help bar, wrapped to the width and capped at two rows.
func (m *Composer) helpView() string {
	helpText := keybindHelp("composer.help")
	for _, pk := range m.pluginKeyBindings {
		helpText += " • " + pk.Key + ": " + pk.Description
	}
	if m.pluginStatus != "" {
		helpText += " • " + m.pluginStatus
	}
	if m.width <= 0 {
		return helpStyle.Render(helpText)
	}
	const maxHelpRows = 2
	lines := strings.Split(lipgloss.Wrap(helpText, m.width, ""), "\n")
	if len(lines) > maxHelpRows {
		lines = lines[:maxHelpRows]
		last := lines[maxHelpRows-1]
		lines[maxHelpRows-1] = ansi.Truncate(last, max(m.width-1, 0), "") + "…"
	}
	return helpStyle.Render(strings.Join(lines, "\n"))
}

// suggestionBoxLines renders the contact suggestion dropdown, fitted to the
// width.
func (m *Composer) suggestionBoxLines() []string {
	sw := suggestionDisplayWidth(m.width)
	rows := make([]string, 0, len(m.suggestions))
	for i, s := range m.suggestions {
		display := suggestionDisplay(s, sw)
		if m.width > 0 {
			display = ansi.Truncate(display, sw, "…")
		}
		if i == m.selectedSuggestion {
			rows = append(rows, selectedSuggestionStyle.Render("> "+display))
		} else {
			rows = append(rows, suggestionStyle.Render("  "+display))
		}
	}
	return strings.Split(suggestionBoxStyle.Render(strings.Join(rows, "\n")), "\n")
}

// overlaySuggestions draws the contact suggestion dropdown over the rows
// below the To field and makes its entries clickable.
func (m *Composer) overlaySuggestions(view string, row int) string {
	box := m.suggestionBoxLines()
	boxW := 0
	for _, l := range box {
		boxW = max(boxW, lipgloss.Width(l))
	}
	for i := range box {
		r := row + i
		if r >= len(m.rowHits) {
			break
		}
		hit := composerHit{maxX: boxW, focus: -1, attachment: -1, suggestion: -1, account: -1}
		if i > 0 && i < len(box)-1 {
			hit.suggestion = i - 1 // first and last rows are the box border
		}
		m.rowHits[r] = append([]composerHit{hit}, m.rowHits[r]...)
	}
	return overlay.Block(view, box, row, 0)
}

// overlayBodyPopup floats popup at the body cursor: below it when there is
// room above the help row, otherwise above it.
func (m *Composer) overlayBodyPopup(view string, popup []string) string {
	if len(popup) == 0 || m.bodyRow < 0 {
		return view
	}
	// The textarea knows where its cursor really is (wrapped lines, scroll
	// offset, prompt and line-number gutter); it only reports it for a real
	// terminal cursor, so ask a copy configured for one.
	ta := m.bodyInput
	ta.SetVirtualCursor(false)
	c := ta.Cursor()
	if c == nil {
		return view
	}
	cursorRow := m.bodyRow + c.Y

	anchorRow := cursorRow + 1
	if m.height > 0 && anchorRow+len(popup) > m.height-1 {
		if cursorRow-len(popup) >= 0 {
			anchorRow = cursorRow - len(popup)
		} else {
			anchorRow = max(m.height-1-len(popup), 0)
		}
	}
	anchorCol := c.X
	if popupW := lipgloss.Width(popup[0]); m.width > 0 && anchorCol+popupW > m.width {
		anchorCol = max(m.width-popupW, 0)
	}
	return overlay.Block(view, popup, anchorRow, anchorCol)
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
		maxWidth = max(maxWidth, lipgloss.Width(s))
	}
	rowWidth := maxWidth + 6 // " abc " badge + word + trailing space

	iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("24")).Foreground(lipgloss.Color("231"))

	rows := make([]string, 0, len(m.spellSuggestions))
	for i, s := range m.spellSuggestions {
		text := " " + iconStyle.Render("abc") + " " + s
		text += strings.Repeat(" ", max(rowWidth-lipgloss.Width(text), 0))
		if i == m.spellSelected {
			rows = append(rows, selStyle.Render(text))
		} else {
			rows = append(rows, rowStyle.Render(text))
		}
	}
	box := suggestionBoxStyle.Render(strings.Join(rows, "\n"))
	return strings.Split(box, "\n")
}

// renderEmojiPopupLines builds the styled, bordered emoji suggestion box and
// returns its rendered lines. Each row shows the shortcode and its preview.
func (m *Composer) renderEmojiPopupLines() []string {
	if !m.emojiShow || len(m.emojiSuggestions) == 0 {
		return nil
	}
	maxWidth := 0
	for _, s := range m.emojiSuggestions {
		maxWidth = max(maxWidth, lipgloss.Width(s))
	}
	rowWidth := maxWidth + 2

	rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("24")).Foreground(lipgloss.Color("231"))

	rows := make([]string, 0, len(m.emojiSuggestions))
	for i, s := range m.emojiSuggestions {
		text := " " + s
		text += strings.Repeat(" ", max(rowWidth-lipgloss.Width(text), 0))
		if i == m.emojiSelected {
			rows = append(rows, selStyle.Render(text))
		} else {
			rows = append(rows, rowStyle.Render(text))
		}
	}
	box := suggestionBoxStyle.Render(strings.Join(rows, "\n"))
	return strings.Split(box, "\n")
}

// dialogContentWidth is the widest content a dialog can hold inside the
// composer, or 0 while the size is unknown.
func (m *Composer) dialogContentWidth() int {
	if m.width <= 0 {
		return 0
	}
	return max(m.width-DialogBoxStyle.GetHorizontalFrameSize(), 1)
}

// dialogView boxes content (wrapping it to the composer width when needed)
// and centers it in the composer area.
func (m *Composer) dialogView(content string) string {
	box := DialogBoxStyle
	if m.width > 0 && lipgloss.Width(content)+box.GetHorizontalFrameSize() > m.width {
		box = box.Width(m.width)
	}
	view, _, _ := centerInArea(box.Render(content), m.width, m.height)
	return view
}

func (m *Composer) renderPluginPrompt() string {
	m.rowHits = nil
	if maxW := m.dialogContentWidth(); maxW > 0 {
		inputW := min(max(lipgloss.Width(m.pluginPromptPlaceholder), 20), maxW)
		m.pluginPromptInput.SetWidth(max(inputW-lipgloss.Width(m.pluginPromptInput.Prompt)-1, 1))
	}
	return m.dialogView(lipgloss.JoinVertical(lipgloss.Left,
		m.pluginPromptPlaceholder,
		"",
		m.pluginPromptInput.View(),
		"",
		HelpStyle.Render("enter: submit • esc: cancel"),
	))
}

// renderAccountPicker renders the account picker dialog and makes its rows
// clickable.
func (m *Composer) renderAccountPicker() string {
	maxW := m.dialogContentWidth()
	fitText := func(s string, reserved int) string {
		if maxW <= 0 {
			return s
		}
		return ansi.Truncate(s, max(maxW-reserved, 1), "…")
	}
	const marker = 4 // item padding + "> "
	lines := []string{fitText("Select Account:", 0), ""}
	for i := range m.accounts {
		acc := &m.accounts[i]
		display := acc.GetSendAsEmail()
		if acc.Name != "" {
			display = fmt.Sprintf("%s (%s)", acc.Name, display)
		}
		if i == m.accountPickerCursor {
			lines = append(lines, selectedItemStyle.Render("> "+fitText(display, marker)))
		} else {
			lines = append(lines, itemStyle.Render("  "+fitText(display, marker)))
		}
	}
	lines = append(lines, "", HelpStyle.Render(fitText("↑/↓: navigate • enter: select • esc: cancel", 0)))

	dialog := DialogBoxStyle.Render(strings.Join(lines, "\n"))
	view, top, left := centerInArea(dialog, m.width, m.height)
	dialogW := lipgloss.Width(dialog)
	firstRow := top + DialogBoxStyle.GetBorderTopSize() + DialogBoxStyle.GetPaddingTop() + 2
	m.rowHits = nil
	for i := range m.accounts {
		row := firstRow + i
		for len(m.rowHits) <= row {
			m.rowHits = append(m.rowHits, nil)
		}
		m.rowHits[row] = []composerHit{{minX: left, maxX: left + dialogW, focus: -1, attachment: -1, suggestion: -1, account: i}}
	}
	return view
}

func (m *Composer) renderConfirmExit() string {
	m.rowHits = nil
	text := t("composer.exit_confirm")
	if maxW := m.dialogContentWidth(); maxW > 0 && lipgloss.Width(text) > maxW {
		text = lipgloss.Wrap(text, maxW, "")
	}
	return m.dialogView(lipgloss.JoinVertical(lipgloss.Center,
		text,
		HelpStyle.Render("\n(y/n)"),
	))
}

func (m *Composer) renderWKDConfirm() string {
	m.rowHits = nil
	maxW := m.dialogContentWidth()
	var msg strings.Builder
	msg.WriteString("Missing PGP keys for:\n\n")
	for _, r := range m.wkdConfirmRecipients {
		line := "  • " + r
		if maxW > 0 {
			line = ansi.Truncate(line, maxW, "…")
		}
		msg.WriteString(line + "\n")
	}
	msg.WriteString("\nDownload from WKD?")
	return m.dialogView(lipgloss.JoinVertical(lipgloss.Center,
		msg.String(),
		HelpStyle.Render("\n(y/n)"),
	))
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

// ToggleCcBcc toggles the visibility of CC/BCC fields.
func (m *Composer) ToggleCcBcc() {
	m.toggleCcBcc()
}

func (m *Composer) toggleCcBcc() tea.Cmd {
	m.showCcBcc = !m.showCcBcc
	return m.leaveHiddenCcBcc()
}

// SetShowCcBcc sets the visibility of CC/BCC fields.
func (m *Composer) SetShowCcBcc(show bool) {
	m.showCcBcc = show
	m.leaveHiddenCcBcc()
}

// leaveHiddenCcBcc moves focus off Cc/Bcc once those rows are hidden, so
// typing never goes into an invisible field.
func (m *Composer) leaveHiddenCcBcc() tea.Cmd {
	if m.ccBccVisible() || (m.focusIndex != focusCc && m.focusIndex != focusBcc) {
		return nil
	}
	return m.setFocusTo(focusSubject)
}

// GetShowCcBcc returns whether CC/BCC fields are visible.
func (m *Composer) GetShowCcBcc() bool {
	return m.showCcBcc
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
	// Open spelling/emoji popups hold offsets into the previous text.
	m.spellShow = false
	m.spellSuggestions = nil
	m.spellWord = ""
	m.emojiShow = false
	m.emojiSuggestions = nil
	m.emojiShortcode = ""
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
	m.pluginPromptInput.SetWidth(40) // refitted to the window when rendered
	m.pluginPromptInput.Focus()
	m.showPluginPrompt = true
}

// HidePluginPrompt deactivates the plugin prompt overlay.
func (m *Composer) HidePluginPrompt() {
	m.showPluginPrompt = false
}

// HasContent reports whether the composer holds anything worth persisting.
// It is used to avoid saving empty drafts when the user quits the composer.
func (m *Composer) HasContent() bool {
	return m.hasAnyRecipient() ||
		strings.TrimSpace(m.subjectInput.Value()) != "" ||
		strings.TrimSpace(m.bodyInput.Value()) != "" ||
		len(m.attachmentPaths) > 0
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
		SignSMIME:       m.signSMIME,
		EncryptSMIME:    m.encryptSMIME,
		SignPGP:         m.signPGP,
		EncryptPGP:      m.encryptPGP,
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
	m.signSMIME = draft.SignSMIME
	m.encryptSMIME = draft.EncryptSMIME
	m.signPGP = draft.SignPGP
	m.encryptPGP = draft.EncryptPGP
	return m
}
