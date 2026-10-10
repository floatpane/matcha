package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/floatpane/matcha/config"
)

// setKeybinds installs a modified copy of the active keybind config for the
// duration of a test and restores it afterwards.
func setKeybinds(t *testing.T, customize func(kb *config.KeybindsConfig)) {
	t.Helper()
	previous := config.Keybinds
	t.Cleanup(func() { config.Keybinds = previous })

	kb := previous
	customize(&kb)
	config.Keybinds = kb
}

func TestEmailHelpShowsCustomKeys(t *testing.T) {
	setKeybinds(t, func(kb *config.KeybindsConfig) {
		kb.Email.Reply = "q"
		kb.Email.Delete = "x"
		kb.Email.FocusAttachments = "g"
		kb.Global.Cancel = "ctrl+g"
	})

	help := (&EmailView{}).renderHelp()
	for _, want := range []string{"q: reply", "x: delete", "g: focus attachments", "ctrl+g: back to inbox"} {
		if !strings.Contains(help, want) {
			t.Errorf("email help missing %q, got %q", want, help)
		}
	}
	for _, unwanted := range []string{"\uf112 r:", "\uea81 d:", "\uf435 tab:", "\ueb06 esc:"} {
		if strings.Contains(help, unwanted) {
			t.Errorf("email help still shows default %q, got %q", unwanted, help)
		}
	}
}

func TestEmailAttachmentHelpShowsCustomKeys(t *testing.T) {
	setKeybinds(t, func(kb *config.KeybindsConfig) {
		kb.Global.NavUp = "p"
		kb.Global.NavDown = "n"
	})

	m := &EmailView{focusOnAttachments: true}
	help := m.renderHelp()
	if !strings.Contains(help, "↑/↓/p/n: navigate") {
		t.Errorf("attachment help missing remapped navigation, got %q", help)
	}
}

func TestEmailHelpKeepsDefaultWording(t *testing.T) {
	setKeybinds(t, func(_ *config.KeybindsConfig) {})

	help := (&EmailView{}).renderHelp()
	want := "\uf112 r: reply • \uf064 shift+r: reply all • \uf064 f: forward • \uea81 d: delete • \uea98 a: archive • \uf435 tab: focus attachments • \ueb06 esc: back to inbox"
	if !strings.Contains(help, want) {
		t.Errorf("default email help changed shape:\ngot:  %q\nwant: %q", help, want)
	}
}

func TestDraftsHelpShowsCustomKeys(t *testing.T) {
	setKeybinds(t, func(kb *config.KeybindsConfig) {
		kb.Drafts.Open = "o"
		kb.Drafts.Delete = "shift+d"
	})

	bindings := NewDrafts(nil).list.AdditionalShortHelpKeys()
	got := make([]string, 0, len(bindings))
	for _, b := range bindings {
		got = append(got, b.Help().Key+": "+b.Help().Desc)
	}
	joined := strings.Join(got, " • ")
	if !strings.Contains(joined, "\ue5fe o: open") || !strings.Contains(joined, "\uea81 shift+d: delete") {
		t.Errorf("drafts help missing custom keys, got %q", joined)
	}
}

func TestFolderHelpShowsCustomKeys(t *testing.T) {
	setKeybinds(t, func(kb *config.KeybindsConfig) {
		kb.Folder.NextFolder = "ctrl+f"
		kb.Folder.PrevFolder = "ctrl+b"
	})

	m := &FolderInbox{inbox: &Inbox{}}
	m.updateHelpKeys()
	if len(m.inbox.extraShortHelpKeys) != 2 {
		t.Fatalf("expected 2 folder help keys, got %d", len(m.inbox.extraShortHelpKeys))
	}
	first := m.inbox.extraShortHelpKeys[0]
	if first.Help().Key != "ctrl+f" || !slices.Contains(first.Keys(), "ctrl+f") {
		t.Errorf("next folder help key = %q, want the configured binding", first.Help().Key)
	}
	if second := m.inbox.extraShortHelpKeys[1]; second.Help().Key != "ctrl+b" {
		t.Errorf("prev folder help key = %q, want %q", second.Help().Key, "ctrl+b")
	}
}

func TestKeybindHelpFillsTranslatedPlaceholders(t *testing.T) {
	setKeybinds(t, func(kb *config.KeybindsConfig) {
		kb.Composer.NextField = "ctrl+j"
		kb.Composer.PrevField = "ctrl+k"
		kb.Global.Cancel = "ctrl+g"
	})

	help := keybindHelp("composer.help")
	if !strings.Contains(help, "ctrl+j/ctrl+k: navigate") || !strings.Contains(help, "ctrl+g: save draft & exit") {
		t.Errorf("composer help missing custom keys, got %q", help)
	}
	if strings.Contains(help, "tab/shift+tab") || strings.Contains(help, "{") {
		t.Errorf("composer help shows defaults or unfilled placeholders: %q", help)
	}
}

func TestJoinKeysDropsDuplicates(t *testing.T) {
	if got := joinKeys("↑", "↓", "↑"); got != "↑/↓" {
		t.Errorf("joinKeys dedup = %q, want %q", got, "↑/↓")
	}
	if got := joinKeys("a", "", "a", "b"); got != "a/b" {
		t.Errorf("joinKeys = %q, want %q", got, "a/b")
	}
}
