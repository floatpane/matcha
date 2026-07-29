package notify

import (
	"os/exec"
	"runtime"
)

// Send delivers a desktop notification with the given title and body.
// On macOS it uses osascript; on Linux it uses notify-send.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("osascript", "-e", "display notification", "-e", body, "-e", "with title", "-e", title, "-e", "sound name \"default\"").Run() //nolint:noctx
	case "linux":
		return exec.Command("notify-send", title, body).Run() //nolint:noctx
	default:
		return nil
	}
}
