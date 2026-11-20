package tui

import (
	"encoding/base64"
	"fmt"
	"mime"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	calendar "github.com/floatpane/go-icalendar"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/theme"
	"github.com/floatpane/matcha/view"
)

var (
	emailHeaderStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).Padding(0, 1)
	attachmentBoxStyle   = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).PaddingLeft(2).MarginTop(1)
	remoteImageRequestID atomic.Uint64
)

type remoteImagesLoadedMsg struct {
	requestID uint64
	body      *view.RenderedBody
	err       error
	// userRequested is set for loads the user asked for (as opposed to
	// re-renders after a resize), whose outcome is reported.
	userRequested bool
}

// LoadRemoteImagesMsg asks the email on screen (full screen or in the split
// preview) to load its remote images.
type LoadRemoteImagesMsg struct{}

// ImageProtocolChangedMsg tells the email on screen that the terminal's
// image support may have changed (inside tmux, another terminal may have
// attached), so that it draws its images again if needed.
type ImageProtocolChangedMsg struct{}

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
	viewport            viewport.Model
	width               int
	height              int
	email               fetcher.Email
	emailIndex          int
	attachmentCursor    int
	focusOnAttachments  bool
	accountID           string
	mailbox             MailboxKind
	disableImages       bool
	showImages          bool
	showRemoteImages    bool
	remoteImagesLoading bool
	remoteLoadID        uint64
	remoteBody          *view.RenderedBody
	remoteFailed        []string // remote image URLs the last load could not show
	remoteImageCount    int      // remote images the body refers to
	renderedProtocol    view.ImageProtocol
	resizedWhileLoading bool // the loading images were sized for an old viewport
	isSMIME             bool
	smimeTrusted        bool
	isEncrypted         bool
	isPGP               bool
	pgpTrusted          bool
	isPGPEncrypted      bool
	imagePlacements     []view.ImagePlacement
	pluginStatus        string
	pluginKeyBindings   []PluginKeyBinding
	hasCalendarInvite   bool
	calendarEvent       *calendar.Event
	originalICSData     []byte
	isPreviewMode       bool
	columnOffset        int // screen column of the view's left edge (split panes)
	rowOffset           int // screen row of the view's top edge (split panes)
	isPatch             bool
	patchInfo           *view.PatchInfo
	// readOnly disables message actions (reply, delete, …); used when the
	// email is shown next to a reply in progress.
	readOnly bool
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

	// Detect git patches and render with diff syntax highlighting.
	patchInfo := view.DetectPatch(email.Body, email.BodyMIMEType, email.Subject, email.From)

	m := &EmailView{
		viewport:          viewport.New(),
		email:             email,
		emailIndex:        emailIndex,
		accountID:         email.AccountID,
		mailbox:           mailbox,
		disableImages:     disableImages,
		showImages:        !disableImages,
		isSMIME:           isSMIME,
		smimeTrusted:      smimeTrusted,
		isEncrypted:       isEncrypted,
		isPGP:             isPGP,
		pgpTrusted:        pgpTrusted,
		isPGPEncrypted:    isPGPEncrypted,
		hasCalendarInvite: calendarEvent != nil,
		calendarEvent:     calendarEvent,
		originalICSData:   originalICSData,
		isPatch:           patchInfo != nil,
		patchInfo:         patchInfo,
	}
	m.setSize(width, height)
	return m
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

// SetRowOffset sets the screen row of the view's top edge. Parents that place
// the view inside a pane call this after layout so images line up.
func (m *EmailView) SetRowOffset(offset int) {
	m.rowOffset = offset
}

// SetColumnOffset sets the screen column of the view's left edge.
func (m *EmailView) SetColumnOffset(offset int) {
	m.columnOffset = offset
}

// SetSize resizes the view and re-lays out its body.
func (m *EmailView) SetSize(width, height int) {
	m.setSize(width, height)
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
		return ReplyToEmailMsg{Email: m.email}, true
	case kb.Email.ReplyAll:
		return ReplyAllEmailMsg{Email: m.email}, true
	case kb.Email.Forward:
		return ForwardEmailMsg{Email: m.email}, true
	}
	return nil, false
}

func (m *EmailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if handled, handledCmd := m.handleKeyPress(msg); handled {
			model, c := handledCmd()
			m.syncLayout()
			return model, c
		}
	case tea.WindowSizeMsg:
		if msg.Width == m.width && msg.Height == m.height {
			return m, nil
		}
		m.setSize(msg.Width, msg.Height)
		if m.remoteImagesLoading {
			m.resizedWhileLoading = true
		} else if m.showRemoteImages {
			// Remote images were sized for the old viewport; reload them
			// (served from cache) so they fit the new one.
			return m, m.loadRemoteImagesCmd(false)
		}
		return m, nil
	case tea.MouseClickMsg:
		m.handleMouseClick(msg)
		return m, nil
	case LoadRemoteImagesMsg:
		return m, m.requestRemoteImages()
	case ImageProtocolChangedMsg:
		return m, m.redrawForImageProtocol()
	case remoteImagesLoadedMsg:
		return m, m.remoteImagesLoaded(msg)
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// handleMouseClick focuses an attachment when one is clicked and returns
// focus to the body for clicks elsewhere. Coordinates are relative to the
// view's top-left corner.
func (m *EmailView) handleMouseClick(msg tea.MouseClickMsg) {
	if msg.Button != tea.MouseLeft {
		return
	}
	y := msg.Y - m.rowOffset
	rows := len(m.email.Attachments)
	if m.chrome().compactAttachments {
		rows = min(rows, 1)
	}
	if top := m.attachmentsTop(); rows > 0 && y >= top && y < top+rows {
		m.focusOnAttachments = true
		if rows > 1 {
			m.attachmentCursor = y - top
		}
	} else {
		m.focusOnAttachments = false
	}
	m.syncLayout()
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
		if len(m.email.Attachments) > 0 && m.attachmentCursor < len(m.email.Attachments) {
			selected := m.email.Attachments[m.attachmentCursor]
			idx := m.emailIndex
			uid := m.email.UID
			accountID := m.accountID
			mailbox := m.mailbox
			return true, func() (tea.Model, tea.Cmd) {
				return m, func() tea.Msg {
					return DownloadAttachmentMsg{
						Index:     idx,
						UID:       uid,
						Filename:  selected.Filename,
						PartID:    selected.PartID,
						Data:      selected.Data,
						AccountID: accountID,
						Mailbox:   mailbox,
						Encoding:  selected.Encoding,
					}
				}
			}
		}
	case kb.Email.FocusAttachments:
		m.focusOnAttachments = false
		return true, func() (tea.Model, tea.Cmd) { return m, nil }
	}
	return false, nil
}

func (m *EmailView) loadRemoteImagesKey() string {
	if config.Keybinds.Email.LoadRemoteImages != "" {
		return config.Keybinds.Email.LoadRemoteImages
	}
	return "I"
}

// canLoadRemoteImages reports whether the load-remote-images key does
// anything: images were not loaded yet, or some failed and can be retried.
func (m *EmailView) canLoadRemoteImages() bool {
	return view.ImageProtocolSupported() && !m.remoteImagesLoading &&
		(!m.showRemoteImages || len(m.remoteFailed) > 0)
}

// OffersRemoteImages reports whether loading remote images is worth
// offering: the email has some and they can still be (re)loaded.
func (m *EmailView) OffersRemoteImages() bool {
	return m.remoteImageCount > 0 && m.canLoadRemoteImages()
}

// requestRemoteImages starts a load the user asked for.
func (m *EmailView) requestRemoteImages() tea.Cmd {
	if !m.canLoadRemoteImages() {
		return nil
	}
	if !m.showImages {
		m.showImages = true
		m.regenerateBody()
	}
	cmd := m.loadRemoteImagesCmd(true)
	m.syncLayout()
	return cmd
}

// loadRemoteImagesCmd renders the body with remote images in the background.
// A load the user asked for retries every image; re-renders for a new size
// skip the ones that failed before, which would only fail again, slowly.
func (m *EmailView) loadRemoteImagesCmd(userRequested bool) tea.Cmd {
	m.remoteImagesLoading = true
	m.resizedWhileLoading = false
	m.remoteLoadID = remoteImageRequestID.Add(1)
	body, mimeType := m.email.Body, m.email.BodyMIMEType
	inline := inlineImagesFromAttachments(m.email.Attachments)
	requestID := m.remoteLoadID
	bounds := m.imageBounds()
	skip := m.remoteImagesToSkip(userRequested)
	return func() tea.Msg {
		rendered, err := view.RenderBody(body, mimeType, inline, H1Style, H2Style, BodyStyle, view.RenderOptions{
			AllowRemoteImages: true,
			SkipRemoteImages:  skip,
			Bounds:            bounds,
		})
		return remoteImagesLoadedMsg{requestID: requestID, body: rendered, err: err, userRequested: userRequested}
	}
}

// redrawForImageProtocol draws the images again when the protocol they were
// drawn with is no longer the terminal's.
func (m *EmailView) redrawForImageProtocol() tea.Cmd {
	if !m.showImages || view.ActiveImageProtocol() == m.renderedProtocol {
		return nil
	}
	if m.showRemoteImages {
		if !m.remoteImagesLoading {
			return m.loadRemoteImagesCmd(false) // downloads come from the cache
		}
		m.resizedWhileLoading = true // reload once the current load is done
		return nil
	}
	m.regenerateBody()
	m.syncLayout()
	return nil
}

// remoteImagesToSkip returns the remote images a load should not download:
// none when the user asked (a retry), the ones that failed before otherwise.
func (m *EmailView) remoteImagesToSkip(userRequested bool) map[string]bool {
	if userRequested || len(m.remoteFailed) == 0 {
		return nil
	}
	skip := make(map[string]bool, len(m.remoteFailed))
	for _, u := range m.remoteFailed {
		skip[u] = true
	}
	return skip
}

// remoteImagesLoaded shows the result of a remote image load and, for loads
// the user asked for, reports images that could not be shown.
func (m *EmailView) remoteImagesLoaded(msg remoteImagesLoadedMsg) tea.Cmd {
	if !m.remoteImagesLoading || msg.requestID != m.remoteLoadID {
		return nil
	}
	m.remoteImagesLoading = false
	if msg.err != nil || msg.body == nil {
		// Keep showing the body without remote images and let the user retry.
		m.showRemoteImages = false
		m.syncLayout()
		if !msg.userRequested {
			return nil
		}
		reason := "unknown error"
		if msg.err != nil {
			reason = msg.err.Error()
		}
		return func() tea.Msg { return NotifyMsg{Message: "Could not load remote images: " + reason} }
	}
	m.showRemoteImages = true
	m.remoteBody = msg.body
	m.remoteFailed = msg.body.FailedRemoteImages
	if m.showImages {
		m.regenerateBody()
	}
	m.syncLayout()
	var cmds []tea.Cmd
	if m.resizedWhileLoading {
		// The images were sized for the viewport before the resize.
		cmds = append(cmds, m.loadRemoteImagesCmd(false))
	}
	if msg.userRequested {
		cmds = append(cmds, remoteImagesNotice(msg.body.RemoteImages, len(msg.body.FailedRemoteImages)))
	}
	return tea.Batch(cmds...)
}

// remoteImagesNotice reports a remote image load in which not every image
// could be shown.
func remoteImagesNotice(total, failed int) tea.Cmd {
	var notice tea.Msg
	switch {
	case total == 0:
		notice = InfoNotifyMsg{Message: "This email has no remote images", Duration: 3}
	case failed == 0:
		return nil
	case total == 1:
		notice = NotifyMsg{Message: "The remote image could not be loaded"}
	case failed == total:
		notice = NotifyMsg{Message: fmt.Sprintf("None of the %d remote images could be loaded", total)}
	default:
		notice = InfoNotifyMsg{Message: fmt.Sprintf("%d of %d remote images could not be loaded", failed, total), Duration: 4}
	}
	return func() tea.Msg { return notice }
}

// handleEmailBodyKey handles key presses when the email body has focus.
func (m *EmailView) handleEmailBodyKey(msg tea.KeyPressMsg) (bool, func() (tea.Model, tea.Cmd)) {
	kb := config.Keybinds
	switch msg.String() {
	case kb.Email.ToggleImages:
		if view.ImageProtocolSupported() {
			m.showImages = !m.showImages
			m.regenerateBody()
			return true, func() (tea.Model, tea.Cmd) { return m, nil }
		}
	case m.loadRemoteImagesKey():
		if m.canLoadRemoteImages() {
			cmd := m.requestRemoteImages()
			return true, func() (tea.Model, tea.Cmd) { return m, cmd }
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
		return true, func() (tea.Model, tea.Cmd) {
			return m, func() tea.Msg {
				return DeleteEmailMsg{UID: uid, AccountID: accountID, Mailbox: mailbox}
			}
		}
	case kb.Email.Archive:
		accountID := m.accountID
		uid := m.email.UID
		mailbox := m.mailbox
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

	case kb.Email.YankBody:
		var body string
		isHTML := m.email.BodyMIMEType == "text/html"
		if isHTML {
			body = ansi.Strip(m.viewport.GetContent())
		} else {
			body = m.email.Body
		}

		body = sanitizeYankedText(body, isHTML)

		if err := clipboard.WriteAll(body); err != nil {
			return true, func() (tea.Model, tea.Cmd) {
				return m, func() tea.Msg { return NotifyMsg{Message: "Failed to copy body: " + err.Error()} }
			}
		}
		return true, func() (tea.Model, tea.Cmd) {
			return m, func() tea.Msg { return InfoNotifyMsg{Message: "Email body copied to clipboard", Duration: 3} }
		}

	case kb.Email.YankHeaders:
		// Format the main headers into a clean string. This is useful for
		// referencing the email in a conversation or saving its metadata.
		// Decode the From header to handle RFC 2047 encoded names (e.g. =?UTF-8?B?...?=).
		dec := new(mime.WordDecoder)
		decodedFrom, err := dec.DecodeHeader(m.email.From)
		if err != nil {
			decodedFrom = m.email.From
		}

		headers := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\nDate: %s",
			decodedFrom,
			strings.Join(m.email.To, ", "),
			m.email.Subject,
			m.email.Date.Format(time.RFC1123Z),
		)
		headers = sanitizeYankedText(headers, false)

		if err := clipboard.WriteAll(headers); err != nil {
			return true, func() (tea.Model, tea.Cmd) {
				return m, func() tea.Msg { return NotifyMsg{Message: "Failed to copy headers: " + err.Error()} }
			}
		}
		return true, func() (tea.Model, tea.Cmd) {
			return m, func() tea.Msg { return InfoNotifyMsg{Message: "Email headers copied to clipboard", Duration: 3} }
		}

	case kb.Email.FocusAttachments:
		if len(m.email.Attachments) > 0 {
			m.focusOnAttachments = true
			return true, func() (tea.Model, tea.Cmd) { return m, nil }
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

// imageBounds limits inline images to the visible body area so they can
// always be seen in full and never spill into neighbouring panes.
func (m *EmailView) imageBounds() view.ImageBounds {
	return view.ImageBounds{
		MaxCols: max(1, m.viewport.Width()-1),
		MaxRows: max(1, m.viewport.Height()),
	}
}

// renderedBody returns the processed body, using the downloaded remote images
// when the user opted in. Ordinary updates never touch the network.
func (m *EmailView) renderedBody() *view.RenderedBody {
	if m.showImages && m.showRemoteImages && m.remoteBody != nil {
		return m.remoteBody
	}
	inlineImages := inlineImagesFromAttachments(m.email.Attachments)
	rendered, err := view.RenderBody(m.email.Body, m.email.BodyMIMEType, inlineImages, H1Style, H2Style, BodyStyle, view.RenderOptions{
		DisableImages: !m.showImages,
		Bounds:        m.imageBounds(),
	})
	if err != nil {
		return &view.RenderedBody{Text: fmt.Sprintf("Error rendering body: %v", err)}
	}
	return rendered
}

// regenerateBody re-renders the email body for the current size and image
// settings. Image positions are resolved after wrapping, so they always match
// the final line layout.
func (m *EmailView) regenerateBody() {
	width := m.viewport.Width()
	if m.isPatch {
		if patchBody, ok := view.RenderPatchBody(m.email.Body, m.email.BodyMIMEType, m.email.Subject, m.email.From, width); ok {
			m.imagePlacements = nil
			m.viewport.SetContent(wrapBodyToWidth(applyBodyTransform(patchBody, m.email), width) + "\n")
			return
		}
	}
	rendered := m.renderedBody()
	m.renderedProtocol = view.ActiveImageProtocol()
	m.remoteImageCount = rendered.RemoteImages
	wrapped := wrapBodyToWidth(applyBodyTransform(rendered.Text, m.email), width)
	text, placements := rendered.Finalize(wrapped)
	m.imagePlacements = placements
	m.viewport.SetContent(text + "\n")
}

// setSize lays the view out for a new size and re-renders the body.
func (m *EmailView) setSize(width, height int) {
	m.width = max(width, 1)
	m.height = max(height, 1)
	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(max(1, m.height-m.chromeHeight()))
	m.regenerateBody()
	// The help bar depends on the body (whether it has remote images), so
	// measure again and fit the body to the final height.
	if h := max(1, m.height-m.chromeHeight()); h != m.viewport.Height() {
		m.viewport.SetHeight(h)
		m.regenerateBody()
	}
}

// syncLayout re-applies the viewport height after state changes that alter
// the height of the surrounding chrome (help text, attachment focus).
func (m *EmailView) syncLayout() {
	if h := max(1, m.height-m.chromeHeight()); h != m.viewport.Height() {
		m.viewport.SetHeight(h)
	}
}

// emailChrome holds the rendered parts of the view around the body.
type emailChrome struct {
	header      string
	calendar    string
	attachments string
	help        string
	// compactAttachments is set when the attachment list is collapsed to a
	// single line; only the selected attachment is then clickable.
	compactAttachments bool
}

func (c emailChrome) height() int {
	h := lipgloss.Height(c.header) + lipgloss.Height(c.help)
	if c.calendar != "" {
		h += lipgloss.Height(c.calendar)
	}
	if c.attachments != "" {
		h += lipgloss.Height(c.attachments)
	}
	return h
}

// minBodyRows is the body height the chrome gives way to on small screens.
const minBodyRows = 3

// chrome renders everything around the body. When the view is too short for
// all of it, parts are compacted one step at a time (one-line help, one-line
// header, a single attachment line, no calendar card) so the body keeps
// some room and the view never grows taller than its height.
func (m *EmailView) chrome() emailChrome {
	c := emailChrome{
		header:      m.renderHeader(3),
		calendar:    m.renderCalendar(),
		attachments: m.renderAttachmentView(false),
		help:        m.renderHelp(),
	}
	budget := m.height - min(minBodyRows, max(1, m.height/3))
	if c.height() <= budget {
		return c
	}
	c.help = ansi.Truncate(strings.ReplaceAll(c.help, "\n", " "), max(1, m.width-1), "…")
	if c.height() <= budget {
		return c
	}
	c.header = m.renderHeader(1)
	if c.height() <= budget {
		return c
	}
	if c.attachments != "" {
		c.attachments = m.renderAttachmentView(true)
		c.compactAttachments = true
	}
	if c.height() <= budget {
		return c
	}
	c.calendar = ""
	return c
}

// chromeHeight is the number of rows used by everything but the body.
func (m *EmailView) chromeHeight() int {
	return m.chrome().height()
}

// bodyTop is the row, relative to the view, where the body viewport starts.
func (m *EmailView) bodyTop() int {
	c := m.chrome()
	top := lipgloss.Height(c.header)
	if c.calendar != "" {
		top += lipgloss.Height(c.calendar)
	}
	return top
}

// attachmentsTop is the row, relative to the view, of the first attachment
// (after the box margin and, when expanded, the "Attachments:" title).
func (m *EmailView) attachmentsTop() int {
	top := m.bodyTop() + m.viewport.Height() + attachmentBoxStyle.GetMarginTop()
	if !m.chrome().compactAttachments {
		top++
	}
	return top
}

func (m *EmailView) View() tea.View {
	c := m.chrome()
	sections := []string{c.header}
	if c.calendar != "" {
		sections = append(sections, c.calendar)
	}
	sections = append(sections, m.viewport.View())
	if c.attachments != "" {
		sections = append(sections, c.attachments)
	}
	sections = append(sections, c.help)
	content := strings.Join(sections, "\n")
	if m.width > 0 && m.height > 0 {
		content = fitBox(content, m.width, m.height)
	}
	return tea.NewView(content)
}

// VisibleImages reports where the out-of-band images of this view are on
// screen. Kitty images that are partly scrolled out of the body are clipped
// to the visible band; Sixel images are only shown while fully visible.
func (m *EmailView) VisibleImages() []ScreenImage {
	if !m.showImages || len(m.imagePlacements) == 0 {
		return nil
	}
	top := m.viewport.YOffset()
	height := m.viewport.Height()
	originRow := m.rowOffset + m.bodyTop()
	var out []ScreenImage
	for i, p := range m.imagePlacements {
		if p.Rows < 1 || p.Cols < 1 {
			continue
		}
		first := max(p.Line, top)
		last := min(p.Line+p.Rows, top+height)
		if first >= last {
			continue
		}
		img := ScreenImage{
			ID:        p.ID,
			Placement: uint32(i + 1),
			Protocol:  p.Protocol,
			Row:       originRow + first - top,
			Col:       m.columnOffset,
			Cols:      p.Cols,
			Rows:      last - first,
			Data:      p.Encoded,
		}
		switch p.Protocol {
		case view.ImageProtocolKitty:
			img.SrcW = p.PixelW
			img.SrcY = (first - p.Line) * p.PixelH / p.Rows
			img.SrcH = max(1, (last-p.Line)*p.PixelH/p.Rows-img.SrcY)
		case view.ImageProtocolSixel:
			if first != p.Line || last != p.Line+p.Rows {
				continue
			}
		case view.ImageProtocolKittyPlaceholder:
			// Drawn by the body text; listed so that it gets uploaded.
		case view.ImageProtocolNone, view.ImageProtocolHalfBlock:
			continue
		}
		out = append(out, img)
	}
	return out
}

// renderHeader renders the email header bar, including crypto and patch
// badges, wrapped to at most maxLines lines.
func (m *EmailView) renderHeader(maxLines int) string {
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

	header := fmt.Sprintf("To: %s | From: %s | Subject: %s%s",
		normalizeInboxRowText(strings.Join(m.email.To, ", ")),
		normalizeInboxRowText(m.email.From),
		normalizeInboxRowText(m.email.Subject),
		cryptoStatus.String())
	// Long headers wrap; cap them so the body keeps most of the space.
	maxLines = max(1, maxLines)
	inner := max(1, m.width-emailHeaderStyle.GetHorizontalFrameSize())
	lines := strings.Split(lipgloss.Wrap(header, inner, ""), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = ansi.Truncate(lines[maxLines-1], max(1, inner-1), "") + "…"
	}
	return emailHeaderStyle.Width(m.width).Render(strings.Join(lines, "\n"))
}

// renderHelp renders the bottom help bar based on current focus and state,
// wrapped to the view width.
func (m *EmailView) renderHelp() string {
	kb := config.Keybinds
	if m.focusOnAttachments {
		helpText := joinHelp(
			helpItem("", navArrowsLabel(), "navigate"),
			helpItem("", keyEnter, "download"),
			helpItem("", joinKeys(kbKey(kb.Global.Cancel, "esc"), kbKey(kb.Email.FocusAttachments, "tab")), "back to email body"),
		)
		if m.pluginStatus != "" {
			helpText += helpSeparator + m.pluginStatus
		}
		return helpStyle.Width(m.width).Render(helpText)
	}

	if m.readOnly {
		helpText := joinHelp(
			helpItem("", navArrowsLabel(), "scroll"),
			helpItem("", joinKeys(kbKey(kb.Global.Cancel, "esc"), replySplitSwitchKey), "back to reply"),
		)
		if view.ImageProtocolSupported() {
			helpText += helpSeparator + helpItem("", kbKey(kb.Email.ToggleImages, "i"), "toggle images").String()
		}
		return helpStyle.Width(m.width).Render(helpText)
	}

	var shortcuts strings.Builder
	shortcuts.WriteString(joinHelp(
		helpItem("", kbKey(kb.Email.Reply, "r"), "reply"),
		helpItem("", kbKey(kb.Email.ReplyAll, "shift+r"), "reply all"),
		helpItem("", kbKey(kb.Email.Forward, "f"), "forward"),
		helpItem("", kbKey(kb.Email.Delete, "d"), "delete"),
		helpItem("", kbKey(kb.Email.Archive, "a"), "archive"),
	))
	if len(m.email.Attachments) > 0 {
		shortcuts.WriteString(helpSeparator)
		shortcuts.WriteString(helpItem("", kbKey(kb.Email.FocusAttachments, "tab"), "focus attachments").String())
	}
	shortcuts.WriteString(helpSeparator)
	shortcuts.WriteString(helpItem("", kbKey(kb.Global.Cancel, "esc"), "back to inbox").String())
	if m.isPatch && m.patchInfo != nil && m.patchInfo.HasDiff {
		shortcuts.WriteString(helpSeparator)
		shortcuts.WriteString(helpItem("", kbKey(kb.Email.ApplyPatch, "p"), "apply patch").String())
	}
	shortcuts.WriteString(helpSeparator)
	shortcuts.WriteString(helpItem("", kbKey(kb.Email.SendPatch, "P"), "send patch").String())
	shortcuts.WriteString(helpSeparator)
	shortcuts.WriteString(helpItem("", kbKey(kb.Email.YankBody, "y"), "yank body").String())
	shortcuts.WriteString(helpSeparator)
	shortcuts.WriteString(helpItem("", kbKey(kb.Email.YankHeaders, "Y"), "yank headers").String())
	if view.ImageProtocolSupported() {
		shortcuts.WriteString(helpSeparator)
		shortcuts.WriteString(helpItem("", kbKey(kb.Email.ToggleImages, "i"), "toggle images").String())
		switch {
		case m.remoteImagesLoading:
			shortcuts.WriteString(helpSeparator)
			shortcuts.WriteString("loading remote images…")
		case !m.showRemoteImages && m.remoteImageCount > 0:
			shortcuts.WriteString(helpSeparator)
			shortcuts.WriteString(helpItem("", m.loadRemoteImagesKey(), "load remote images").String())
		case m.showRemoteImages && len(m.remoteFailed) > 0:
			shortcuts.WriteString(helpSeparator)
			shortcuts.WriteString(helpItem("", m.loadRemoteImagesKey(), "retry remote images").String())
		}
	}
	for _, pk := range m.pluginKeyBindings {
		shortcuts.WriteString(helpSeparator)
		shortcuts.WriteString(pk.Key)
		shortcuts.WriteString(": ")
		shortcuts.WriteString(pk.Description)
	}
	if m.pluginStatus != "" {
		shortcuts.WriteString(helpSeparator)
		shortcuts.WriteString(m.pluginStatus)
	}
	return helpStyle.Width(m.width).Render(shortcuts.String())
}

// renderAttachmentView renders the attachment list panel. The compact form
// is a single line showing the selected attachment.
func (m *EmailView) renderAttachmentView(compact bool) string {
	if len(m.email.Attachments) == 0 {
		return ""
	}
	inner := max(1, m.width-attachmentBoxStyle.GetHorizontalFrameSize())
	if compact {
		cursor := min(m.attachmentCursor, len(m.email.Attachments)-1)
		style := itemStyle
		marker := "  "
		if m.focusOnAttachments {
			style = selectedItemStyle
			marker = "▌ "
		}
		label := fmt.Sprintf("%s%s (%d/%d)", marker, normalizeInboxRowText(m.email.Attachments[cursor].Filename), cursor+1, len(m.email.Attachments))
		return attachmentBoxStyle.Render(style.Render(ansi.Truncate(label, max(1, inner-style.GetHorizontalFrameSize()), "…")))
	}
	lines := make([]string, 0, len(m.email.Attachments)+1)
	lines = append(lines, "Attachments:")
	for i, attachment := range m.email.Attachments {
		cursor := "  "
		style := itemStyle
		if m.focusOnAttachments && i == m.attachmentCursor {
			cursor = "▌ "
			style = selectedItemStyle
		}
		name := ansi.Truncate(cursor+normalizeInboxRowText(attachment.Filename), max(1, inner-style.GetHorizontalFrameSize()), "…")
		lines = append(lines, style.Render(name))
	}
	return attachmentBoxStyle.Render(strings.Join(lines, "\n"))
}

// renderCalendar renders the meeting invite card, or "" without an invite.
func (m *EmailView) renderCalendar() string {
	if !m.hasCalendarInvite || m.calendarEvent == nil {
		return ""
	}
	return renderCalendarInvite(m.calendarEvent, m.width)
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
	m.syncLayout()
}

// SetPluginKeyBindings sets the plugin-registered key bindings for display in the help bar.
func (m *EmailView) SetPluginKeyBindings(bindings []PluginKeyBinding) {
	m.pluginKeyBindings = bindings
	m.syncLayout()
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

// renderCalendarInvite renders a calendar invite card no wider than width.
func renderCalendarInvite(event *calendar.Event, width int) string {
	if event == nil {
		return ""
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.ActiveTheme.Accent).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)
	if width > 0 {
		style = style.Width(min(width, 80))
	}

	kb := config.Keybinds
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
	b.WriteString(lipgloss.NewStyle().Italic(true).Render(fmt.Sprintf("Press %s:Accept  %s:Decline  %s:Tentative",
		kbKey(kb.Email.RsvpAccept, "1"), kbKey(kb.Email.RsvpDecline, "2"), kbKey(kb.Email.RsvpTentative, "3"))))

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

// truncateString shortens s to at most maxLen display cells, appending "..."
// when it was cut. It never splits a multi-byte character.
func truncateString(s string, maxLen int) string {
	if ansi.StringWidth(s) <= maxLen {
		return s
	}
	return ansi.Truncate(s, maxLen, "") + "..."
}

// sanitizeYankedText cleans up text before copying it to the clipboard.
// It normalizes line endings (removing CR from CRLF) and trims trailing
// whitespace left by the TUI renderer to fill the screen width.
func sanitizeYankedText(text string, isHTML bool) string {
	// 1. Remove carriage returns (CRLF -> LF) to avoid "^M" in the clipboard.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")

	if isHTML {
		// 2. Trim trailing spaces on each line. The HTML renderer pads lines
		// with spaces to fill the viewport width. We don't want that in the
		// clipboard.
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " ")
		}
		text = strings.Join(lines, "\n")

		// 3. Collapse 3 or more consecutive newlines into just 2 (one blank line)
		reNewlines := regexp.MustCompile(`\n{3,}`)
		text = reNewlines.ReplaceAllString(text, "\n\n")
	}

	return strings.TrimSpace(text)
}
