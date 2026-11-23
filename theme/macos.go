package theme

import (
	"runtime"

	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/clib/macos"
)

// MacOSNativeTheme returns the 'Native' theme updated with the current macOS
// system appearance. It only reads global state, so it is safe to call from
// a background goroutine; apply the result with SetNative on the UI
// goroutine (the renderer reads ActiveTheme without synchronization).
func MacOSNativeTheme() (Theme, error) {
	t := Native
	if runtime.GOOS != "darwin" {
		return t, nil
	}

	appearance, err := macos.GetAppearance()
	if err != nil {
		return t, err
	}

	t.Accent = lipgloss.Color(appearance.AccentColor)
	t.Directory = lipgloss.Color(appearance.AccentColor)

	if appearance.DarkMode {
		// Dark mode specifics if needed
		t.AccentText = lipgloss.Color("#FFFDF5")
		t.Contrast = lipgloss.Color("#000000")
	} else {
		// Light mode specifics
		t.AccentText = lipgloss.Color("#000000")
		t.Contrast = lipgloss.Color("#FFFFFF")
		t.Secondary = lipgloss.Color("240")
	}
	return t, nil
}

// SetNative installs t as the 'Native' theme and, when Native is active,
// as the active theme. Call it on the UI goroutine (or before the program
// starts) and rebuild styles afterwards.
func SetNative(t Theme) {
	Native = t
	// BuiltinThemes holds a copy taken at init; keep it in sync so picking
	// "Native" later in the settings uses the system colors too.
	for i := range BuiltinThemes {
		if BuiltinThemes[i].Name == Native.Name {
			BuiltinThemes[i] = Native
		}
	}
	if ActiveTheme.Name == "Native" {
		ActiveTheme = Native
	}
}

// SyncWithMacOS updates the 'Native' theme with current macOS system appearance.
//
// It writes ActiveTheme, which the renderer reads without synchronization, so
// it must not run concurrently with the TUI: call it before the program
// starts, or use MacOSNativeTheme in the background and SetNative on the UI
// goroutine.
func SyncWithMacOS() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	t, err := MacOSNativeTheme()
	if err != nil {
		return err
	}
	SetNative(t)
	return nil
}
