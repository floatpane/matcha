//go:build windows

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

	// On Windows, syscall.Kill is not available. We use OpenProcess instead.
	// We only need PROCESS_QUERY_LIMITED_INFORMATION (0x1000)
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Process could not be opened, which likely means it doesn't exist
		return pid, false
	}
	defer syscall.CloseHandle(h)

	// Check if the process is still running or has exited
	var exitCode uint32
	err = syscall.GetExitCodeProcess(h, &exitCode)
	if err != nil {
		return pid, false
	}

	// STILL_ACTIVE is 259
	return pid, exitCode == 259
}

// RemovePID removes the PID file.
func RemovePID(path string) error {
	return os.Remove(path)
}
