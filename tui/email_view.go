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
	"github.com/floatpane/matcha/calendar"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/theme"
	"github.com/floatpane/matcha/view"
)

// ClearKittyGraphics sends the Kitty graphics protocol delete command directly to stdout.
func ClearKittyGraphics() {
	// Delete all images: a=d (action=delete), d=A (delete all)
	os.Stdout.WriteString("\x1b_Ga=d,d=A\x1b\\")
	os.Stdout.Sync()
}

var (
	emailHeaderStyle   = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).Padding(0, 1)
	attachmentBoxStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).PaddingLeft(2).MarginTop(1)
)

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
		if att.Filename == "smime-status.internal" {
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

	body, placements, err := view.ProcessBodyWithInline(email.Body, inlineImages, H1Style, H2Style, BodyStyle, !showImages)
	if err != nil {
		body = fmt.Sprintf("Error rendering body: %v", err)
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
	}
}

// NewEmailViewPreview creates EmailView in preview mode with column offset for images
func NewEmailViewPreview(email fetcher.Email, width, height, colOffset int, disableImages bool) *EmailView {
	ev := NewEmailView(email, 0, width, height, MailboxInbox, disableImages)
	ev.isPreviewMode = true
	ev.columnOffset = colOffset
	return ev
}

func (m *EmailView) Init() tea.Cmd {
	return nil
}

func (m *EmailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Handle 'esc' key locally
		if msg.String() == "esc" {
			if m.focusOnAttachments {
				m.focusOnAttachments = false
				return m, nil
			}
			// Clear Kitty graphics before returning to mailbox
			ClearKittyGraphics()
			return m, func() tea.Msg { return BackToMailboxMsg{Mailbox: m.mailbox} }
		}

		if m.focusOnAttachments {
			switch msg.String() {
			case "up", "k":
				if m.attachmentCursor > 0 {
					m.attachmentCursor--
				}
			case "down", "j":
				if m.attachmentCursor < len(m.email.Attachments)-1 {
					m.attachmentCursor++
				}
			case "enter":
				if len(m.email.Attachments) > 0 {
					selected := m.email.Attachments[m.attachmentCursor]
					idx := m.emailIndex
					accountID := m.accountID
					return m, func() tea.Msg {
						return DownloadAttachmentMsg{
							Index:     idx,
							Filename:  selected.Filename,
							PartID:    selected.PartID,
							Data:      selected.Data,
							AccountID: accountID,
							Mailbox:   m.mailbox,
						}
					}
				}
			case "tab":
				m.focusOnAttachments = false
			}
		} else {
			switch msg.String() {
			case "i":
				if view.ImageProtocolSupported() {
					m.showImages = !m.showImages
					ClearKittyGraphics()

					inlineImages := inlineImagesFromAttachments(m.email.Attachments)
					body, placements, err := view.ProcessBodyWithInline(m.email.Body, inlineImages, H1Style, H2Style, BodyStyle, !m.showImages)
					if err != nil {
						body = fmt.Sprintf("Error rendering body: %v", err)
					}
					m.imagePlacements = placements
					wrapped := wrapBodyToWidth(body, m.viewport.Width())
					m.viewport.SetContent(wrapped + "\n")
					return m, nil
				}
			case "r":
				// Clear Kitty graphics before opening composer
				ClearKittyGraphics()
				return m, func() tea.Msg { return ReplyToEmailMsg{Email: m.email} }
			case "f":
				// Clear Kitty graphics before opening composer
				ClearKittyGraphics()
				return m, func() tea.Msg { return ForwardEmailMsg{Email: m.email} }
			case "d":
				accountID := m.accountID
				uid := m.email.UID
				// Clear Kitty graphics before transitioning
				ClearKittyGraphics()
				return m, func() tea.Msg {
					return DeleteEmailMsg{UID: uid, AccountID: accountID, Mailbox: m.mailbox}
				}
			case "a":
				accountID := m.accountID
				uid := m.email.UID
				// Clear Kitty graphics before transitioning
				ClearKittyGraphics()
				return m, func() tea.Msg {
					return ArchiveEmailMsg{UID: uid, AccountID: accountID, Mailbox: m.mailbox}
				}
			case "1", "2", "3":
				if m.hasCalendarInvite && m.calendarEvent != nil {
					var response string
					switch msg.String() {
					case "1":
						response = "ACCEPTED"
					case "2":
						response = "DECLINED"
					case "3":
						response = "TENTATIVE"
					}

					return m, func() tea.Msg {
						return SendRSVPMsg{
							OriginalICS: m.originalICSData,
							Event:       m.calendarEvent,
							Response:    response,
							AccountID:   m.accountID,
							InReplyTo:   m.email.MessageID,
							References:  m.email.References,
						}
					}
				}
			case "tab":
				if len(m.email.Attachments) > 0 {
					m.focusOnAttachments = true
				}
			}
		}
	case tea.WindowSizeMsg:
		header := fmt.Sprintf("To: %s\nFrom: %s\nSubject: %s ", strings.Join(m.email.To, ", "), m.email.From, m.email.Subject)
		headerHeight := lipgloss.Height(header) + 2
		attachmentHeight := 0
		if len(m.email.Attachments) > 0 {
			attachmentHeight = len(m.email.Attachments) + 2
		}
		// Update viewport dimensions
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - headerHeight - attachmentHeight)

		// When the window size changes, wrap and clear kitty images to keep placement stable
		ClearKittyGraphics()
		inlineImages := inlineImagesFromAttachments(m.email.Attachments)
		body, placements, err := view.ProcessBodyWithInline(m.email.Body, inlineImages, H1Style, H2Style, BodyStyle, !m.showImages)
		if err != nil {
			body = fmt.Sprintf("Error rendering body: %v", err)
		}
		m.imagePlacements = placements
		wrapped := wrapBodyToWidth(body, m.viewport.Width())
		m.viewport.SetContent(wrapped + "\n")
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *EmailView) View() tea.View {
	// Clear image placements (but keep uploaded image data in terminal memory)
	// before re-rendering to prevent stacking on scroll. Uses d=a (delete all
	// placements) instead of d=A (delete all including data) so that images
	// can be re-displayed by ID without re-uploading.
	os.Stdout.WriteString("\x1b_Ga=d,d=a\x1b\\")
	os.Stdout.Sync()

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

	header := fmt.Sprintf("To: %s | From: %s | Subject: %s%s", strings.Join(m.email.To, ", "), m.email.From, m.email.Subject, cryptoStatus.String())
	styledHeader := emailHeaderStyle.Width(m.viewport.Width()).Render(header)

	var help string
	if m.focusOnAttachments {
		helpText := "↑/↓: navigate • enter: download • esc/tab: back to email body"
		if m.pluginStatus != "" {
			helpText += " • " + m.pluginStatus
		}
		help = helpStyle.Render(helpText)
	} else {
		var shortcuts strings.Builder
		shortcuts.WriteString("\uf112 r: reply • \uf064 f: forward • \uea81 d: delete • \uea98 a: archive • \uf435 tab: focus attachments • \ueb06 esc: back to inbox")
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
		help = helpStyle.Render(shortcuts.String())
	}

	var attachmentView string
	if len(m.email.Attachments) > 0 {
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
		attachmentView = attachmentBoxStyle.Render(b.String())
	}

	// Render visible images directly to stdout. Bubbletea v2's ultraviolet
	// renderer uses a cell-based model that cannot pass through graphics
	// protocol escape sequences, so we write them out-of-band.
	if m.showImages && len(m.imagePlacements) > 0 {
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
				screenRow := headerLines + (p.Line - yOffset)
				if m.columnOffset > 0 {
					view.RenderImageToStdout(p, screenRow, m.columnOffset+1)
				} else {
					view.RenderImageToStdout(p, screenRow)
				}
			}
		}
	}

	// Render calendar invite card if present
	var calendarView string
	if m.hasCalendarInvite && m.calendarEvent != nil {
		calendarView = renderCalendarInvite(m.calendarEvent)
	}

	// m.viewport.View() returns a string in Bubbles v2 viewport
	if calendarView != "" {
		return tea.NewView(fmt.Sprintf("%s\n%s\n%s\n%s\n%s", styledHeader, calendarView, m.viewport.View(), attachmentView, help))
	}
	return tea.NewView(fmt.Sprintf("%s\n%s\n%s\n%s", styledHeader, m.viewport.View(), attachmentView, help))
}

// GetAccountID returns the account ID for this email
func (m *EmailView) GetAccountID() string {
	return m.accountID
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
	b.WriteString(fmt.Sprintf("Title:    %s\n", event.Summary))
	b.WriteString(fmt.Sprintf("When:     %s\n", formatEventTime(event.Start, event.End)))

	if event.Location != "" {
		b.WriteString(fmt.Sprintf("Where:    %s\n", event.Location))
	}

	b.WriteString(fmt.Sprintf("Organizer: %s\n", event.Organizer))

	if event.Description != "" {
		desc := truncateString(event.Description, 100)
		b.WriteString(fmt.Sprintf("\n%s\n", desc))
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
