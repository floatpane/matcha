package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

// replySplitSwitchKey moves focus between the composer and the original
// email. It must not be a printable key: the composer is a text editor, and a
// printable switch key (the old "[") made typing "[d" delete the email.
const replySplitSwitchKey = "ctrl+]"

// ReplySplitView shows the original email alongside the composer when replying.
//
//	horizontal → composer LEFT  | email RIGHT
//	vertical   → composer TOP   | email BOTTOM
//
// The email pane is read-only: it scrolls and can toggle images, but its
// actions (delete, archive, reply, …) are disabled so keystrokes meant for
// the reply can never act on the original message.
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
	r := m.emailContentRect()
	m.emailView = NewEmailViewPreview(email, r.w, r.h, r.x, r.y, disableImages)
	m.emailView.readOnly = true
	return m
}

// Composer exposes the inner Composer so main.go can call composer-specific
// methods without knowing about ReplySplitView.
func (m *ReplySplitView) Composer() *Composer { return m.composer }

func (m *ReplySplitView) Init() tea.Cmd {
	return tea.Batch(m.emailView.Init(), m.composer.Init())
}

func (m *ReplySplitView) isVertical() bool {
	return m.orientation == config.SplitPaneVertical
}

// composerRect is the composer pane (it has no border of its own).
func (m *ReplySplitView) composerRect() paneRect {
	w, h := max(1, m.width), max(1, m.height)
	if m.isVertical() {
		return paneRect{x: 0, y: 0, w: w, h: replySplitFirstPane(h, 8, 5)}
	}
	return paneRect{x: 0, y: 0, w: replySplitFirstPane(w, 40, 20), h: h}
}

// emailPaneRect is the email pane, including the separator border.
func (m *ReplySplitView) emailPaneRect() paneRect {
	c := m.composerRect()
	w, h := max(1, m.width), max(1, m.height)
	if m.isVertical() {
		return paneRect{x: 0, y: c.h, w: w, h: max(1, h-c.h)}
	}
	return paneRect{x: c.w, y: 0, w: max(1, w-c.w), h: h}
}

// emailContentRect is where the email view is drawn inside its pane.
func (m *ReplySplitView) emailContentRect() paneRect {
	p := m.emailPaneRect()
	if m.isVertical() {
		top := previewPaneVerticalStyle.GetBorderTopSize()
		return paneRect{x: p.x, y: p.y + top, w: p.w, h: max(1, p.h-top)}
	}
	left := previewPaneStyle.GetBorderLeftSize() + previewPaneStyle.GetPaddingLeft()
	return paneRect{x: p.x + left, y: p.y, w: max(1, p.w-left), h: p.h}
}

// replySplitFirstPane sizes the composer: 60% when vertical, 50% when
// horizontal, but at least minFirst while leaving minSecond for the email.
func replySplitFirstPane(total, minFirst, minSecond int) int {
	size := total / 2
	if minFirst == 8 { // vertical: give the composer a bit more room
		size = total * 3 / 5
	}
	size = max(size, minFirst)
	if total-size < minSecond {
		size = total - minSecond
	}
	return max(1, min(size, total-1))
}

func (m *ReplySplitView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case remoteImagesLoadedMsg, LoadRemoteImagesMsg, ImageProtocolChangedMsg:
		_, cmd := m.emailView.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		c := m.composerRect()
		_, cmd := m.composer.Update(tea.WindowSizeMsg{Width: c.w, Height: c.h})
		r := m.emailContentRect()
		m.emailView.SetColumnOffset(r.x)
		m.emailView.SetRowOffset(r.y)
		_, ecmd := m.emailView.Update(tea.WindowSizeMsg{Width: r.w, Height: r.h})
		return m, tea.Batch(cmd, ecmd)

	case tea.MouseWheelMsg:
		if m.emailPaneRect().contains(msg.X, msg.Y) {
			var cmd tea.Cmd
			m.emailView.viewport, cmd = m.emailView.viewport.Update(msg)
			return m, cmd
		}
		_, cmd := m.composer.Update(msg)
		return m, cmd

	case tea.MouseClickMsg:
		if m.emailPaneRect().contains(msg.X, msg.Y) {
			m.focusComposer = false
			return m, nil
		}
		m.focusComposer = true
		_, cmd := m.composer.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		s := msg.String()
		kb := config.Keybinds
		isSwitch := s == replySplitSwitchKey ||
			(msg.Text == "" && (s == kb.Folder.FocusInbox || s == kb.Folder.FocusPreview))
		if m.focusComposer {
			if isSwitch {
				m.focusComposer = false
				return m, nil
			}
			_, cmd := m.composer.Update(msg)
			return m, cmd
		}
		// Email pane focused: it only scrolls and toggles images.
		switch s {
		case kb.Global.Cancel, kb.Folder.FocusInbox, kb.Folder.FocusPreview:
			m.focusComposer = true
			return m, nil
		case kb.Email.ToggleImages, m.emailView.loadRemoteImagesKey():
			_, cmd := m.emailView.Update(msg)
			return m, cmd
		}
		if isSwitch {
			m.focusComposer = true
			return m, nil
		}
		var cmd tea.Cmd
		m.emailView.viewport, cmd = m.emailView.viewport.Update(msg)
		return m, cmd
	}

	// All other messages (spellcheck results, draft autosave, etc.) go to composer.
	_, cmd := m.composer.Update(msg)
	return m, cmd
}

func (m *ReplySplitView) View() tea.View {
	emailBorderColor := unfocusedBorderColor
	if !m.focusComposer {
		emailBorderColor = focusedBorderColor
	}

	c := m.composerRect()
	r := m.emailContentRect()
	composerPane := fitBox(m.composer.View().Content, c.w, c.h)
	emailBody := fitBox(m.emailView.View().Content, r.w, r.h)

	if m.isVertical() {
		emailPane := previewPaneVerticalStyle.BorderForeground(emailBorderColor).Render(emailBody)
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, composerPane, emailPane))
	}
	emailPane := previewPaneStyle.BorderForeground(emailBorderColor).Render(emailBody)
	return tea.NewView(lipgloss.JoinHorizontal(lipgloss.Top, composerPane, emailPane))
}

// VisibleImages reports the original email's out-of-band images.
func (m *ReplySplitView) VisibleImages() []ScreenImage {
	return m.emailView.VisibleImages()
}
