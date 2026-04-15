package notify

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
)

// ErrNotAvailable is returned when the required notification tool is not
// installed on the system (notify-send on Linux, osascript on macOS).
var ErrNotAvailable = errors.New("desktop notification tool not found")

// Send delivers a desktop notification with the given title and body.
// On macOS it uses osascript; on Linux it uses notify-send.
// Returns ErrNotAvailable if the notification binary cannot be found.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("osascript"); err != nil {
			return fmt.Errorf("%w: osascript: %v", ErrNotAvailable, err)
		}
		script := fmt.Sprintf(`display notification %q with title %q sound name "default"`, body, title)
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return fmt.Errorf("%w: notify-send: %v", ErrNotAvailable, err)
		}
		return exec.Command("notify-send", title, body).Run()
	default:
		return nil
	}
}
