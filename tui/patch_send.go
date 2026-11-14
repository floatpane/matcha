package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/gitmail"
	"github.com/floatpane/matcha/theme"
)

const (
	psFocusRepo = iota
	psFocusCommitRange
	psFocusTo
	psFocusCc
	psFocusSend
	psFocusFrom // account selector; only focusable with several accounts
)

// patchPreviewLimit caps how much of a patch the preview holds.
const patchPreviewLimit = 256 << 10

// patchPreviewMsg carries a generated patch preview, or why it failed.
type patchPreviewMsg struct {
	preview string
	err     error
}

// patchCheckedMsg reports whether the patch could be generated before it is
// handed off for sending.
type patchCheckedMsg struct {
	send SendPatchMsg
	err  error
}

// PatchSend is a TUI form for sending git patches via email.
// The user specifies a local repository, commit range, and recipient,
// and the patch is generated via git format-patch and sent via SMTP.
type PatchSend struct {
	focusIndex      int
	repoInput       textinput.Model
	commitInput     textinput.Model
	toInput         textinput.Model
	ccInput         textinput.Model
	accounts        []config.Account
	selectedAccount int
	width           int
	height          int
	errMsg          string
	working         bool // a git run is in flight
	preview         viewport.Model
	showPreview     bool
}

// NewPatchSend creates a new patch send form with the given accounts.
func NewPatchSend(accounts []config.Account) *PatchSend {
	tiStyles := ThemedTextInputStyles()

	newInput := func(placeholder, prompt string) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.Prompt = prompt
		t.CharLimit = 512
		t.SetStyles(tiStyles)
		return t
	}

	repo := newInput("/path/to/repo", "Repo: ")
	if cwd, err := os.Getwd(); err == nil {
		repo.SetValue(cwd)
	}

	commit := newInput("HEAD~1..HEAD", "Range: ")
	to := newInput("recipient@example.com", "To: ")
	cc := newInput("cc@example.com (optional)", "Cc: ")

	repo.Focus()

	return &PatchSend{
		repoInput:   repo,
		commitInput: commit,
		toInput:     to,
		ccInput:     cc,
		accounts:    accounts,
		preview:     viewport.New(),
	}
}

func (m *PatchSend) Init() tea.Cmd {
	return textinput.Blink
}

func (m *PatchSend) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		for _, inp := range m.inputs() {
			inp.SetWidth(max(msg.Width-lipgloss.Width(inp.Prompt)-1, 1))
		}
		m.sizePreview()
		return m, nil

	case patchPreviewMsg:
		m.working = false
		if msg.err != nil {
			m.errMsg = fmt.Sprintf("Error generating patch: %v", msg.err)
			return m, nil
		}
		m.errMsg = ""
		m.preview.SetContent(msg.preview)
		m.preview.GotoTop()
		m.showPreview = true
		return m, nil

	case patchCheckedMsg:
		m.working = false
		if msg.err != nil {
			m.errMsg = fmt.Sprintf("Error generating patch: %v", msg.err)
			return m, nil
		}
		send := msg.send
		return m, func() tea.Msg { return send }

	case tea.MouseWheelMsg:
		if m.showPreview {
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		}

	case tea.KeyPressMsg:
		if m.showPreview {
			if msg.String() == config.Keybinds.Global.Cancel {
				m.showPreview = false
				return m, nil
			}
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		}
		if handled, cmd := m.handleFormKey(msg); handled {
			return m, cmd
		}
	}

	var cmd tea.Cmd
	switch m.focusIndex {
	case psFocusRepo:
		m.repoInput, cmd = m.repoInput.Update(msg)
	case psFocusCommitRange:
		m.commitInput, cmd = m.commitInput.Update(msg)
	case psFocusTo:
		m.toInput, cmd = m.toInput.Update(msg)
	case psFocusCc:
		m.ccInput, cmd = m.ccInput.Update(msg)
	}
	return m, cmd
}

// handleFormKey handles navigation and actions; it reports false when the
// key should be typed into the focused input. Navigation deliberately uses
// non-printable keys only, so every letter can be typed into the fields.
func (m *PatchSend) handleFormKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case config.Keybinds.Global.Cancel:
		return true, func() tea.Msg { return BackToInboxMsg{} }
	case keyDown, "tab":
		m.moveFocus(1)
		return true, nil
	case "up", keyShiftTab:
		m.moveFocus(-1)
		return true, nil
	case keyLeft, keyRight:
		if m.focusIndex == psFocusFrom && len(m.accounts) > 1 {
			delta := 1
			if msg.String() == keyLeft {
				delta = -1
			}
			m.selectedAccount = (m.selectedAccount + delta + len(m.accounts)) % len(m.accounts)
			return true, nil
		}
	case keyEnter:
		if m.focusIndex == psFocusSend {
			return true, m.sendPatch()
		}
		m.moveFocus(1)
		return true, nil
	case "ctrl+p":
		return true, m.previewPatch()
	}
	m.errMsg = ""
	return false, nil
}

func (m *PatchSend) inputs() []*textinput.Model {
	return []*textinput.Model{&m.repoInput, &m.commitInput, &m.toInput, &m.ccInput}
}

// focusOrder lists the focusable elements in tab order.
func (m *PatchSend) focusOrder() []int {
	order := []int{psFocusRepo, psFocusCommitRange, psFocusTo, psFocusCc, psFocusSend}
	if len(m.accounts) > 1 {
		order = append([]int{psFocusFrom}, order...)
	}
	return order
}

func (m *PatchSend) moveFocus(delta int) {
	order := m.focusOrder()
	pos := slices.Index(order, m.focusIndex)
	if pos < 0 {
		pos = 0
	} else {
		pos = (pos + delta + len(order)) % len(order)
	}
	m.focusIndex = order[pos]
	m.updateFocus()
}

func (m *PatchSend) updateFocus() {
	for i, inp := range m.inputs() {
		if i == m.focusIndex {
			inp.Focus()
		} else {
			inp.Blur()
		}
	}
}

// sizePreview fits the preview viewport inside the bordered box of the
// preview screen.
func (m *PatchSend) sizePreview() {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		w, h = 80, 24
	}
	frame := patchPreviewStyle()
	// title + blank above the box, blank + help below it
	m.preview.SetWidth(max(w-frame.GetHorizontalFrameSize(), 1))
	m.preview.SetHeight(max(h-frame.GetVerticalFrameSize()-4, 1))
}

func patchPreviewStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ActiveTheme.AccentDark).
		Padding(0, 1)
}

func (m *PatchSend) previewPatch() tea.Cmd {
	if m.working {
		return nil
	}
	repoDir := strings.TrimSpace(m.repoInput.Value())
	commitRange := strings.TrimSpace(m.commitInput.Value())
	if repoDir == "" || commitRange == "" {
		m.errMsg = "Repository and commit range are required"
		return nil
	}
	m.working = true
	m.errMsg = ""
	m.sizePreview()
	return func() tea.Msg {
		raw, err := gitmail.GeneratePatch(repoDir, commitRange)
		if err != nil {
			return patchPreviewMsg{err: err}
		}
		preview := string(raw)
		if len(preview) > patchPreviewLimit {
			// Cut on a rune boundary so the preview never ends in a
			// broken UTF-8 sequence.
			preview = strings.ToValidUTF8(preview[:patchPreviewLimit], "") + "\n... (truncated)"
		}
		// Tabs would make the terminal's width differ from the measured one.
		return patchPreviewMsg{preview: strings.ReplaceAll(preview, "\t", "    ")}
	}
}

func (m *PatchSend) sendPatch() tea.Cmd {
	if m.working {
		return nil
	}
	repoDir := strings.TrimSpace(m.repoInput.Value())
	commitRange := strings.TrimSpace(m.commitInput.Value())
	to := strings.TrimSpace(m.toInput.Value())
	cc := strings.TrimSpace(m.ccInput.Value())

	if repoDir == "" {
		m.errMsg = "Repository path is required"
		return nil
	}
	if commitRange == "" {
		m.errMsg = "Commit range is required"
		return nil
	}
	if to == "" {
		m.errMsg = "Recipient (To) is required"
		return nil
	}

	absRepo, err := filepath.Abs(repoDir)
	if err != nil {
		m.errMsg = fmt.Sprintf("Invalid repo path: %v", err)
		return nil
	}

	accountID := ""
	if m.selectedAccount < len(m.accounts) {
		accountID = m.accounts[m.selectedAccount].ID
	}

	send := SendPatchMsg{
		To:          to,
		Cc:          cc,
		RepoDir:     absRepo,
		CommitRange: commitRange,
		AccountID:   accountID,
	}
	m.working = true
	m.errMsg = ""
	// Generate the patch once up front: git's error (bad repo or range) is
	// shown in the form instead of failing later in the background.
	return func() tea.Msg {
		_, err := gitmail.GeneratePatch(absRepo, commitRange)
		return patchCheckedMsg{send: send, err: err}
	}
}

// wrapToWidth word-wraps s to the window width (no-op while it is unknown).
func (m *PatchSend) wrapToWidth(s string) string {
	if m.width <= 0 {
		return s
	}
	return lipgloss.Wrap(s, m.width, "")
}

func (m *PatchSend) View() tea.View {
	t := theme.ActiveTheme

	if m.showPreview {
		return m.previewView()
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(t.AccentText).
		Background(t.AccentDark).
		Padding(0, 1).
		Render(" 📮 Send Git Patch ")

	b.WriteString(title)
	b.WriteString("\n\n")

	// Account selector
	if len(m.accounts) > 0 {
		acc := &m.accounts[min(m.selectedAccount, len(m.accounts)-1)]
		from := "From: " + acc.FormatFromHeader()
		fromStyle := lipgloss.NewStyle().Foreground(t.Secondary)
		if m.focusIndex == psFocusFrom {
			from = "From: ‹ " + acc.FormatFromHeader() + " ›"
			fromStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
		}
		b.WriteString(fromStyle.Render(m.wrapToWidth(from)))
		b.WriteString("\n\n")
	}

	// Form fields
	for _, inp := range m.inputs() {
		b.WriteString(inp.View())
		b.WriteString("\n\n")
	}

	// Send button
	buttonStyle := lipgloss.NewStyle().Foreground(t.Secondary)
	if m.focusIndex == psFocusSend {
		buttonStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	}
	b.WriteString(buttonStyle.Render("[ Send Patch ]"))
	b.WriteString("\n\n")

	if m.working {
		b.WriteString(lipgloss.NewStyle().Foreground(t.Secondary).PaddingLeft(2).Render("Generating patch…"))
		b.WriteString("\n\n")
	}

	// Error message
	if m.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(t.Danger).PaddingLeft(2)
		errText := "✗ " + m.errMsg
		if m.width > 2 {
			errText = lipgloss.Wrap(errText, m.width-2, "")
		}
		b.WriteString(errStyle.Render(errText))
		b.WriteString("\n\n")
	}

	// Help
	items := []helpEntry{
		helpItem("", joinKeys("tab", navArrowsLabel()), "navigate"),
		helpItem("", keyEnter, "send"),
		helpItem("", "ctrl+p", "preview"),
		helpItem("", cancelKey(), "cancel"),
	}
	if m.focusIndex == psFocusFrom {
		items = append(items, helpItem("", "←/→", "account"))
	}
	b.WriteString(helpStyle.Render(m.wrapToWidth(joinHelp(items...))))

	return tea.NewView(clampRendered(b.String(), m.width, m.height))
}

func (m *PatchSend) previewView() tea.View {
	t := theme.ActiveTheme
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(t.AccentText).
		Background(t.AccentDark).
		Padding(0, 1).
		Render(" 📮 Patch Preview ")

	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(patchPreviewStyle().Render(m.preview.View()))
	b.WriteString("\n\n")

	help := helpStyle.Render(m.wrapToWidth(joinHelp(
		helpItem("", navArrowsLabel(), "scroll"),
		helpItem("", cancelKey(), "back to form"),
	)))
	b.WriteString(help)

	return tea.NewView(clampRendered(b.String(), m.width, m.height))
}
