package config

import (
	_ "embed"
	"encoding/json"

	keybind "github.com/floatpane/go-keybind"
)

const keyDelete = "delete" // used in ValidateKeybinds action map keys

//go:embed default_keybinds.json
var defaultKeybindsJSON []byte

// Keybinds is the active keybind configuration. Initialized to defaults at
// package init; overwritten by LoadKeybindsFromDir when config is loaded.
var Keybinds = defaultKeybinds()

// KeybindsConfig holds all configurable key bindings organized by area.
type KeybindsConfig struct {
	Global   GlobalKeys   `json:"global"`
	Inbox    InboxKeys    `json:"inbox"`
	Email    EmailKeys    `json:"email"`
	Composer ComposerKeys `json:"composer"`
	Folder   FolderKeys   `json:"folder"`
	Drafts   DraftsKeys   `json:"drafts"`
}

type GlobalKeys struct {
	Quit           string `json:"quit"`
	Cancel         string `json:"cancel"`
	NavUp          string `json:"nav_up"`
	NavDown        string `json:"nav_down"`
	CommandPalette string `json:"command_palette"`
}

type InboxKeys struct {
	VisualMode     string `json:"visual_mode"`
	ToggleThreaded string `json:"toggle_threaded"`
	Delete         string `json:"delete"`
	Archive        string `json:"archive"`
	Refresh        string `json:"refresh"`
	Search         string `json:"search"`
	Filter         string `json:"filter"`
	Open           string `json:"open"`
	NextTab        string `json:"next_tab"`
	PrevTab        string `json:"prev_tab"`
}

type EmailKeys struct {
	Reply            string `json:"reply"`
	Forward          string `json:"forward"`
	Delete           string `json:"delete"`
	Archive          string `json:"archive"`
	ToggleImages     string `json:"toggle_images"`
	RsvpAccept       string `json:"rsvp_accept"`
	RsvpDecline      string `json:"rsvp_decline"`
	RsvpTentative    string `json:"rsvp_tentative"`
	FocusAttachments string `json:"focus_attachments"`
}

type ComposerKeys struct {
	ExternalEditor string `json:"external_editor"`
	NextField      string `json:"next_field"`
	PrevField      string `json:"prev_field"`
	Delete         string `json:"delete"`
	SpellNext      string `json:"spell_next"`
	SpellPrev      string `json:"spell_prev"`
	SpellAccept    string `json:"spell_accept"`
	SpellDismiss   string `json:"spell_dismiss"`
}

type FolderKeys struct {
	NextFolder   string `json:"next_folder"`
	PrevFolder   string `json:"prev_folder"`
	Move         string `json:"move"`
	FocusPreview string `json:"focus_preview"`
	FocusInbox   string `json:"focus_inbox"`
}

type DraftsKeys struct {
	Open   string `json:"open"`
	Delete string `json:"delete"`
}

func defaultKeybinds() KeybindsConfig {
	var kb KeybindsConfig
	if err := json.Unmarshal(defaultKeybindsJSON, &kb); err != nil {
		panic("matcha: malformed default_keybinds.json: " + err.Error())
	}
	return kb
}

// LoadKeybindsFromDir reads keybinds.json from cfgDir, writing defaults if
// the file does not exist, then updates the package-level Keybinds var.
func LoadKeybindsFromDir(cfgDir string) error {
	kb, err := keybind.Load(cfgDir, "keybinds.json", defaultKeybinds())
	if err != nil {
		return err
	}
	Keybinds = kb
	return nil
}

// ValidateKeybinds returns a list of conflict descriptions where two different
// actions within the same area are mapped to the same key. Cross-area
// duplicates are intentional (e.g. "d" = delete in both inbox and email view).
func ValidateKeybinds(kb KeybindsConfig) []string {
	return keybind.Validate(map[string]map[string]string{
		"global": {
			"quit":            kb.Global.Quit,
			"cancel":          kb.Global.Cancel,
			"nav_up":          kb.Global.NavUp,
			"nav_down":        kb.Global.NavDown,
			"command_palette": kb.Global.CommandPalette,
		},
		"inbox": {
			"visual_mode":     kb.Inbox.VisualMode,
			"toggle_threaded": kb.Inbox.ToggleThreaded,
			keyDelete:         kb.Inbox.Delete,
			"archive":         kb.Inbox.Archive,
			"refresh":         kb.Inbox.Refresh,
			"search":          kb.Inbox.Search,
			"filter":          kb.Inbox.Filter,
			"open":            kb.Inbox.Open,
			"next_tab":        kb.Inbox.NextTab,
			"prev_tab":        kb.Inbox.PrevTab,
		},
		"email": {
			"reply":             kb.Email.Reply,
			"forward":           kb.Email.Forward,
			keyDelete:           kb.Email.Delete,
			"archive":           kb.Email.Archive,
			"toggle_images":     kb.Email.ToggleImages,
			"rsvp_accept":       kb.Email.RsvpAccept,
			"rsvp_decline":      kb.Email.RsvpDecline,
			"rsvp_tentative":    kb.Email.RsvpTentative,
			"focus_attachments": kb.Email.FocusAttachments,
		},
		"composer": {
			"external_editor": kb.Composer.ExternalEditor,
			"next_field":      kb.Composer.NextField,
			"prev_field":      kb.Composer.PrevField,
			keyDelete:         kb.Composer.Delete,
			// spell_* bindings intentionally excluded — spell_accept reusing
			// "tab" with next_field and spell_dismiss reusing "esc" with cancel
			// are deliberate: the spellcheck popup intercepts before those handlers.
		},
		"folder": {
			"next_folder":   kb.Folder.NextFolder,
			"prev_folder":   kb.Folder.PrevFolder,
			"move":          kb.Folder.Move,
			"focus_preview": kb.Folder.FocusPreview,
			"focus_inbox":   kb.Folder.FocusInbox,
		},
		"drafts": {
			"open":    kb.Drafts.Open,
			keyDelete: kb.Drafts.Delete,
		},
	})
}
