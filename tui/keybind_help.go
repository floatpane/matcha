package tui

import (
	"strings"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/i18n"
)

// helpSeparator joins the fragments of every bottom help bar.
const helpSeparator = " • "

// kbKey returns the configured key for an action. Remapped keys are shown and
// matched identically; an empty entry falls back to the built-in default so a
// help bar never renders a blank key.
func kbKey(key, def string) string {
	if strings.TrimSpace(key) == "" {
		return def
	}
	return key
}

// helpEntry is a single "icon key: description" fragment of a help bar.
type helpEntry struct {
	icon string
	key  string
	desc string
}

// helpItem builds one help fragment. Pass an empty icon for fragments without
// a nerd-font glyph.
func helpItem(icon, key, desc string) helpEntry {
	return helpEntry{icon: icon, key: key, desc: desc}
}

func (e helpEntry) String() string {
	fragment := e.key + ": " + e.desc
	if e.icon != "" {
		fragment = e.icon + " " + fragment
	}
	return fragment
}

// joinHelp renders help fragments as one bullet separated line.
func joinHelp(entries ...helpEntry) string {
	fragments := make([]string, 0, len(entries))
	for _, entry := range entries {
		fragments = append(fragments, entry.String())
	}
	return strings.Join(fragments, helpSeparator)
}

// joinKeys merges key labels into one slash separated label, dropping empty
// and repeated entries so an unremapped binding keeps its original wording.
func joinKeys(keys ...string) string {
	seen := make(map[string]bool, len(keys))
	unique := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, key)
	}
	return strings.Join(unique, "/")
}

// navArrowsLabel describes up/down navigation. Arrow keys always work, so the
// default binding reads "↑/↓"; remapped keys are appended, e.g. "↑/↓/p/n".
func navArrowsLabel() string {
	kb := config.Keybinds
	up := kbKey(kb.Global.NavUp, "k")
	down := kbKey(kb.Global.NavDown, "j")
	if up == "k" && down == "j" {
		return "↑/↓"
	}
	return joinKeys("↑", "↓", up, down)
}

// navVimLabel describes up/down navigation by the configured keys themselves,
// matching the "j/k" wording used by the folder overlays.
func navVimLabel() string {
	kb := config.Keybinds
	return joinKeys(kbKey(kb.Global.NavDown, "j"), kbKey(kb.Global.NavUp, "k"))
}

// cancelKey returns the configured cancel/back key, used by help bars in every
// view that routes back navigation through the global bindings.
func cancelKey() string {
	return kbKey(config.Keybinds.Global.Cancel, "esc")
}

// keybindHelpData exposes the active keybinds as template variables, letting
// translated help strings show the user's own keys instead of the defaults.
func keybindHelpData() map[string]interface{} {
	kb := config.Keybinds
	return map[string]interface{}{
		"NavArrows":   navArrowsLabel(),
		"NavVim":      navVimLabel(),
		"Cancel":      kbKey(kb.Global.Cancel, "esc"),
		"Quit":        kbKey(kb.Global.Quit, "ctrl+c"),
		"NextField":   kbKey(kb.Composer.NextField, "tab"),
		"PrevField":   kbKey(kb.Composer.PrevField, "shift+tab"),
		"Editor":      kbKey(kb.Composer.ExternalEditor, "ctrl+e"),
		"UndoSend":    kbKey(kb.Composer.UndoSend, "u"),
		"NextFolder":  kbKey(kb.Folder.NextFolder, "tab"),
		"PrevFolder":  kbKey(kb.Folder.PrevFolder, "shift+tab"),
		"Move":        kbKey(kb.Folder.Move, "m"),
		"FocusFolder": joinKeys(kbKey(kb.Folder.FocusPreview, "]"), kbKey(kb.Folder.FocusInbox, "[")),
	}
}

// keybindHelp resolves a help string through i18n (plugin overrides included) and
// fills its {Placeholder} slots with the active keybinds.
func keybindHelp(key string) string {
	return i18n.NewTemplate(t(key)).Execute(keybindHelpData())
}
