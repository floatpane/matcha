package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Send delivers a desktop notification with the given title and body.
// On macOS it uses osascript; on Linux it uses notify-send.
//
// If the platform-specific helper binary is not on PATH (common on minimal
// Linux installs or Wayland-only setups that do not ship notify-send), Send
// returns an explicit error instead of silently swallowing the failure so
// callers can log or surface it in the UI. See #522.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("osascript"); err != nil {
			return fmt.Errorf("desktop notifications require osascript, which was not found on PATH: %w", err)
		}
		script := fmt.Sprintf(`display notification %q with title %q sound name "default"`, body, title)
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return fmt.Errorf("desktop notifications require notify-send (libnotify / notify-osd), which was not found on PATH: %w", err)
		}
		return exec.Command("notify-send", title, body).Run()
	default:
		return nil
	}
}
