package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/theme"
)

func TestSettingsNavigationWraps(t *testing.T) {
	settings := NewSettings(&config.Config{
		Accounts: []config.Account{
			{ID: "account-1", Email: "one@example.com"},
			{ID: "account-2", Email: "two@example.com"},
		},
		MailingLists: []config.MailingList{
			{Name: "List One"},
			{Name: "List Two"},
		},
	})

	t.Run("menu", func(t *testing.T) {
		settings.menuCursor = 0
		model, _ := settings.updateMenu(tea.KeyPressMsg{Code: tea.KeyUp})
		settings = model.(*Settings)
		if settings.menuCursor != int(CategoryPlugins) {
			t.Fatalf("up from first menu item should wrap to last, got %d", settings.menuCursor)
		}

		model, _ = settings.updateMenu(tea.KeyPressMsg{Code: tea.KeyDown})
		settings = model.(*Settings)
		if settings.menuCursor != 0 {
			t.Fatalf("down from last menu item should wrap to first, got %d", settings.menuCursor)
		}
	})

	t.Run("general", func(t *testing.T) {
		settings.generalCursor = 0
		last := len(settings.buildGeneralOptions()) - 1

		model, _ := settings.updateGeneral(tea.KeyPressMsg{Code: tea.KeyUp})
		settings = model.(*Settings)
		if settings.generalCursor != last {
			t.Fatalf("up from first general item should wrap to last, got %d", settings.generalCursor)
		}

		model, _ = settings.updateGeneral(tea.KeyPressMsg{Code: tea.KeyDown})
		settings = model.(*Settings)
		if settings.generalCursor != 0 {
			t.Fatalf("down from last general item should wrap to first, got %d", settings.generalCursor)
		}
	})

	t.Run("accounts", func(t *testing.T) {
		settings.accountsCursor = 0
		last := len(settings.cfg.Accounts)

		model, _ := settings.updateAccounts(tea.KeyPressMsg{Code: tea.KeyUp})
		settings = model.(*Settings)
		if settings.accountsCursor != last {
			t.Fatalf("up from first account item should wrap to add account, got %d", settings.accountsCursor)
		}

		model, _ = settings.updateAccounts(tea.KeyPressMsg{Code: tea.KeyDown})
		settings = model.(*Settings)
		if settings.accountsCursor != 0 {
			t.Fatalf("down from add account should wrap to first, got %d", settings.accountsCursor)
		}
	})

	t.Run("mailing lists", func(t *testing.T) {
		settings.listsCursor = 0
		last := len(settings.cfg.MailingLists)

		model, _ := settings.updateMailingLists(tea.KeyPressMsg{Code: tea.KeyUp})
		settings = model.(*Settings)
		if settings.listsCursor != last {
			t.Fatalf("up from first mailing list should wrap to add list, got %d", settings.listsCursor)
		}

		model, _ = settings.updateMailingLists(tea.KeyPressMsg{Code: tea.KeyDown})
		settings = model.(*Settings)
		if settings.listsCursor != 0 {
			t.Fatalf("down from add list should wrap to first, got %d", settings.listsCursor)
		}
	})

	t.Run("theme", func(t *testing.T) {
		themes := theme.AllThemes()
		if len(themes) < 2 {
			t.Skip("need at least two themes to test wrap-around")
		}

		settings.themeCursor = 0
		model, _ := settings.updateTheme(tea.KeyPressMsg{Code: tea.KeyUp})
		settings = model.(*Settings)
		if settings.themeCursor != len(themes)-1 {
			t.Fatalf("up from first theme should wrap to last, got %d", settings.themeCursor)
		}

		model, _ = settings.updateTheme(tea.KeyPressMsg{Code: tea.KeyDown})
		settings = model.(*Settings)
		if settings.themeCursor != 0 {
			t.Fatalf("down from last theme should wrap to first, got %d", settings.themeCursor)
		}
	})
}

func TestSettingsHorizontalPaneFocus(t *testing.T) {
	t.Run("right moves focus from menu to content", func(t *testing.T) {
		settings := NewSettings(&config.Config{})
		settings.activePane = PaneMenu
		settings.menuCursor = int(CategoryGeneral)

		model, _ := settings.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		settings = model.(*Settings)

		if settings.activePane != PaneContent {
			t.Fatalf("right from menu pane should focus content, got %d", settings.activePane)
		}
	})

	t.Run("esc moves focus from content to menu", func(t *testing.T) {
		settings := NewSettings(&config.Config{})
		settings.activePane = PaneContent
		settings.activeCategory = CategoryGeneral
		settings.menuCursor = int(CategoryGeneral)

		model, _ := settings.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		settings = model.(*Settings)

		if settings.activePane != PaneMenu {
			t.Fatalf("esc from content pane should focus menu, got %d", settings.activePane)
		}
	})

	t.Run("left moves focus from content to menu", func(t *testing.T) {
		settings := NewSettings(&config.Config{})
		settings.activePane = PaneContent
		settings.activeCategory = CategoryGeneral
		settings.menuCursor = int(CategoryGeneral)

		model, _ := settings.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		settings = model.(*Settings)

		if settings.activePane != PaneMenu {
			t.Fatalf("left from content pane should focus menu, got %d", settings.activePane)
		}
	})

	t.Run("left does not exit settings from menu", func(t *testing.T) {
		settings := NewSettings(&config.Config{})
		settings.activePane = PaneMenu

		model, cmd := settings.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		settings = model.(*Settings)

		if cmd != nil {
			t.Fatal("left from menu pane should not return to choice menu")
		}
		if settings.activePane != PaneMenu {
			t.Fatalf("left from menu pane should keep menu focused, got %d", settings.activePane)
		}
	})
}

func TestSettingsEncryptionLeftKeyInInput(t *testing.T) {
	t.Run("at input start returns to menu", func(t *testing.T) {
		settings := NewSettings(&config.Config{})
		settings.activePane = PaneContent
		settings.activeCategory = CategoryEncryption
		settings.encFocusIndex = 0
		settings.encPasswordInput.SetValue("secret")
		settings.encPasswordInput.SetCursor(0)
		settings.encPasswordInput.Focus()

		model, _ := settings.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		settings = model.(*Settings)

		if settings.activePane != PaneMenu {
			t.Fatalf("left at start of encryption input should focus menu, got %d", settings.activePane)
		}
		if settings.encPasswordInput.Value() != "" {
			t.Fatal("left at start of encryption input should clear input like esc")
		}
	})

	t.Run("inside input moves cursor", func(t *testing.T) {
		settings := NewSettings(&config.Config{})
		settings.activePane = PaneContent
		settings.activeCategory = CategoryEncryption
		settings.encFocusIndex = 0
		settings.encPasswordInput.SetValue("secret")
		settings.encPasswordInput.SetCursor(1)
		settings.encPasswordInput.Focus()

		model, _ := settings.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		settings = model.(*Settings)

		if settings.activePane != PaneContent {
			t.Fatalf("left inside encryption input should keep content focused, got %d", settings.activePane)
		}
		if settings.encPasswordInput.Position() != 0 {
			t.Fatalf("left inside encryption input should move cursor left, got position %d", settings.encPasswordInput.Position())
		}
	})
}

func TestFilePickerNavigationWraps(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}

	picker := NewFilePicker(dir)
	if len(picker.items) != 2 {
		t.Fatalf("expected two picker items, got %d", len(picker.items))
	}

	model, _ := picker.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	picker = model.(*FilePicker)
	if picker.cursor != len(picker.items)-1 {
		t.Fatalf("up from first file should wrap to last, got %d", picker.cursor)
	}

	model, _ = picker.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	picker = model.(*FilePicker)
	if picker.cursor != 0 {
		t.Fatalf("down from last file should wrap to first, got %d", picker.cursor)
	}
}

func TestFilePickerNavigationEmptyDirectoryDoesNotWrap(t *testing.T) {
	picker := NewFilePicker(t.TempDir())
	if len(picker.items) != 0 {
		t.Fatalf("expected empty picker, got %d items", len(picker.items))
	}

	model, _ := picker.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	picker = model.(*FilePicker)
	if picker.cursor != 0 {
		t.Fatalf("empty picker cursor should remain zero after up, got %d", picker.cursor)
	}

	model, _ = picker.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	picker = model.(*FilePicker)
	if picker.cursor != 0 {
		t.Fatalf("empty picker cursor should remain zero after down, got %d", picker.cursor)
	}
}
