package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateKeybinds_NoConflicts(t *testing.T) {
	kb := defaultKeybinds()
	conflicts := ValidateKeybinds(kb)
	if len(conflicts) != 0 {
		t.Errorf("default keybinds have conflicts: %v", conflicts)
	}
}

func TestValidateKeybinds_InboxConflict(t *testing.T) {
	kb := defaultKeybinds()
	kb.Inbox.Archive = kb.Inbox.Delete // same key as delete
	conflicts := ValidateKeybinds(kb)
	if len(conflicts) == 0 {
		t.Fatal("expected conflict, got none")
	}
	found := false
	for _, c := range conflicts {
		if strings.Contains(c, "inbox") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected inbox conflict, got: %v", conflicts)
	}
}

func TestValidateKeybinds_CrossAreaNotConflict(t *testing.T) {
	kb := defaultKeybinds()
	// "d" is delete in both inbox and email — intentional, not a conflict
	kb.Inbox.Delete = "d"
	kb.Email.Delete = "d"
	conflicts := ValidateKeybinds(kb)
	if len(conflicts) != 0 {
		t.Errorf("cross-area duplicates should not be conflicts: %v", conflicts)
	}
}

func TestValidateKeybinds_EmptyKeySkipped(t *testing.T) {
	kb := defaultKeybinds()
	kb.Drafts.Delete = ""
	kb.Drafts.Open = ""
	conflicts := ValidateKeybinds(kb)
	if len(conflicts) != 0 {
		t.Errorf("empty keys should not produce conflicts: %v", conflicts)
	}
}

func TestLoadKeybindsFromDir_WritesDefault(t *testing.T) {
	dir := t.TempDir()
	if err := LoadKeybindsFromDir(dir); err != nil {
		t.Fatalf("LoadKeybindsFromDir: %v", err)
	}
	if Keybinds.Inbox.Delete == "" {
		t.Error("expected inbox.delete to be set after loading defaults")
	}
}

func TestLoadKeybindsFromDir_ParsesCustom(t *testing.T) {
	dir := t.TempDir()
	// Write defaults first
	if err := LoadKeybindsFromDir(dir); err != nil {
		t.Fatalf("write defaults: %v", err)
	}

	// Override inbox delete key
	custom := `{"inbox":{"delete":"x","archive":"a","refresh":"r","open":"enter","next_tab":"l","prev_tab":"h","visual_mode":"v"},"global":{"quit":"ctrl+c","cancel":"esc","nav_up":"k","nav_down":"j"},"email":{"reply":"r","forward":"f","delete":"d","archive":"a","toggle_images":"i","rsvp_accept":"1","rsvp_decline":"2","rsvp_tentative":"3","focus_attachments":"tab"},"composer":{"external_editor":"ctrl+e","next_field":"tab","prev_field":"shift+tab"},"folder":{"next_folder":"tab","prev_folder":"shift+tab","move":"m","focus_preview":"]","focus_inbox":"["},"drafts":{"open":"enter","delete":"d"}}`
	if err := os.WriteFile(filepath.Join(dir, "keybinds.json"), []byte(custom), 0600); err != nil {
		t.Fatalf("write custom: %v", err)
	}
	if err := LoadKeybindsFromDir(dir); err != nil {
		t.Fatalf("LoadKeybindsFromDir custom: %v", err)
	}
	if Keybinds.Inbox.Delete != "x" {
		t.Errorf("expected inbox.delete=x, got %q", Keybinds.Inbox.Delete)
	}
}
