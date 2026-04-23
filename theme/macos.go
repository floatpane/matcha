package theme

import (
	"runtime"

	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/clib/macos"
)

// SyncWithMacOS updates the 'Native' theme with current macOS system appearance.
func SyncWithMacOS() error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	appearance, err := macos.GetAppearance()
	if err != nil {
		return err
	}

	// Update Native theme
	Native.Accent = lipgloss.Color(appearance.AccentColor)
	Native.Directory = lipgloss.Color(appearance.AccentColor)

	if appearance.DarkMode {
		// Dark mode specifics if needed
		Native.AccentText = lipgloss.Color("#FFFDF5")
		Native.Contrast = lipgloss.Color("#000000")
	} else {
		// Light mode specifics
		Native.AccentText = lipgloss.Color("#000000")
		Native.Contrast = lipgloss.Color("#FFFFFF")
		Native.Secondary = lipgloss.Color("240")
	}

	// If the active theme is 'Native', update it immediately
	if ActiveTheme.Name == "Native" {
		ActiveTheme = Native
	}

	return nil
}
