package palette

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/export"
	"github.com/floatpane/matcha/tui"
)

// New creates a new, closed Palette wrapper.
func New() *Palette {
	return &Palette{}
}

// Palette is a thin wrapper around the command palette model.
type Palette struct {
	*tui.CommandPalette
	open bool
}

func (p *Palette) Open(commands []tui.PaletteCommand, width, height int) tea.Cmd {
	p.CommandPalette = tui.NewCommandPalette(commands, width, height)
	p.open = true
	return p.Init()
}

func (p *Palette) Close() {
	p.open = false
	p.CommandPalette = nil
}

func (p *Palette) IsOpen() bool {
	return p.open
}

func (p *Palette) SetOpen(open bool) {
	p.open = open
}

func (p *Palette) UpdateSize(width, height int) {
	if p.CommandPalette != nil {
		p.SetSize(width, height)
	}
}

func (p *Palette) Render(content string, width, height int) string {
	if p.CommandPalette == nil {
		return content
	}
	return p.CommandPalette.Render(content, width, height)
}

func (p *Palette) HandleKey(msg tea.KeyPressMsg) tea.Cmd {
	if p.CommandPalette == nil {
		return nil
	}
	switch msg.String() {
	case "enter":
		action := p.SelectedCmd()
		return action
	default:
		return p.Update(msg)
	}
}

// Allowed reports whether the command palette may be opened for the active view.
func Allowed(current tea.Model) bool {
	switch v := current.(type) {
	case *tui.Composer, *tui.Login, *tui.SignatureEditor, *tui.MailingListEditor, *tui.ContactEditor,
		*tui.PasswordPrompt, *tui.FilePicker, *tui.SaveFilePicker, *tui.Status:
		return false
	case *tui.Inbox:
		return !v.IsSearchActive() && !v.IsFilterActive()
	case *tui.FolderInbox:
		if inbox := v.GetInbox(); inbox != nil {
			return !inbox.IsSearchActive() && !inbox.IsFilterActive()
		}
	}
	return true
}

// BuildCommands assembles context-specific and global palette commands.
func BuildCommands(current tea.Model, folderInbox *tui.FolderInbox) []tui.PaletteCommand {
	kb := config.Keybinds
	var cmds []tui.PaletteCommand

	switch v := current.(type) {
	case *tui.EmailView:
		cmds = append(cmds,
			tui.PaletteCommand{Title: "Reply", Hint: kb.Email.Reply, Keywords: "respond answer", Action: KeyAction(kb.Email.Reply)},
			tui.PaletteCommand{Title: "Forward", Hint: kb.Email.Forward, Keywords: "fwd send on", Action: KeyAction(kb.Email.Forward)},
			tui.PaletteCommand{Title: "Archive email", Hint: kb.Email.Archive, Keywords: "file store", Action: KeyAction(kb.Email.Archive)},
			tui.PaletteCommand{Title: "Delete email", Hint: kb.Email.Delete, Keywords: "trash remove", Action: KeyAction(kb.Email.Delete)},
			tui.PaletteCommand{Title: "Toggle images", Hint: kb.Email.ToggleImages, Keywords: "pictures show hide", Action: KeyAction(kb.Email.ToggleImages)},
		)
		cmds = append(cmds, EmailExportCommands(v, folderInbox)...)
	case *tui.Inbox, *tui.FolderInbox:
		cmds = append(cmds,
			tui.PaletteCommand{Title: "Refresh", Hint: kb.Inbox.Refresh, Keywords: "reload sync fetch", Action: KeyAction(kb.Inbox.Refresh)},
			tui.PaletteCommand{Title: "Search mail", Hint: kb.Inbox.Search, Keywords: "find query", Action: KeyAction(kb.Inbox.Search)},
			tui.PaletteCommand{Title: "Filter", Hint: kb.Inbox.Filter, Keywords: "narrow", Action: KeyAction(kb.Inbox.Filter)},
			tui.PaletteCommand{Title: "Toggle threaded view", Hint: kb.Inbox.ToggleThreaded, Keywords: "conversation thread", Action: KeyAction(kb.Inbox.ToggleThreaded)},
			tui.PaletteCommand{Title: "Select / visual mode", Hint: kb.Inbox.VisualMode, Keywords: "multi batch", Action: KeyAction(kb.Inbox.VisualMode)},
			tui.PaletteCommand{Title: "Archive selected", Hint: kb.Inbox.Archive, Keywords: "file store", Action: KeyAction(kb.Inbox.Archive)},
			tui.PaletteCommand{Title: "Delete selected", Hint: kb.Inbox.Delete, Keywords: "trash remove", Action: KeyAction(kb.Inbox.Delete)},
			tui.PaletteCommand{Title: "Move to folder", Hint: kb.Folder.Move, Keywords: "file relocate", Action: KeyAction(kb.Folder.Move)},
		)
		if fi, ok := current.(*tui.FolderInbox); ok && fi.HasSplitPreview() {
			if ev := fi.GetPreviewPane(); ev != nil {
				cmds = append(cmds, EmailExportCommands(ev, folderInbox)...)
			}
		}
	}

	cmds = append(cmds,
		tui.PaletteCommand{Title: "Compose new email", Keywords: "write new mail send", Action: func() tea.Msg { return tui.GoToSendMsg{} }},
		tui.PaletteCommand{Title: "Go to Inbox", Keywords: "mail folders", Action: func() tea.Msg { return tui.GoToInboxMsg{} }},
		tui.PaletteCommand{Title: "Drafts", Keywords: "saved unsent", Action: func() tea.Msg { return tui.GoToDraftsMsg{} }},
		tui.PaletteCommand{Title: "Plugin marketplace", Keywords: "plugins install extensions", Action: func() tea.Msg { return tui.GoToMarketplaceMsg{} }},
		tui.PaletteCommand{Title: "Settings", Keywords: "preferences config accounts theme", Action: func() tea.Msg { return tui.GoToSettingsMsg{} }},
		tui.PaletteCommand{Title: "Main menu", Keywords: "home start choice", Action: func() tea.Msg { return tui.GoToChoiceMenuMsg{} }},
		tui.PaletteCommand{Title: "Quit Matcha", Keywords: "exit close", Action: tea.Quit},
	)
	return cmds
}

// EmailExportCommands builds export/open-in-browser commands for an email view.
func EmailExportCommands(ev *tui.EmailView, folderInbox *tui.FolderInbox) []tui.PaletteCommand {
	email := ev.GetEmail()
	accountID := ev.GetAccountID()
	folderName := folderNameFromInbox(folderInbox)

	exportAction := func(format string) func() tea.Msg {
		return func() tea.Msg {
			return tui.GoToSaveFilePickerMsg{
				Email:         email,
				Account:       accountID,
				Folder:        folderName,
				Mailbox:       ev.GetMailbox(),
				Format:        format,
				SuggestedName: export.SuggestFilename(email.Subject, format),
			}
		}
	}
	openInBrowserAction := func() tea.Msg {
		return tui.OpenEmailInBrowserMsg{
			Email:   email,
			Account: accountID,
			Folder:  folderName,
		}
	}

	return []tui.PaletteCommand{
		{Title: "Export as HTML", Keywords: "export save html file", Action: exportAction("html")},
		{Title: "Export as Markdown", Keywords: "export save markdown md file", Action: exportAction("markdown")},
		{Title: "Open in browser", Keywords: "open browser web view original", Action: openInBrowserAction},
	}
}

func folderNameFromInbox(folderInbox *tui.FolderInbox) string {
	if folderInbox != nil {
		return folderInbox.GetCurrentFolder()
	}
	return "INBOX"
}

// KeyAction returns a palette action that replays a keybinding as a synthetic key press.
func KeyAction(binding string) func() tea.Msg {
	if binding == "" {
		return nil
	}
	k := KeyMsgFromBinding(binding)
	return func() tea.Msg { return k }
}

var namedKeyCodes = map[string]rune{
	"tab":       tea.KeyTab,
	"enter":     tea.KeyEnter,
	"return":    tea.KeyEnter,
	"esc":       tea.KeyEscape,
	"escape":    tea.KeyEscape,
	"space":     tea.KeySpace,
	"backspace": tea.KeyBackspace,
	"delete":    tea.KeyDelete,
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
	"home":      tea.KeyHome,
	"end":       tea.KeyEnd,
	"pgup":      tea.KeyPgUp,
	"pgdown":    tea.KeyPgDown,
}

// KeyMsgFromBinding turns a keybinding string into a synthetic key press.
func KeyMsgFromBinding(s string) tea.KeyPressMsg {
	parts := strings.Split(s, "+")
	base := parts[len(parts)-1]

	var mod tea.KeyMod
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "ctrl":
			mod |= tea.ModCtrl
		case "alt", "opt", "option":
			mod |= tea.ModAlt
		case "shift":
			mod |= tea.ModShift
		case "meta", "cmd", "command":
			mod |= tea.ModMeta
		case "super", "win":
			mod |= tea.ModSuper
		case "hyper":
			mod |= tea.ModHyper
		}
	}

	if code, ok := namedKeyCodes[base]; ok {
		return tea.KeyPressMsg{Code: code, Mod: mod}
	}

	r := []rune(base)
	if len(r) == 0 {
		return tea.KeyPressMsg{Mod: mod}
	}
	km := tea.KeyPressMsg{Code: r[0], Mod: mod}
	if mod == 0 {
		km.Text = base
	}
	return km
}
