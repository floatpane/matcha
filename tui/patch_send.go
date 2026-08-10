package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
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
)

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
	preview         string
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
	}
}

func (m *PatchSend) Init() tea.Cmd {
	return textinput.Blink
}

func (m *PatchSend) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case config.Keybinds.Global.Cancel:
			if m.showPreview {
				m.showPreview = false
				return m, nil
			}
			return m, func() tea.Msg { return BackToInboxMsg{} }

		case config.Keybinds.Global.NavDown, "tab":
			m.nextField()
			return m, nil

		case config.Keybinds.Global.NavUp, "shift+tab":
			m.prevField()
			return m, nil

		case "enter":
			if m.focusIndex == psFocusSend {
				return m, m.sendPatch()
			}
			m.nextField()
			return m, nil

		case "ctrl+p":
			m.previewPatch()
			return m, nil

		default:
			m.errMsg = ""
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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

func (m *PatchSend) nextField() {
	m.focusIndex++
	if m.focusIndex > psFocusSend {
		m.focusIndex = psFocusRepo
	}
	m.updateFocus()
}

func (m *PatchSend) prevField() {
	m.focusIndex--
	if m.focusIndex < psFocusRepo {
		m.focusIndex = psFocusSend
	}
	m.updateFocus()
}

func (m *PatchSend) updateFocus() {
	inputs := []*textinput.Model{&m.repoInput, &m.commitInput, &m.toInput, &m.ccInput}
	for i, inp := range inputs {
		if i == m.focusIndex {
			inp.Focus()
		} else {
			inp.Blur()
		}
	}
}

func (m *PatchSend) previewPatch() {
	repoDir := strings.TrimSpace(m.repoInput.Value())
	commitRange := strings.TrimSpace(m.commitInput.Value())
	if repoDir == "" || commitRange == "" {
		m.errMsg = "Repository and commit range are required"
		return
	}

	raw, err := gitmail.GeneratePatch(repoDir, commitRange)
	if err != nil {
		m.errMsg = fmt.Sprintf("Error generating patch: %v", err)
		return
	}

	// Truncate preview for display
	preview := string(raw)
	if len(preview) > 2000 {
		preview = preview[:2000] + "\n... (truncated)"
	}
	m.preview = preview
	m.showPreview = true
	m.errMsg = ""
}

func (m *PatchSend) sendPatch() tea.Cmd {
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
	if len(m.accounts) > 0 {
		accountID = m.accounts[0].ID
	}

	return func() tea.Msg {
		return SendPatchMsg{
			To:          to,
			Cc:          cc,
			RepoDir:     absRepo,
			CommitRange: commitRange,
			AccountID:   accountID,
		}
	}
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
		acc := m.accounts[0]
		if len(m.accounts) > 1 {
			acc = m.accounts[m.selectedAccount]
		}
		fromStyle := lipgloss.NewStyle().Foreground(t.Secondary)
		b.WriteString(fromStyle.Render(fmt.Sprintf("From: %s <%s>", acc.Name, acc.SendAsEmail)))
		b.WriteString("\n\n")
	}

	// Form fields
	b.WriteString(m.repoInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.commitInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.toInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.ccInput.View())
	b.WriteString("\n\n")

	// Send button
	buttonStyle := lipgloss.NewStyle().Foreground(t.Secondary)
	if m.focusIndex == psFocusSend {
		buttonStyle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	}
	b.WriteString(buttonStyle.Render("[ Send Patch ]"))
	b.WriteString("\n\n")

	// Error message
	if m.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(t.Danger).PaddingLeft(2)
		b.WriteString(errStyle.Render("✗ " + m.errMsg))
		b.WriteString("\n\n")
	}

	// Help
	help := helpStyle.Render("tab/↓/↑: navigate • enter: send • ctrl+p: preview • esc: cancel")
	b.WriteString(help)

	return tea.NewView(b.String())
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

	previewStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.AccentDark).
		Padding(0, 1)

	b.WriteString(previewStyle.Render(m.preview))
	b.WriteString("\n\n")

	help := helpStyle.Render("esc: back to form")
	b.WriteString(help)

	return tea.NewView(b.String())
}
