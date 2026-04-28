package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
	Quit    string `json:"quit"`
	Cancel  string `json:"cancel"`
	NavUp   string `json:"nav_up"`
	NavDown string `json:"nav_down"`
}

type InboxKeys struct {
	VisualMode string `json:"visual_mode"`
	Delete     string `json:"delete"`
	Archive    string `json:"archive"`
	Refresh    string `json:"refresh"`
	Open       string `json:"open"`
	NextTab    string `json:"next_tab"`
	PrevTab    string `json:"prev_tab"`
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
	path := filepath.Join(cfgDir, "keybinds.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("keybinds: read %s: %w", path, err)
		}
		// File missing — write defaults.
		if err := os.MkdirAll(cfgDir, 0700); err != nil {
			return fmt.Errorf("keybinds: mkdir %s: %w", cfgDir, err)
		}
		if err := os.WriteFile(path, defaultKeybindsJSON, 0600); err != nil {
			return fmt.Errorf("keybinds: write defaults to %s: %w", path, err)
		}
		Keybinds = defaultKeybinds()
		return nil
	}

	var kb KeybindsConfig
	if err := json.Unmarshal(data, &kb); err != nil {
		return fmt.Errorf("keybinds: parse %s: %w", path, err)
	}
	Keybinds = kb
	return nil
}

// ValidateKeybinds returns a list of conflict descriptions where two different
// actions within the same area are mapped to the same key. Cross-area
// duplicates are intentional (e.g. "d" = delete in both inbox and email view).
func ValidateKeybinds(kb KeybindsConfig) []string {
	var conflicts []string

	check := func(area string, bindings map[string]string) {
		seen := make(map[string]string) // key → action name
		for action, key := range bindings {
			if key == "" {
				continue
			}
			if prev, ok := seen[key]; ok {
				conflicts = append(conflicts,
					fmt.Sprintf("conflict in %s: key %q used for both %q and %q", area, key, prev, action))
			} else {
				seen[key] = action
			}
		}
	}

	check("global", map[string]string{
		"quit":     kb.Global.Quit,
		"cancel":   kb.Global.Cancel,
		"nav_up":   kb.Global.NavUp,
		"nav_down": kb.Global.NavDown,
	})
	check("inbox", map[string]string{
		"visual_mode": kb.Inbox.VisualMode,
		"delete":      kb.Inbox.Delete,
		"archive":     kb.Inbox.Archive,
		"refresh":     kb.Inbox.Refresh,
		"open":        kb.Inbox.Open,
		"next_tab":    kb.Inbox.NextTab,
		"prev_tab":    kb.Inbox.PrevTab,
	})
	check("email", map[string]string{
		"reply":             kb.Email.Reply,
		"forward":           kb.Email.Forward,
		"delete":            kb.Email.Delete,
		"archive":           kb.Email.Archive,
		"toggle_images":     kb.Email.ToggleImages,
		"rsvp_accept":       kb.Email.RsvpAccept,
		"rsvp_decline":      kb.Email.RsvpDecline,
		"rsvp_tentative":    kb.Email.RsvpTentative,
		"focus_attachments": kb.Email.FocusAttachments,
	})
	check("composer", map[string]string{
		"external_editor": kb.Composer.ExternalEditor,
		"next_field":      kb.Composer.NextField,
		"prev_field":      kb.Composer.PrevField,
	})
	check("folder", map[string]string{
		"next_folder":   kb.Folder.NextFolder,
		"prev_folder":   kb.Folder.PrevFolder,
		"move":          kb.Folder.Move,
		"focus_preview": kb.Folder.FocusPreview,
		"focus_inbox":   kb.Folder.FocusInbox,
	})
	check("drafts", map[string]string{
		"open":   kb.Drafts.Open,
		"delete": kb.Drafts.Delete,
	})

	return conflicts
}
