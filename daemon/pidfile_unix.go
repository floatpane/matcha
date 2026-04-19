//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// WritePID writes the current process ID to the given path.
func WritePID(path string) error {
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0644)
}

// ReadPID reads the process ID from the given path.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID file: %w", err)
	}
	return pid, nil
}

// IsRunning checks if a daemon process is alive using the PID file.
func IsRunning(path string) (int, bool) {
	pid, err := ReadPID(path)
	if err != nil {
		return 0, false
	}
	// Signal 0 checks if process exists without sending a signal.
	err = syscall.Kill(pid, 0)
	return pid, err == nil
}

// RemovePID removes the PID file.
func RemovePID(path string) error {
	return os.Remove(path)
}
