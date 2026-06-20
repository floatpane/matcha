package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

// ReplySplitView shows the original email alongside the composer when replying.
//
// Layout mirrors the FolderInbox split-pane convention so Kitty images never
// overflow into the composer:
//
//	horizontal → composer LEFT  | email RIGHT   (images overflow off right edge)
//	vertical   → composer TOP   | email BOTTOM  (images overflow off bottom edge)
type ReplySplitView struct {
	emailView     *EmailView
	composer      *Composer
	focusComposer bool // true = composer focused (default); false = email view focused
	orientation   string
	width, height int
}

// NewReplySplitView creates a split view for replying to an email.
func NewReplySplitView(
	email fetcher.Email,
	composer *Composer,
	orientation string,
	disableImages bool,
	width, height int,
) *ReplySplitView {
	m := &ReplySplitView{
		composer:      composer,
		orientation:   orientation,
		focusComposer: true,
		width:         width,
		height:        height,
	}

	var ew, eh, colOff, rowOff int
	if orientation == config.SplitPaneVertical {
		// Email is in the BOTTOM half — composer height + 1 border row above it.
		ew = width
		eh = m.emailHeight()
		colOff = 2 // left border/padding of the pane
		rowOff = m.composerHeight() + 1
	} else {
		// Email is on the RIGHT — composer width + 2 for right-border/padding.
		ew = m.emailWidth()
		eh = height - 1
		colOff = m.composerWidth() + 2
		rowOff = 0
	}
	m.emailView = NewEmailViewPreview(email, ew, eh, colOff, rowOff, disableImages)
	return m
}

// Composer exposes the inner Composer so main.go can call composer-specific
// methods without knowing about ReplySplitView.
func (m *ReplySplitView) Composer() *Composer { return m.composer }

func (m *ReplySplitView) Init() tea.Cmd {
	return tea.Batch(m.emailView.Init(), m.composer.Init())
}

func (m *ReplySplitView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	kb := config.Keybinds

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.orientation == config.SplitPaneVertical {
			// Resize composer (top half).
			composerMsg := tea.WindowSizeMsg{Width: m.width, Height: m.composerHeight()}
			var cmd tea.Cmd
			_, cmd = m.composer.Update(composerMsg)
			// Resize email view (bottom half) and update its row offset.
			emailMsg := tea.WindowSizeMsg{Width: m.width, Height: m.emailHeight()}
			m.emailView.Update(emailMsg)
			m.emailView.rowOffset = m.composerHeight() + 1
			m.emailView.columnOffset = 2
			return m, cmd
		}
		// Horizontal: resize composer (left) and email (right).
		composerMsg := tea.WindowSizeMsg{Width: m.composerWidth() - 2, Height: msg.Height - 1}
		var cmd tea.Cmd
		_, cmd = m.composer.Update(composerMsg)
		emailMsg := tea.WindowSizeMsg{Width: m.emailWidth() - 2, Height: msg.Height - 2}
		m.emailView.Update(emailMsg)
		m.emailView.columnOffset = m.composerWidth() + 2
		return m, cmd

	case tea.KeyPressMsg:
		s := msg.String()
		if m.focusComposer {
			// [ switches focus to the email view.
			if s == kb.Folder.FocusInbox {
				m.focusComposer = false
				return m, nil
			}
			var cmd tea.Cmd
			_, cmd = m.composer.Update(msg)
			return m, cmd
		}
		// Email view is focused — ] switches back to composer.
		if s == kb.Folder.FocusPreview {
			m.focusComposer = true
			return m, nil
		}
		var cmd tea.Cmd
		_, cmd = m.emailView.Update(msg)
		return m, cmd
	}

	// All other messages (spellcheck results, draft autosave, etc.) go to composer.
	var cmd tea.Cmd
	_, cmd = m.composer.Update(msg)
	return m, cmd
}

func (m *ReplySplitView) View() tea.View {
	emailFocused := !m.focusComposer
	composerBorderColor := unfocusedBorderColor
	emailBorderColor := unfocusedBorderColor
	if emailFocused {
		emailBorderColor = focusedBorderColor
	} else {
		composerBorderColor = focusedBorderColor
	}

	if m.orientation == config.SplitPaneVertical {
		// Composer TOP, Email BOTTOM.
		cw := m.width
		ch := m.composerHeight()
		ew := m.width
		eh := m.emailHeight()

		composerPane := inboxPaneVerticalStyle.
			BorderForeground(composerBorderColor).
			Width(cw).
			Height(ch).
			Render(m.composer.View().Content)

		emailPane := previewPaneVerticalStyle.
			BorderForeground(emailBorderColor).
			Width(ew).
			Height(eh).
			Render(m.emailView.View().Content)

		content := lipgloss.JoinVertical(lipgloss.Left, composerPane, emailPane)
		return tea.NewView(content)
	}

	// Horizontal: Composer LEFT, Email RIGHT.
	cw := m.composerWidth()
	ew := m.emailWidth()
	avail := m.height - 1

	composerPane := inboxPaneStyle.
		BorderForeground(composerBorderColor).
		Width(cw).
		Height(avail).
		Render(m.composer.View().Content)

	emailPane := previewPaneStyle.
		BorderForeground(emailBorderColor).
		Width(ew).
		Height(avail).
		Render(m.emailView.View().Content)

	content := lipgloss.JoinHorizontal(lipgloss.Top, composerPane, emailPane)
	return tea.NewView(content)
}

// composerWidth is the composer pane content width in horizontal split.
func (m *ReplySplitView) composerWidth() int {
	w := m.width / 2
	if w < 40 {
		w = 40
	}
	return w
}

// emailWidth is the email pane content width in horizontal split.
func (m *ReplySplitView) emailWidth() int {
	w := m.width - m.composerWidth()
	if w < 30 {
		w = 30
	}
	return w
}

// composerHeight is the composer pane content height in vertical split (60%).
func (m *ReplySplitView) composerHeight() int {
	h := int(float64(m.height) * 0.6)
	if h < 5 {
		h = 5
	}
	return h
}

// emailHeight is the email pane content height in vertical split.
func (m *ReplySplitView) emailHeight() int {
	h := m.height - m.composerHeight() - 1 // 1 border row between panes
	if h < 5 {
		h = 5
	}
	return h
}
