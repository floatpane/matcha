package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/config"
)

func settingsKey(s string) tea.KeyPressMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

// settingsRowOf returns the screen row of the first rendered line containing
// needle, or -1.
func settingsRowOf(t *testing.T, s *Settings, needle string) int {
	t.Helper()
	for i, line := range strings.Split(ansi.Strip(s.View().Content), "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	t.Fatalf("%q not found in settings view", needle)
	return -1
}

// TestSettingsDeletePromptDoesNotLeakAcrossCategories reproduces the crash
// where a delete prompt opened under Accounts survived esc and was rendered
// (and indexed) by the empty Mailing Lists page.
func TestSettingsDeletePromptDoesNotLeakAcrossCategories(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := NewSettings(&config.Config{
		Accounts: []config.Account{{ID: "a", Email: "a@example.com"}},
	})
	s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// The original crash: Settings → Accounts → d → esc → j j. Esc now
	// answers the prompt; a second esc returns to the menu, from where j j
	// previews the empty Mailing Lists page.
	for _, k := range []string{"j", "enter", "d", "esc", "j", "j", "esc", "j", "j"} {
		s.Update(settingsKey(k))
		_ = s.View() // must not panic
	}
	if s.activeCategory != CategoryMailingLists || s.activePane != PaneMenu {
		t.Fatalf("expected the Mailing Lists preview, got category %d pane %d", s.activeCategory, s.activePane)
	}
	if s.accountsConfirmingDelete || s.listsConfirmingDelete {
		t.Fatal("no delete prompt may survive leaving the page")
	}

	// Even a stale flag must not be able to index another category's list.
	s.accountsConfirmingDelete = true
	s.activePane = PaneContent
	_ = s.View()
	s.activePane = PaneMenu
	s.accountsConfirmingDelete = false

	// Opening the lists page and pressing y must not act on a prompt that
	// was opened elsewhere.
	s.cfg.MailingLists = []config.MailingList{{Name: "keep me"}}
	s.Update(settingsKey("enter"))
	s.Update(settingsKey("y"))
	if len(s.cfg.MailingLists) != 1 {
		t.Fatal("y on the mailing lists page deleted a list without a prompt")
	}
}

// TestSettingsEscCancelsPromptBeforeLeaving keeps esc on a pending prompt:
// it answers "no" instead of jumping to the menu with the prompt still set.
func TestSettingsEscCancelsPromptBeforeLeaving(t *testing.T) {
	s := NewSettings(&config.Config{
		Accounts: []config.Account{{ID: "a", Email: "a@example.com"}},
	})
	s.openCategory(CategoryAccounts)
	s.Update(settingsKey("d"))
	if !s.accountsConfirmingDelete {
		t.Fatal("d should open the delete prompt")
	}
	s.Update(settingsKey("esc"))
	if s.accountsConfirmingDelete {
		t.Fatal("esc should cancel the delete prompt")
	}
	if s.activePane != PaneContent {
		t.Fatal("esc on a prompt should stay on the page")
	}
}

// TestSettingsMenuClickOpensCategory checks the menu click rows against the
// rendered view (the first category used to be off by one) and that a click
// opens a category exactly like enter does.
func TestSettingsMenuClickOpensCategory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := NewSettings(&config.Config{})
	s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if got, want := s.categoriesStartY(), settingsRowOf(t, s, "▌ General"); got != want {
		t.Fatalf("categoriesStartY() = %d, but General is drawn on row %d", got, want)
	}

	row := settingsRowOf(t, s, "Accounts")
	s.Update(tea.MouseClickMsg{X: 5, Y: row, Button: tea.MouseLeft})
	if s.activeCategory != CategoryAccounts || s.activePane != PaneContent || s.menuCursor != int(CategoryAccounts) {
		t.Fatalf("click on Accounts opened category %d (pane %d, menu %d)", s.activeCategory, s.activePane, s.menuCursor)
	}

	// Like enter, a click on App Encryption focuses the password field.
	s.leaveContent()
	row = settingsRowOf(t, s, "App Encryption")
	s.Update(tea.MouseClickMsg{X: 5, Y: row, Button: tea.MouseLeft})
	if s.activeCategory != CategoryEncryption {
		t.Fatalf("click opened category %d, want encryption", s.activeCategory)
	}
	if !config.IsSecureModeEnabled() && !s.encPasswordInput.Focused() {
		t.Fatal("opening encryption by click should focus the password input")
	}
}

// TestSettingsContentClickSelectsVisibleRow clicks rows of a scrolled list:
// a click selects the row under the pointer, a double click activates it.
func TestSettingsContentClickSelectsVisibleRow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := NewSettings(&config.Config{})
	s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s.openCategory(CategoryGeneral)
	s.generalCursor = len(s.buildGeneralOptions()) - 1 // scrolls the list

	label := i18nT("settings_general.show_original_on_reply")
	row := settingsRowOf(t, s, label)
	s.Update(tea.MouseClickMsg{X: 40, Y: row, Button: tea.MouseLeft})
	if got := s.buildGeneralOptions()[s.generalCursor].labelKey; got != "settings_general.show_original_on_reply" {
		t.Fatalf("click selected %q", got)
	}

	accounts := NewSettings(&config.Config{Accounts: []config.Account{
		{ID: "a", Email: "a@example.com"},
		{ID: "b", Email: "b@example.com"},
	}})
	accounts.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	accounts.openCategory(CategoryAccounts)
	row = settingsRowOf(t, accounts, "b@example.com")
	accounts.Update(tea.MouseClickMsg{X: 40, Y: row, Button: tea.MouseLeft})
	if accounts.accountsCursor != 1 || accounts.isCryptoConfig {
		t.Fatalf("single click should only select (cursor %d, crypto %v)", accounts.accountsCursor, accounts.isCryptoConfig)
	}
	accounts.Update(tea.MouseClickMsg{X: 40, Y: row, Button: tea.MouseLeft})
	if !accounts.isCryptoConfig || accounts.editingAccountIdx != 1 {
		t.Fatal("double click should activate the account like enter")
	}
}

// TestSettingsViewFitsScreen renders every category at common sizes: the
// view must be exactly one screen tall, no row wider than the terminal, and
// the help bar must stay on screen.
func TestSettingsViewFitsScreen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &config.Config{Accounts: []config.Account{{ID: "a", Email: "a@example.com", Name: "Alice", FetchEmail: "a@example.com"}}}
	for _, size := range [][2]int{{80, 24}, {60, 20}} {
		for cat := 0; cat < settingsCategoryCount; cat++ {
			s := NewSettings(cfg)
			s.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			s.openCategory(SettingsCategory(cat))
			lines := strings.Split(ansi.Strip(s.View().Content), "\n")
			if len(lines) != size[1] {
				t.Errorf("%dx%d category %d: %d rows", size[0], size[1], cat, len(lines))
			}
			for i, line := range lines {
				if w := ansi.StringWidth(line); w > size[0] {
					t.Errorf("%dx%d category %d: row %d is %d wide", size[0], size[1], cat, i, w)
				}
			}
			if !strings.Contains(strings.Join(lines, "\n"), i18nT("settings.help_content")) {
				t.Errorf("%dx%d category %d: help bar not on screen", size[0], size[1], cat)
			}
		}
	}
}

// TestSettingsEncryptionSubmitsOnce locks the form while encryption runs and
// clears a stale error once it succeeds. The returned commands are never
// executed: they would enable secure mode for real.
func TestSettingsEncryptionSubmitsOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if config.IsSecureModeEnabled() {
		t.Skip("secure mode is enabled in this environment")
	}
	s := NewSettings(&config.Config{})
	s.openCategory(CategoryEncryption)
	s.encPasswordInput.SetValue("correct horse")
	s.encConfirmInput.SetValue("correct horse")
	s.encFocusIndex = 2

	if _, cmd := s.Update(settingsKey("enter")); cmd == nil || !s.encEnabling {
		t.Fatal("enter on the button should start encryption")
	}
	if _, cmd := s.Update(settingsKey("enter")); cmd != nil {
		t.Fatal("a second enter while encrypting must not submit again")
	}

	s.encError = "previous failure"
	s.Update(SecureModeEnabledMsg{})
	if s.encError != "" || s.encEnabling {
		t.Fatalf("success should clear the error and the busy state (err %q)", s.encError)
	}
}

// TestSettingsRestoreStateSyncsMenu keeps the menu highlight on the category
// that is shown after a restore (e.g. returning from the contact editor).
func TestSettingsRestoreStateSyncsMenu(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := NewSettings(&config.Config{})
	s.RestoreState(SettingsState{ActivePane: PaneContent, ActiveCategory: CategoryContacts})
	if s.menuCursor != int(CategoryContacts) {
		t.Fatalf("menu highlights %d while showing contacts", s.menuCursor)
	}
}
