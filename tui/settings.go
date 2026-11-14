package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	overlay "github.com/floatpane/bubble-overlay"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/internal/passwordstrength"
	"github.com/floatpane/matcha/plugin"
	"github.com/floatpane/matcha/theme"
)

var (
	accountItemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedAccountItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42")).Bold(true)
	accountEmailStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dangerStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	successStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

	settingsFocusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	settingsBlurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// settingsDoubleClick is the longest gap between two clicks on the same row
// that still counts as a double click.
const settingsDoubleClick = 500 * time.Millisecond

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
	CategoryPlugins
	CategoryContacts
)

// settingsCategoryCount is the number of entries in the settings menu.
const settingsCategoryCount = int(CategoryContacts) + 1

type Settings struct {
	cfg    *config.Config
	width  int
	height int

	activePane     SettingsPane
	activeCategory SettingsCategory

	// Menu state
	menuCursor int

	// Sub-components states
	generalCursor  int
	accountsCursor int
	themeCursor    int
	listsCursor    int
	// Pending delete prompts. Each category owns its own flag so a prompt can
	// never leak into (and index) another category's list.
	accountsConfirmingDelete bool
	listsConfirmingDelete    bool

	// Contacts state
	contactsCursor     int
	contactsConfirming bool
	contactsList       []config.Contact

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
	encPasswordInput    textinput.Model
	encConfirmInput     textinput.Model
	encFocusIndex       int
	encError            string
	encEnabling         bool // EnableSecureMode is running
	encDisabling        bool // DisableSecureMode is running
	confirmingDisable   bool
	passwordMeter       passwordstrength.Meter
	encPasswordStrength passwordstrength.Strength

	// Plugin settings state
	plugins             *plugin.Manager
	pluginListCursor    int
	pluginSelected      string // name of plugin whose settings are open ("" = list view)
	pluginSettingCursor int
	pluginEditing       bool
	pluginEditingKey    string
	pluginEditingType   plugin.SettingType
	pluginInput         textinput.Model

	// scroll holds the first visible body row of each scrollable page.
	scroll map[string]int

	// Double-click detection for content rows.
	lastClickTime time.Time
	lastClickKey  string
	lastClickIdx  int
}

type SettingsState struct {
	ActivePane     SettingsPane
	ActiveCategory SettingsCategory
	MenuCursor     int
	GeneralCursor  int
	AccountsCursor int
	ThemeCursor    int
	ListsCursor    int
	PluginCursor   int
	ContactsCursor int
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
		contactsList:       config.GetNamedContacts(),
		smimeCertInput:     newInput("/path/to/cert.pem", "> ", false),
		smimeKeyInput:      newInput("/path/to/private_key.pem", "> ", false),
		pgpPublicKeyInput:  newInput("/path/to/public_key.asc", "> ", false),
		pgpPrivateKeyInput: newInput("/path/to/private_key.asc", "> ", false),
		pgpPINInput:        newInput("YubiKey PIN (6-8 digits)", "> ", true),
		pgpKeySource:       "file",
		encPasswordInput:   newInput("Password", "> ", true),
		encConfirmInput:    newInput("Confirm Password", "> ", true),
		passwordMeter:      passwordstrength.NewLibMeter(),
		pluginInput:        newInput("", "> ", false),
		scroll:             make(map[string]int),
	}
}

// SetPlugins attaches the plugin manager so the settings view can list and
// edit plugin-declared settings.
func (m *Settings) SetPlugins(p *plugin.Manager) {
	m.plugins = p
}

func (m *Settings) GetState() SettingsState {
	return SettingsState{
		ActivePane:     m.activePane,
		ActiveCategory: m.activeCategory,
		MenuCursor:     m.menuCursor,
		GeneralCursor:  m.generalCursor,
		AccountsCursor: m.accountsCursor,
		ThemeCursor:    m.themeCursor,
		ListsCursor:    m.listsCursor,
		PluginCursor:   m.pluginListCursor,
		ContactsCursor: m.contactsCursor,
	}
}

func (m *Settings) RestoreState(state SettingsState) {
	m.activePane = state.ActivePane
	m.activeCategory = state.ActiveCategory
	m.menuCursor = state.MenuCursor
	// The highlighted menu entry always names the category on screen; a
	// state that only sets ActiveCategory must not leave "General" selected.
	if cat := int(state.ActiveCategory); cat >= 0 && cat < settingsCategoryCount {
		m.menuCursor = cat
	}
	m.generalCursor = state.GeneralCursor
	m.accountsCursor = state.AccountsCursor
	m.themeCursor = state.ThemeCursor
	m.listsCursor = state.ListsCursor
	m.pluginListCursor = state.PluginCursor
	m.contactsCursor = state.ContactsCursor
	if state.ActiveCategory == CategoryContacts {
		m.contactsList = config.GetNamedContacts()
	}
	m.clearPrompts()
}

func (m *Settings) Init() tea.Cmd {
	return textinput.Blink
}

// categoriesStartY returns the screen row where the first settings category
// appears, derived from the rendered menu header.
func (m *Settings) categoriesStartY() int {
	return docStyle.GetMarginTop() + blockHeight(m.menuHeader())
}

func (m *Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		m.handleWheel(msg)
		return m, nil

	case tea.MouseClickMsg:
		return m, m.handleClick(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeInputs()
		return m, nil

	case tea.KeyPressMsg:
		return m.updateKeyPress(msg)

	case SecureModeEnabledMsg:
		m.encEnabling = false
		if msg.Err != nil {
			m.encError = msg.Err.Error()
			return m, nil
		}
		m.encError = ""
		m.encPasswordInput.SetValue("")
		m.encConfirmInput.SetValue("")
		m.encPasswordStrength = ""
		m.encPasswordInput.Blur()
		m.encConfirmInput.Blur()
		if m.activeCategory == CategoryEncryption {
			m.leaveContent()
		}
		return m, nil

	case SecureModeDisabledMsg:
		m.encDisabling = false
		if msg.Err != nil {
			m.encError = msg.Err.Error()
			return m, nil
		}
		m.encError = ""
		m.confirmingDisable = false
		if m.activeCategory == CategoryEncryption {
			m.leaveContent()
		}
		return m, nil
	}

	// Update text inputs if active
	if m.activePane == PaneContent {
		switch {
		case m.activeCategory == CategoryEncryption:
			m.encPasswordInput, cmd = m.encPasswordInput.Update(msg)
			cmds = append(cmds, cmd)
			m.encConfirmInput, cmd = m.encConfirmInput.Update(msg)
			cmds = append(cmds, cmd)
		case m.activeCategory == CategoryAccounts && m.isCryptoConfig:
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
		case m.activeCategory == CategoryPlugins && m.pluginEditing:
			m.pluginInput, cmd = m.pluginInput.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// resizeInputs sizes every text input to the content pane. A textinput
// renders its prompt plus one extra cell for the cursor next to the value,
// so both are subtracted to keep a full field from wrapping.
func (m *Settings) resizeInputs() {
	width := m.contentWidth()
	if width <= 0 {
		return
	}
	for _, in := range []*textinput.Model{
		&m.smimeCertInput, &m.smimeKeyInput, &m.pgpPublicKeyInput,
		&m.pgpPrivateKeyInput, &m.pgpPINInput, &m.pluginInput,
		&m.encPasswordInput, &m.encConfirmInput,
	} {
		in.SetWidth(max(1, width-lipgloss.Width(in.Prompt)-1))
	}
}

func (m *Settings) updateKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.activePane == PaneContent {
		switch msg.String() {
		case "esc":
			// Pages with their own esc handling (prompts, sub-forms) keep it.
			if !m.contentHandlesEsc() {
				m.leaveContent()
				return m, nil
			}
		case keyLeft:
			if m.canFocusSettingsMenuWithLeft() {
				m.leaveContent()
				return m, nil
			}
		}
	}

	if m.activePane == PaneMenu {
		return m.updateMenu(msg)
	}
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
	case CategoryPlugins:
		return m.updatePlugins(msg)
	case CategoryContacts:
		return m.updateContacts(msg)
	}

	return m, nil
}

// contentHandlesEsc reports whether the active page consumes esc itself
// (cancelling a prompt, closing a sub-form or input) instead of returning to
// the menu.
func (m *Settings) contentHandlesEsc() bool {
	switch m.activeCategory {
	case CategoryAccounts:
		return m.isCryptoConfig || m.accountsConfirmingDelete
	case CategoryMailingLists:
		return m.listsConfirmingDelete
	case CategoryContacts:
		return m.contactsConfirming
	case CategoryEncryption:
		// The password form clears its inputs on esc; the enabled page only
		// keeps esc while its disable prompt is open.
		return !config.IsSecureModeEnabled() || m.confirmingDisable
	case CategoryPlugins:
		return m.pluginEditing || m.pluginSelected != ""
	case CategoryGeneral, CategoryTheme:
		return false
	default:
		return false
	}
}

func (m *Settings) canFocusSettingsMenuWithLeft() bool {
	switch m.activeCategory {
	case CategoryAccounts:
		return !m.isCryptoConfig && !m.accountsConfirmingDelete
	case CategoryMailingLists:
		return !m.listsConfirmingDelete
	case CategoryEncryption:
		return config.IsSecureModeEnabled() && !m.confirmingDisable
	case CategoryPlugins:
		return !m.pluginEditing && m.pluginSelected == ""
	case CategoryContacts:
		return !m.contactsConfirming
	case CategoryGeneral, CategoryTheme:
		return true
	default:
		return true
	}
}

// clearPrompts cancels every pending confirmation prompt.
func (m *Settings) clearPrompts() {
	m.accountsConfirmingDelete = false
	m.listsConfirmingDelete = false
	m.contactsConfirming = false
	m.confirmingDisable = false
}

// leaveContent returns focus to the menu, dropping any pending prompt so it
// cannot resurface (or act) on another category.
func (m *Settings) leaveContent() {
	m.clearPrompts()
	m.activePane = PaneMenu
}

// openCategory focuses the content pane of cat, resetting the transient state
// a fresh visit starts from. Keyboard (enter/right) and mouse clicks share it.
func (m *Settings) openCategory(cat SettingsCategory) tea.Cmd {
	m.menuCursor = int(cat)
	m.activeCategory = cat
	m.activePane = PaneContent
	m.clearPrompts()
	m.isCryptoConfig = false
	m.pluginSelected = ""
	m.pluginEditing = false
	m.pluginInput.Blur()

	switch cat {
	case CategoryGeneral, CategoryAccounts, CategoryMailingLists, CategoryPlugins:
		// no extra reset needed
	case CategoryContacts:
		m.contactsList = config.GetNamedContacts()
		m.contactsCursor = min(m.contactsCursor, len(m.contactsList))
	case CategoryTheme:
		// Find current theme index
		themes := theme.AllThemes()
		for i, t := range themes {
			if t.Name == theme.ActiveTheme.Name {
				m.themeCursor = i
				break
			}
		}
	case CategoryEncryption:
		m.encError = ""
		m.encPasswordInput.SetValue("")
		m.encConfirmInput.SetValue("")
		m.encPasswordStrength = ""
		m.encFocusIndex = 0
		// encEnabling/encDisabling are left alone: an operation that is
		// still running must keep blocking a second submit.
		m.encPasswordInput.Blur()
		m.encConfirmInput.Blur()
		if !config.IsSecureModeEnabled() {
			m.encPasswordInput.Focus()
		}
	}

	return textinput.Blink
}

func (m *Settings) contentItemStyle(selected bool) lipgloss.Style {
	if selected && m.activePane == PaneContent {
		return selectedAccountItemStyle
	}
	return accountItemStyle
}

func (m *Settings) contentCursor(selected bool) string {
	if selected && m.activePane == PaneContent {
		return "▌ "
	}
	return "  "
}

func (m *Settings) contentFocusStyle() lipgloss.Style {
	if m.activePane == PaneContent {
		return settingsFocusedStyle
	}
	return settingsBlurredStyle
}

func (m *Settings) updateMenu(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.menuCursor = (m.menuCursor - 1 + settingsCategoryCount) % settingsCategoryCount
	case keyDown, "j":
		m.menuCursor = (m.menuCursor + 1) % settingsCategoryCount
	case keyRight, "l", keyEnter:
		return m, m.openCategory(SettingsCategory(m.menuCursor))
	case "esc":
		return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
	}
	m.activeCategory = SettingsCategory(m.menuCursor)
	return m, nil
}

// --- Mouse -------------------------------------------------------------------

// handleWheel scrolls whichever pane has focus.
func (m *Settings) handleWheel(msg tea.MouseWheelMsg) {
	if m.dialogOpen() {
		return
	}
	var delta int
	switch msg.Button {
	case tea.MouseWheelDown:
		delta = 1
	case tea.MouseWheelUp:
		delta = -1
	default:
		return
	}
	if m.activePane == PaneMenu {
		m.menuCursor = max(0, min(settingsCategoryCount-1, m.menuCursor+delta))
		m.activeCategory = SettingsCategory(m.menuCursor)
		return
	}
	if cur, n := m.listCursor(); cur != nil && n > 0 {
		*cur = max(0, min(n-1, *cur+delta))
	}
}

// handleClick opens a category clicked in the menu, and selects (click) or
// activates (double click) a row clicked in the content pane.
func (m *Settings) handleClick(msg tea.MouseClickMsg) tea.Cmd {
	if msg.Button != tea.MouseLeft || m.dialogOpen() {
		return nil
	}
	menuLeft := docStyle.GetMarginLeft()
	contentLeft := menuLeft + settingsMenuWidth
	if msg.X >= menuLeft && msg.X < contentLeft {
		idx := msg.Y - m.categoriesStartY()
		if idx >= 0 && idx < settingsCategoryCount {
			return m.openCategory(SettingsCategory(idx))
		}
		return nil
	}
	if msg.X < contentLeft || (m.width > 0 && msg.X >= m.width-docStyle.GetMarginRight()) {
		return nil
	}
	row := msg.Y - docStyle.GetMarginTop()
	if maxH := m.contentMaxHeight(); row < 0 || (maxH > 0 && row >= maxH) {
		return nil
	}
	if m.activePane == PaneMenu {
		// A click on the previewed page focuses it, like enter.
		return m.openCategory(m.activeCategory)
	}
	return m.clickContentRow(row)
}

// clickContentRow maps a content-relative row to a list entry of the active
// page using the same layout the view renders.
func (m *Settings) clickContentRow(row int) tea.Cmd {
	page := m.currentPage()
	if !page.rows {
		return nil
	}
	_, layout := m.layoutPage(page, m.contentWidth(), m.contentMaxHeight())
	rel := row - layout.bodyTop
	if rel < 0 || rel >= layout.end-layout.start {
		return nil
	}
	idx := layout.start + rel
	cur, n := m.listCursor()
	if cur == nil || idx >= n {
		return nil
	}

	now := time.Now()
	double := page.key == m.lastClickKey && idx == m.lastClickIdx &&
		now.Sub(m.lastClickTime) < settingsDoubleClick
	m.lastClickTime, m.lastClickKey, m.lastClickIdx = now, page.key, idx
	*cur = idx
	if !double {
		return nil
	}
	m.lastClickTime = time.Time{} // a third click starts a new pair
	_, cmd := m.updateKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	return cmd
}

// listCursor returns the cursor of the active list page and its entry count,
// or nil for pages that are forms rather than lists.
func (m *Settings) listCursor() (*int, int) {
	switch m.activeCategory {
	case CategoryGeneral:
		return &m.generalCursor, len(m.buildGeneralOptions())
	case CategoryAccounts:
		if m.isCryptoConfig {
			return nil, 0
		}
		return &m.accountsCursor, len(m.cfg.Accounts) + 1
	case CategoryTheme:
		return &m.themeCursor, len(theme.AllThemes())
	case CategoryMailingLists:
		return &m.listsCursor, len(m.cfg.MailingLists) + 1
	case CategoryContacts:
		return &m.contactsCursor, len(m.contactsList) + 1
	case CategoryPlugins:
		if m.plugins == nil || m.pluginEditing {
			return nil, 0
		}
		if m.pluginSelected == "" {
			return &m.pluginListCursor, len(m.plugins.Schemas())
		}
		return &m.pluginSettingCursor, len(m.plugins.Schema(m.pluginSelected))
	case CategoryEncryption:
		return nil, 0
	}
	return nil, 0
}

// --- Layout ------------------------------------------------------------------

// contentWidth is the usable width of the content pane (0 before the first
// WindowSizeMsg).
func (m *Settings) contentWidth() int {
	if m.width <= 0 {
		return 0
	}
	return max(1, m.width-docStyle.GetHorizontalMargins()-settingsMenuWidth-settingsContentPad)
}

// contentMaxHeight is how many rows the menu and content panes may use, which
// keeps the help bar (and a blank line above it) on screen (0 = unknown).
func (m *Settings) contentMaxHeight() int {
	if m.height <= 0 {
		return 0
	}
	return max(1, m.height-docStyle.GetVerticalMargins()-blockHeight(m.helpBar())-1)
}

// helpBar renders the bottom help line, wrapped to the screen.
func (m *Settings) helpBar() string {
	text := t("settings.help_content")
	if m.activePane == PaneMenu {
		text = t("settings.help_menu")
	}
	return helpStyle.Render(wrapBlock(text, m.width-docStyle.GetHorizontalMargins()))
}

// menuHeader is the block drawn above the category list (title plus a blank
// line).
func (m *Settings) menuHeader() string {
	return titleStyle.Render(ansi.Truncate(t("settings.title"), m.menuInnerWidth(), "…")) + "\n"
}

// menuInnerWidth is the menu's width minus its right padding and border.
func (m *Settings) menuInnerWidth() int {
	return settingsMenuWidth - settingsMenuPadRight - 1
}

func (m *Settings) renderMenu() string {
	categories := []string{
		t("settings.category_general"),
		t("settings.category_accounts"),
		t("settings.category_theme"),
		t("settings.category_mailing_lists"),
		t("settings.category_encryption"),
		t("settings.category_plugins"),
		t("settings.category_contacts"),
	}

	lines := make([]string, 0, len(categories)+1)
	lines = append(lines, m.menuHeader())
	for i, c := range categories {
		cursor := "  "
		style := accountItemStyle
		if m.menuCursor == i {
			if m.activePane == PaneMenu {
				cursor = "▌ "
				style = selectedAccountItemStyle
			} else {
				cursor = "• "
				style = selectedAccountItemStyle.UnsetBold()
			}
		}
		// One row per category keeps clicks mapped to the right entry.
		label := ansi.Truncate(cursor+c, m.menuInnerWidth()-style.GetHorizontalPadding(), "…")
		lines = append(lines, style.Render(label))
	}

	return lipgloss.NewStyle().
		Width(settingsMenuWidth).
		PaddingRight(settingsMenuPadRight).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(theme.ActiveTheme.Secondary).
		Render(strings.Join(lines, "\n"))
}

// currentPage builds the content page of the active category.
func (m *Settings) currentPage() settingsPage {
	switch m.activeCategory {
	case CategoryGeneral:
		return m.generalPage()
	case CategoryAccounts:
		return m.accountsPage()
	case CategoryTheme:
		return m.themePage()
	case CategoryMailingLists:
		return m.mailingListsPage()
	case CategoryEncryption:
		return m.encryptionPage()
	case CategoryPlugins:
		return m.pluginsPage()
	case CategoryContacts:
		return m.contactsPage()
	}
	return settingsPage{focusTop: -1, focusBot: -1}
}

// pendingConfirm describes the confirmation prompt of the active page, if any.
func (m *Settings) pendingConfirm() (title, detail string, ok bool) {
	if m.activePane != PaneContent {
		return "", "", false
	}
	switch m.activeCategory {
	case CategoryAccounts:
		if m.accountsConfirmingDelete && m.accountsCursor < len(m.cfg.Accounts) {
			return "Delete account?", m.cfg.Accounts[m.accountsCursor].Email, true
		}
	case CategoryMailingLists:
		if m.listsConfirmingDelete && m.listsCursor < len(m.cfg.MailingLists) {
			return t("settings_mailing_lists.delete_confirm"), m.cfg.MailingLists[m.listsCursor].Name, true
		}
	case CategoryContacts:
		if m.contactsConfirming && m.contactsCursor < len(m.contactsList) {
			return t("settings_contacts.delete_confirm"), m.contactsList[m.contactsCursor].Name, true
		}
	case CategoryEncryption:
		if m.confirmingDisable {
			return t("settings_encryption.disable_confirm"), t("settings_encryption.disable_warning"), true
		}
	case CategoryGeneral, CategoryTheme, CategoryPlugins:
	}
	return "", "", false
}

// dialogOpen reports whether a confirmation prompt is on screen; it is modal,
// so mouse input behind it is ignored.
func (m *Settings) dialogOpen() bool {
	_, _, ok := m.pendingConfirm()
	return ok
}

// renderDialog draws a centered y/n confirmation box that fits the screen.
func (m *Settings) renderDialog(title, detail string) string {
	lines := []string{
		dangerStyle.Render(title),
		accountEmailStyle.Render(detail),
		HelpStyle.Render("\n(y/n)"),
	}
	if m.width > 0 {
		inner := max(1, m.width-DialogBoxStyle.GetHorizontalFrameSize()-2)
		for i, line := range lines {
			if lipgloss.Width(line) > inner {
				lines[i] = wrapBlock(line, inner)
			}
		}
	}
	return DialogBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Center, lines...))
}

func (m *Settings) View() tea.View {
	maxH := m.contentMaxHeight()

	right, _ := m.layoutPage(m.currentPage(), m.contentWidth(), maxH)
	rightPanel := lipgloss.NewStyle().PaddingLeft(settingsContentPad).Render(right)
	content := clipLines(lipgloss.JoinHorizontal(lipgloss.Top, m.renderMenu(), rightPanel), maxH)

	help := m.helpBar()
	gap := 2 // newlines between content and help: one blank line
	if m.height > 0 {
		gap = max(1, m.height-docStyle.GetVerticalMargins()-blockHeight(content)-blockHeight(help)+1)
	}
	screen := content + strings.Repeat("\n", gap) + help
	if m.width > 0 {
		screen = truncateLines(screen, m.width-docStyle.GetHorizontalMargins())
	}
	rendered := docStyle.Render(screen)

	if title, detail, ok := m.pendingConfirm(); ok {
		dialog := m.renderDialog(title, detail)
		if m.width > 0 && m.height > 0 {
			rendered = overlay.Center(rendered, dialog, m.width, m.height)
		} else {
			rendered += "\n\n" + dialog
		}
	}

	v := tea.NewView(rendered)
	if config.MouseEnabled != nil && *config.MouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// saveConfig persists the settings and, when that fails, returns a command
// that surfaces the error instead of silently dropping it.
func (m *Settings) saveConfig() tea.Cmd {
	if err := config.SaveConfig(m.cfg); err != nil {
		return settingsErrorCmd("Could not save settings", err)
	}
	return nil
}

// settingsErrorCmd reports err to the user through the app's notification.
func settingsErrorCmd(what string, err error) tea.Cmd {
	text := what + ": " + err.Error()
	return func() tea.Msg { return NotifyMsg{Message: text} }
}

func (m *Settings) UpdateConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	m.cfg = cfg
	m.accountsCursor = min(m.accountsCursor, len(cfg.Accounts))
	m.listsCursor = min(m.listsCursor, len(cfg.MailingLists))
	if m.accountsCursor >= len(cfg.Accounts) {
		m.accountsConfirmingDelete = false
	}
	if m.listsCursor >= len(cfg.MailingLists) {
		m.listsConfirmingDelete = false
	}
	if m.editingAccountIdx >= len(cfg.Accounts) {
		m.isCryptoConfig = false
	}
}
