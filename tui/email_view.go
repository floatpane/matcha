package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	calendar "github.com/floatpane/go-icalendar"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/theme"
	"github.com/floatpane/matcha/view"
	"github.com/floatpane/termimage"
	"github.com/floatpane/termimage/detect"
)

// ClearKittyGraphics clears any rendered image residue using termimage's
// protocol-aware clear (Kitty deletes all placements, Sixel/HalfBlock erases
// the trailing rows). The name is kept for callsite compatibility.
//
// termimage.Auto is not handled by termimage.Clear (it falls to the rows-based
// branch and no-ops when rows=0), so resolve the protocol up front.
func ClearKittyGraphics() {
	proto := detect.Best()
	if err := termimage.Clear(os.Stdout, proto, 0); err != nil {
		return
	}
	os.Stdout.Sync() //nolint:errcheck,gosec
}

var (
	emailHeaderStyle   = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).Padding(0, 1)
	attachmentBoxStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).PaddingLeft(2).MarginTop(1)
)

// BodyTransformer, if set, post-processes the rendered email body before it is
// placed in the viewport. main.go wires this up to the plugin manager so that
// plugins registered on the "email_body_render" hook can rewrite, recolor, or
// remove parts of the displayed body.
var BodyTransformer func(body string, email fetcher.Email) string

func applyBodyTransform(body string, email fetcher.Email) string {
	if BodyTransformer == nil {
		return body
	}
	return BodyTransformer(body, email)
}

type EmailView struct {
	viewport           viewport.Model
	email              fetcher.Email
	emailIndex         int
	attachmentCursor   int
	focusOnAttachments bool
	accountID          string
	mailbox            MailboxKind
	disableImages      bool
	showImages         bool
	isSMIME            bool
	smimeTrusted       bool
	isEncrypted        bool
	isPGP              bool
	pgpTrusted         bool
	isPGPEncrypted     bool
	imagePlacements    []view.ImagePlacement
	pluginStatus       string
	pluginKeyBindings  []PluginKeyBinding
	hasCalendarInvite  bool
	calendarEvent      *calendar.Event
	originalICSData    []byte
	isPreviewMode      bool
	columnOffset       int // horizontal offset for image rendering in split pane
	rowOffset          int // vertical offset for image rendering in split pane (vertical layout)
	isPatch            bool
	patchInfo          *view.PatchInfo
}

func NewEmailView(email fetcher.Email, emailIndex, width, height int, mailbox MailboxKind, disableImages bool) *EmailView {
	isSMIME := false
	smimeTrusted := false
	isEncrypted := false
	isPGP := false
	pgpTrusted := false
	isPGPEncrypted := false
	var filteredAtts []fetcher.Attachment
	var calendarEvent *calendar.Event
	var originalICSData []byte

	for _, att := range email.Attachments {
		if att.Filename == "smime-status.internal" { //nolint:gocritic
			isSMIME = att.IsSMIMESignature || att.IsSMIMEEncrypted
			smimeTrusted = att.SMIMEVerified
			isEncrypted = att.IsSMIMEEncrypted
		} else if att.IsSMIMESignature || att.Filename == "smime.p7s" || att.Filename == "smime.p7m" || strings.HasPrefix(att.MIMEType, "application/pkcs7") {
			// Extract S/MIME status from detached signature attachments
			if att.IsSMIMESignature && !isSMIME {
				isSMIME = true
				smimeTrusted = att.SMIMEVerified
			}
			// Skip UI rendering
		} else if att.Filename == "pgp-status.internal" {
			isPGP = att.IsPGPSignature || att.IsPGPEncrypted
			pgpTrusted = att.PGPVerified
			isPGPEncrypted = att.IsPGPEncrypted
		} else if att.IsPGPSignature || att.Filename == "signature.asc" || att.MIMEType == "application/pgp-signature" || att.MIMEType == "application/pgp-encrypted" {
			// Extract PGP status from detached signature attachments
			if att.IsPGPSignature && !isPGP {
				isPGP = true
				pgpTrusted = att.PGPVerified
			}
			// Skip UI rendering
		} else if att.IsCalendarInvite {
			// Parse calendar invite if not already parsed
			if len(att.Data) > 0 && calendarEvent == nil {
				if event, err := calendar.ParseICS(att.Data); err == nil {
					calendarEvent = event
					originalICSData = att.Data
				}
			}
			// Don't show .ics in regular attachment list
		} else {
			filteredAtts = append(filteredAtts, att)
		}
	}
	email.Attachments = filteredAtts

	// Pass the styles from the tui package to the view package
	inlineImages := inlineImagesFromAttachments(email.Attachments)

	// Initial state for showImages matches config unless overridden later
	showImages := !disableImages

	// Detect git patches and render with diff syntax highlighting.
	patchInfo := view.DetectPatch(email.Body, email.BodyMIMEType, email.Subject, email.From)
	isPatch := patchInfo != nil

	body, placements, err := view.ProcessBodyWithInline(email.Body, email.BodyMIMEType, inlineImages, H1Style, H2Style, BodyStyle, !showImages)
	if err != nil {
		body = fmt.Sprintf("Error rendering body: %v", err)
	}
	body = applyBodyTransform(body, email)

	// If a patch was detected, replace the body with the patch-rendered version
	// (commit message + highlighted diff + metadata banner).
	if isPatch {
		// The viewport will be created below with this width; pass it in so the
		// diff code box can be sized to fit.
		patchBody, ok := view.RenderPatchBody(email.Body, email.BodyMIMEType, email.Subject, email.From, width)
		if ok {
			body = applyBodyTransform(patchBody, email)
		}
	}

	// Create header and compute heights that reduce viewport space.
	header := fmt.Sprintf("From: %s\nSubject: %s", email.From, email.Subject)
	headerHeight := lipgloss.Height(header) + 2

	attachmentHeight := 0
	if len(email.Attachments) > 0 {
		attachmentHeight = len(email.Attachments) + 2
	}

	// Account for calendar card height
	calendarHeight := 0
	if calendarEvent != nil {
		calendarHeight = 10 // Approximate height for calendar card
	}

	// Build viewport with initial size and set wrapped content.
	vp := viewport.New()
	vp.SetWidth(width)
	vp.SetHeight(height - headerHeight - attachmentHeight - calendarHeight)
	wrapped := wrapBodyToWidth(body, vp.Width())
	vp.SetContent(wrapped + "\n")

	return &EmailView{
		viewport:          vp,
		email:             email,
		emailIndex:        emailIndex,
		accountID:         email.AccountID,
		mailbox:           mailbox,
		disableImages:     disableImages,
		showImages:        showImages,
		isSMIME:           isSMIME,
		smimeTrusted:      smimeTrusted,
		isEncrypted:       isEncrypted,
		isPGP:             isPGP,
		pgpTrusted:        pgpTrusted,
		isPGPEncrypted:    isPGPEncrypted,
		imagePlacements:   placements,
		hasCalendarInvite: calendarEvent != nil,
		calendarEvent:     calendarEvent,
		originalICSData:   originalICSData,
		isPreviewMode:     false,
		isPatch:           isPatch,
		patchInfo:         patchInfo,
	}
}

// NewEmailViewPreview creates EmailView in preview mode with column and row
// offsets for out-of-band image rendering. rowOffset is non-zero in vertical
// split layouts where the preview sits below the inbox pane.
func NewEmailViewPreview(email fetcher.Email, width, height, colOffset, rowOffset int, disableImages bool) *EmailView {
	ev := NewEmailView(email, 0, width, height, MailboxInbox, disableImages)
	ev.isPreviewMode = true
	ev.columnOffset = colOffset
	ev.rowOffset = rowOffset
	return ev
}

// SetRowOffset updates the vertical offset used for out-of-band image rendering.
// Call this after a window resize in vertical split mode so images stay aligned
// with the preview pane's new position.
func (m *EmailView) SetRowOffset(offset int) {
	m.rowOffset = offset
}

// SetColumnOffset updates the horizontal offset used for out-of-band image
// rendering. Call this after a layout change so images stay aligned with the
// preview pane's new horizontal position.
func (m *EmailView) SetColumnOffset(offset int) {
	m.columnOffset = offset
}

func (m *EmailView) Init() tea.Cmd {
	return nil
}

// handleComposeAction checks if the key matches reply, reply-all, or forward
// and returns the corresponding message. Returns false if no match.
func (m *EmailView) handleComposeAction(key string) (tea.Msg, bool) {
	kb := config.Keybinds
	switch key {
	case kb.Email.Reply:
		ClearKittyGraphics()
		return ReplyToEmailMsg{Email: m.email}, true
	case kb.Email.ReplyAll:
		ClearKittyGraphics()
		return ReplyAllEmailMsg{Email: m.email}, true
	case kb.Email.Forward:
		ClearKittyGraphics()
		return ForwardEmailMsg{Email: m.email}, true
	}
	return nil, false
}

func (m *EmailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	cmds := make([]tea.Cmd, 0, 1)

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if handled, handledCmd := m.handleKeyPress(msg); handled {
			return handledCmd()
		}
	case tea.WindowSizeMsg:
		m.handleWindowSize(msg)
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleKeyPress processes a key press in the email view. It returns a non-nil
// cmd function when the key was handled and the caller should return its
// result immediately.
func (m *EmailView) handleKeyPress(msg tea.KeyPressMsg) (bool, func() (tea.Model, tea.Cmd)) {
	kb := config.Keybinds
	if msg.String() == kb.Global.Cancel {
		if m.focusOnAttachments {
			m.focusOnAttachments = false
			return true, func() (tea.Model, tea.Cmd) { return m, nil }
		}
		ClearKittyGraphics()
		return true, func() (tea.Model, tea.Cmd) {
			return m, func() tea.Msg { return BackToMailboxMsg{Mailbox: m.mailbox} }
		}
	}

	if m.focusOnAttachments {
		return m.handleAttachmentFocusKey(msg)
	}
	return m.handleEmailBodyKey(msg)
}

// handleAttachmentFocusKey handles key presses when the attachment list has
// focus.
func (m *EmailView) handleAttachmentFocusKey(msg tea.KeyPressMsg) (bool, func() (tea.Model, tea.Cmd)) {
	kb := config.Keybinds
	switch msg.String() {
	case "up", kb.Global.NavUp:
		if len(m.email.Attachments) > 0 {
			m.attachmentCursor = (m.attachmentCursor - 1 + len(m.email.Attachments)) % len(m.email.Attachments)
		}
		return true, func() (tea.Model, tea.Cmd) { return m, nil }
	case keyDown, kb.Global.NavDown:
		if len(m.email.Attachments) > 0 {
			m.attachmentCursor = (m.attachmentCursor + 1) % len(m.email.Attachments)
		}
		return true, func() (tea.Model, tea.Cmd) { return m, nil }
	case keyEnter:
		if len(m.email.Attachments) > 0 {
			selected := m.email.Attachments[m.attachmentCursor]
			idx := m.emailIndex
			accountID := m.accountID
			mailbox := m.mailbox
			return true, func() (tea.Model, tea.Cmd) {
				return m, func() tea.Msg {
					return DownloadAttachmentMsg{
						Index:     idx,
						Filename:  selected.Filename,
						PartID:    selected.PartID,
						Data:      selected.Data,
						AccountID: accountID,
						Mailbox:   mailbox,
					}
				}
			}
		}
	case kb.Email.FocusAttachments:
		m.focusOnAttachments = false
	}
	return false, nil
}

// handleEmailBodyKey handles key presses when the email body has focus.
func (m *EmailView) handleEmailBodyKey(msg tea.KeyPressMsg) (bool, func() (tea.Model, tea.Cmd)) {
	kb := config.Keybinds
	switch msg.String() {
	case kb.Email.ToggleImages:
		if view.ImageProtocolSupported() {
			m.showImages = !m.showImages
			ClearKittyGraphics()
			m.regenerateBody()
			return true, func() (tea.Model, tea.Cmd) { return m, nil }
		}
	case kb.Email.Reply, kb.Email.ReplyAll, kb.Email.Forward:
		if composeMsg, ok := m.handleComposeAction(msg.String()); ok {
			return true, func() (tea.Model, tea.Cmd) {
				return m, func() tea.Msg { return composeMsg }
			}
		}
	case kb.Email.Delete:
		accountID := m.accountID
		uid := m.email.UID
		mailbox := m.mailbox
		ClearKittyGraphics()
		return true, func() (tea.Model, tea.Cmd) {
			return m, func() tea.Msg {
				return DeleteEmailMsg{UID: uid, AccountID: accountID, Mailbox: mailbox}
			}
		}
	case kb.Email.Archive:
		accountID := m.accountID
		uid := m.email.UID
		mailbox := m.mailbox
		ClearKittyGraphics()
		return true, func() (tea.Model, tea.Cmd) {
			return m, func() tea.Msg {
				return ArchiveEmailMsg{UID: uid, AccountID: accountID, Mailbox: mailbox}
			}
		}
	case kb.Email.RsvpAccept, kb.Email.RsvpDecline, kb.Email.RsvpTentative:
		if m.hasCalendarInvite && m.calendarEvent != nil {
			response := rsvpResponseFromKey(msg.String(), kb)
			accountID := m.accountID
			originalICS := m.originalICSData
			event := m.calendarEvent
			inReplyTo := m.email.MessageID
			references := m.email.References
			return true, func() (tea.Model, tea.Cmd) {
				return m, func() tea.Msg {
					return SendRSVPMsg{
						OriginalICS: originalICS,
						Event:       event,
						Response:    response,
						AccountID:   accountID,
						InReplyTo:   inReplyTo,
						References:  references,
					}
				}
			}
		}
	case kb.Email.ApplyPatch:
		if m.isPatch && m.patchInfo != nil && m.patchInfo.HasDiff {
			return true, func() (tea.Model, tea.Cmd) {
				return m, func() tea.Msg {
					return ApplyPatchMsg{
						RawEmail:  m.email.Body,
						Subject:   m.email.Subject,
						From:      m.email.From,
						AccountID: m.accountID,
					}
				}
			}
		}
	case kb.Email.SendPatch:
		return true, func() (tea.Model, tea.Cmd) {
			return m, func() tea.Msg { return GoToSendPatchMsg{} }
		}
	case kb.Email.FocusAttachments:
		if len(m.email.Attachments) > 0 {
			m.focusOnAttachments = true
		}
	}
	return false, nil
}

// rsvpResponseFromKey maps an RSVP key to its calendar response string.
func rsvpResponseFromKey(key string, kb config.KeybindsConfig) string {
	switch key {
	case kb.Email.RsvpAccept:
		return "ACCEPTED"
	case kb.Email.RsvpDecline:
		return "DECLINED"
	case kb.Email.RsvpTentative:
		return "TENTATIVE"
	}
	return ""
}

// regenerateBody re-renders the email body after a state change such as
// toggling image display.
func (m *EmailView) regenerateBody() {
	inlineImages := inlineImagesFromAttachments(m.email.Attachments)
	body, placements, err := view.ProcessBodyWithInline(m.email.Body, m.email.BodyMIMEType, inlineImages, H1Style, H2Style, BodyStyle, !m.showImages)
	if err != nil {
		body = fmt.Sprintf("Error rendering body: %v", err)
	}
	body = applyBodyTransform(body, m.email)
	m.imagePlacements = placements
	wrapped := wrapBodyToWidth(body, m.viewport.Width())
	m.viewport.SetContent(wrapped + "\n")
}

// handleWindowSize updates the viewport and re-renders the body when the
// terminal is resized.
func (m *EmailView) handleWindowSize(msg tea.WindowSizeMsg) {
	header := fmt.Sprintf("To: %s\nFrom: %s\nSubject: %s ", strings.Join(m.email.To, ", "), m.email.From, m.email.Subject)
	headerHeight := lipgloss.Height(header) + 2
	attachmentHeight := 0
	if len(m.email.Attachments) > 0 {
		attachmentHeight = len(m.email.Attachments) + 2
	}
	m.viewport.SetWidth(msg.Width)
	m.viewport.SetHeight(msg.Height - headerHeight - attachmentHeight)

	ClearKittyGraphics()
	inlineImages := inlineImagesFromAttachments(m.email.Attachments)
	body, placements, err := view.ProcessBodyWithInline(m.email.Body, m.email.BodyMIMEType, inlineImages, H1Style, H2Style, BodyStyle, !m.showImages)
	if err != nil {
		body = fmt.Sprintf("Error rendering body: %v", err)
	}
	body = applyBodyTransform(body, m.email)
	if m.isPatch {
		if patchBody, ok := view.RenderPatchBody(m.email.Body, m.email.BodyMIMEType, m.email.Subject, m.email.From, m.viewport.Width()); ok {
			body = applyBodyTransform(patchBody, m.email)
		}
	}
	m.imagePlacements = placements
	wrapped := wrapBodyToWidth(body, m.viewport.Width())
	m.viewport.SetContent(wrapped + "\n")
}

func (m *EmailView) View() tea.View {
	os.Stdout.WriteString("\x1b_Ga=d,d=a\x1b\\") //nolint:errcheck,gosec
	os.Stdout.Sync()                             //nolint:errcheck,gosec

	styledHeader := m.renderHeader()
	help := m.renderHelp()
	attachmentView := m.renderAttachmentView()

	if m.showImages && len(m.imagePlacements) > 0 {
		m.renderVisibleImages(styledHeader)
	}

	calendarView := ""
	if m.hasCalendarInvite && m.calendarEvent != nil {
		calendarView = renderCalendarInvite(m.calendarEvent)
	}

	var v tea.View
	if calendarView != "" {
		v = tea.NewView(fmt.Sprintf("%s\n%s\n%s\n%s\n%s", styledHeader, calendarView, m.viewport.View(), attachmentView, help))
	} else {
		v = tea.NewView(fmt.Sprintf("%s\n%s\n%s\n%s", styledHeader, m.viewport.View(), attachmentView, help))
	}
	if config.MouseEnabled != nil && *config.MouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// renderHeader renders the email header bar, including crypto and patch badges.
func (m *EmailView) renderHeader() string {
	var cryptoStatus strings.Builder

	if m.isEncrypted {
		cryptoStatus.WriteString(lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent).Render(" [S/MIME: 🔒 Encrypted]"))
	} else if m.isSMIME {
		if m.smimeTrusted {
			cryptoStatus.WriteString(lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent).Render(" [S/MIME: ✅ Trusted]"))
		} else {
			cryptoStatus.WriteString(lipgloss.NewStyle().Foreground(theme.ActiveTheme.Danger).Render(" [S/MIME: ❌ Untrusted]"))
		}
	}
	if m.isPGPEncrypted {
		cryptoStatus.WriteString(lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent).Render(" [PGP: 🔒 Encrypted]"))
	} else if m.isPGP {
		if m.pgpTrusted {
			cryptoStatus.WriteString(lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent).Render(" [PGP: ✅ Verified]"))
		} else {
			cryptoStatus.WriteString(lipgloss.NewStyle().Foreground(theme.ActiveTheme.Danger).Render(" [PGP: ⚠️ Unverified]"))
		}
	}
	if m.isPatch {
		cryptoStatus.WriteString(lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent).Render(" [📮 Patch]"))
	}

	header := fmt.Sprintf("To: %s | From: %s | Subject: %s%s", strings.Join(m.email.To, ", "), m.email.From, m.email.Subject, cryptoStatus.String())
	return emailHeaderStyle.Width(m.viewport.Width()).Render(header)
}

// renderHelp renders the bottom help bar based on current focus and state.
func (m *EmailView) renderHelp() string {
	if m.focusOnAttachments {
		helpText := "↑/↓: navigate • enter: download • esc/tab: back to email body"
		if m.pluginStatus != "" {
			helpText += " • " + m.pluginStatus
		}
		return helpStyle.Render(helpText)
	}

	var shortcuts strings.Builder
	shortcuts.WriteString("\uf112 r: reply • \uf064 shift+r: reply all • \uf064 f: forward • \uea81 d: delete • \uea98 a: archive • \uf435 tab: focus attachments • \ueb06 esc: back to inbox")
	if m.isPatch && m.patchInfo != nil && m.patchInfo.HasDiff {
		shortcuts.WriteString(" • \uf126 p: apply patch")
	}
	shortcuts.WriteString(" • \uf1d3 P: send patch")
	if view.ImageProtocolSupported() {
		shortcuts.WriteString("• \uf03e i: toggle images")
	}
	for _, pk := range m.pluginKeyBindings {
		shortcuts.WriteString(" • ")
		shortcuts.WriteString(pk.Key)
		shortcuts.WriteString(": ")
		shortcuts.WriteString(pk.Description)
	}
	if m.pluginStatus != "" {
		shortcuts.WriteString(" • ")
		shortcuts.WriteString(m.pluginStatus)
	}
	help := helpStyle.Render(shortcuts.String())
	if m.isPreviewMode {
		help = lipgloss.NewStyle().PaddingLeft(4).Render(help)
	}
	return help
}

// renderAttachmentView renders the attachment list panel.
func (m *EmailView) renderAttachmentView() string {
	if len(m.email.Attachments) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Attachments:\n")
	for i, attachment := range m.email.Attachments {
		cursor := "  "
		style := itemStyle
		if m.focusOnAttachments && i == m.attachmentCursor {
			cursor = "> "
			style = selectedItemStyle
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%s", cursor, attachment.Filename)))
		b.WriteString("\n")
	}
	return attachmentBoxStyle.Render(b.String())
}

// renderVisibleImages writes visible inline image placements directly to
// stdout. Bubbletea v2's ultraviolet renderer uses a cell-based model that
// cannot pass through graphics protocol escape sequences, so we write them
// out-of-band.
func (m *EmailView) renderVisibleImages(styledHeader string) {
	headerLines := lipgloss.Height(styledHeader) + 1 // +1 for the newline after header
	yOffset := m.viewport.YOffset()
	vpHeight := m.viewport.Height()

	for i := range m.imagePlacements {
		p := &m.imagePlacements[i]
		// Only render if the image's top line is within the viewport.
		// We can't partially clip images scrolled off the top (Kitty
		// always renders from the top-left), so we hide them once
		// their start line scrolls above the viewport.
		if p.Line >= yOffset && p.Line < yOffset+vpHeight {
			screenRow := m.rowOffset + headerLines + (p.Line - yOffset)
			if m.columnOffset > 0 {
				view.RenderImageToStdout(p, screenRow, m.columnOffset+1)
			} else {
				view.RenderImageToStdout(p, screenRow)
			}
		}
	}
}

// GetAccountID returns the account ID for this email
func (m *EmailView) GetAccountID() string {
	return m.accountID
}

// GetMailbox returns the mailbox kind for this email view
func (m *EmailView) GetMailbox() MailboxKind {
	return m.mailbox
}

// IsPreviewMode reports whether this EmailView is rendering inside the split
// preview pane rather than full-screen.
func (m *EmailView) IsPreviewMode() bool {
	return m.isPreviewMode
}

// IsPatch returns true if the currently viewed email is a git patch.
func (m *EmailView) IsPatch() bool {
	return m.isPatch
}

// GetPatchInfo returns the parsed patch metadata, or nil if not a patch.
func (m *EmailView) GetPatchInfo() *view.PatchInfo {
	return m.patchInfo
}

// SetPluginStatus sets a persistent status string from plugins, shown in the help bar.
func (m *EmailView) SetPluginStatus(status string) {
	m.pluginStatus = status
}

// SetPluginKeyBindings sets the plugin-registered key bindings for display in the help bar.
func (m *EmailView) SetPluginKeyBindings(bindings []PluginKeyBinding) {
	m.pluginKeyBindings = bindings
}

func inlineImagesFromAttachments(atts []fetcher.Attachment) []view.InlineImage {
	var imgs []view.InlineImage
	for _, att := range atts {
		if !att.Inline || len(att.Data) == 0 || att.ContentID == "" {
			continue
		}
		imgs = append(imgs, view.InlineImage{
			CID:    att.ContentID,
			Base64: base64.StdEncoding.EncodeToString(att.Data),
		})
	}
	return imgs
}

func wrapBodyToWidth(body string, width int) string {
	return BodyStyle.Width(width).Render(body)
}

// GetEmail returns the email being viewed
func (m *EmailView) GetEmail() fetcher.Email {
	return m.email
}

// renderCalendarInvite renders a calendar invite card
func renderCalendarInvite(event *calendar.Event) string {
	if event == nil {
		return ""
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.ActiveTheme.Accent).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	var b strings.Builder
	b.WriteString("📅 Meeting Invite\n\n")
	fmt.Fprintf(&b, "Title:    %s\n", event.Summary)
	fmt.Fprintf(&b, "When:     %s\n", formatEventTime(event.Start, event.End))

	if event.Location != "" {
		fmt.Fprintf(&b, "Where:    %s\n", event.Location)
	}

	fmt.Fprintf(&b, "Organizer: %s\n", event.Organizer)

	if event.Description != "" {
		desc := truncateString(event.Description, 100)
		fmt.Fprintf(&b, "\n%s\n", desc)
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Italic(true).Render("Press 1:Accept  2:Decline  3:Tentative"))

	return style.Render(b.String())
}

// formatEventTime formats event start/end times
func formatEventTime(start, end time.Time) string {
	start = start.Local()
	end = end.Local()
	if start.Format("2006-01-02") == end.Format("2006-01-02") {
		// Same day
		return fmt.Sprintf("%s, %s - %s",
			start.Format("Mon Jan 2, 2006"),
			start.Format("3:04 PM"),
			end.Format("3:04 PM"))
	}
	// Multi-day
	return fmt.Sprintf("%s - %s",
		start.Format("Mon Jan 2 3:04 PM"),
		end.Format("Mon Jan 2 3:04 PM"))
}

// truncateString truncates string to maxLen
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
