package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func newTestMarketplace() Marketplace {
	RebuildStyles()
	return Marketplace{
		installed: map[string]bool{},
		width:     80,
		height:    24,
		state:     marketplaceReady,
		entries:   mpEntries(20, "zero"),
	}
}

func marketplaceRowOf(t *testing.T, mp Marketplace, needle string) int {
	t.Helper()
	for i, line := range strings.Split(ansi.Strip(mp.View().Content), "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	t.Fatalf("%q not rendered", needle)
	return -1
}

// TestMarketplaceClickMapsRenderedRows clicks entries with the default logo
// and with a plugin banner of another height, and clicks the footer, which
// must not select (or install) entries that are not on screen.
func TestMarketplaceClickMapsRenderedRows(t *testing.T) {
	old := uiProvider
	defer func() { uiProvider = old }()
	for _, banner := range []string{"", "ONE\nTWO\nTHREE"} {
		uiProvider = nil
		if banner != "" {
			uiProvider = &stubUIProvider{banner: banner}
		}
		mp := newTestMarketplace()

		row := marketplaceRowOf(t, mp, "Plugin otherc")
		model, _ := mp.Update(tea.MouseClickMsg{X: 6, Y: row, Button: tea.MouseLeft})
		mp = model.(Marketplace)
		if mp.cursor != 2 {
			t.Fatalf("banner %q: clicking otherc selected %d", banner, mp.cursor)
		}

		footer := marketplaceRowOf(t, mp, "/20")
		for y := footer; y < mp.height; y++ {
			model, _ = mp.Update(tea.MouseClickMsg{X: 6, Y: y, Button: tea.MouseLeft})
			mp = model.(Marketplace)
			model, _ = mp.Update(tea.MouseClickMsg{X: 6, Y: y, Button: tea.MouseLeft})
			mp = model.(Marketplace)
			if mp.cursor != 2 || mp.confirmingInstall {
				t.Fatalf("banner %q: footer row %d selected %d (install prompt %v)", banner, y, mp.cursor, mp.confirmingInstall)
			}
		}
	}
}

// TestMarketplaceMouseIgnoredBehindDialog keeps the entry a prompt is about
// from changing under it.
func TestMarketplaceMouseIgnoredBehindDialog(t *testing.T) {
	mp := newTestMarketplace()
	model, _ := mp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mp = model.(Marketplace)
	if !mp.confirmingInstall {
		t.Fatal("enter should ask to install")
	}
	model, _ = mp.Update(tea.MouseWheelMsg{X: 6, Y: 15, Button: tea.MouseWheelDown})
	mp = model.(Marketplace)
	model, _ = mp.Update(tea.MouseClickMsg{X: 6, Y: 15, Button: tea.MouseLeft})
	mp = model.(Marketplace)
	if mp.cursor != 0 || mp.installTarget.Name != "zero" {
		t.Fatalf("mouse moved the cursor behind the prompt (cursor %d, target %q)", mp.cursor, mp.installTarget.Name)
	}
}

// TestMarketplaceStatusClears drops "Installing…" on failure, expires result
// messages, and shows notifications in the status line when standalone.
func TestMarketplaceStatusClears(t *testing.T) {
	mp := newTestMarketplace()
	mp.confirmingInstall = true
	mp.installTarget = mp.entries[0]
	model, _ := mp.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	mp = model.(Marketplace)
	if mp.status == "" {
		t.Fatal("confirming should show progress")
	}

	model, _ = mp.Update(PluginInstalledMsg{Name: "zero", Err: errors.New("offline")})
	mp = model.(Marketplace)
	if mp.status != "" {
		t.Fatalf("a failed install left %q in the status line", mp.status)
	}

	model, cmd := mp.Update(PluginInstalledMsg{Name: "zero"})
	mp = model.(Marketplace)
	if mp.status == "" || cmd == nil {
		t.Fatal("success should show a message that expires")
	}
	model, _ = mp.Update(mpStatusClearMsg{seq: mp.statusSeq})
	mp = model.(Marketplace)
	if mp.status != "" {
		t.Fatal("the success message should expire")
	}

	mp.standalone = true
	model, _ = mp.Update(NotifyMsg{Message: "registry unavailable"})
	mp = model.(Marketplace)
	if mp.status != "registry unavailable" {
		t.Fatalf("standalone marketplace should show notifications, got %q", mp.status)
	}
}
