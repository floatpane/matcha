package macos

import (
	_ "embed"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

//go:embed file_picker.swift
var filePickerSwift string

// OpenFilePicker launches the native macOS file picker.
// It returns a list of selected absolute file paths.
func OpenFilePicker(initialPath string) ([]string, error) {
	if runtime.GOOS != goosDarwin {
		return nil, fmt.Errorf("OpenFilePicker is only supported on macOS")
	}

	binFile, err := compiledHelper("file_picker", filePickerSwift)
	if err != nil {
		return nil, err
	}

	// Run
	args := []string{}
	if initialPath != "" {
		args = append(args, initialPath)
	}
	out, err := exec.Command(binFile, args...).Output() //nolint:noctx
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Exit code 1 usually means user cancelled
			return nil, nil
		}
		return nil, fmt.Errorf("failed to run picker helper: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}

	paths := strings.Split(trimmed, "\n")
	return paths, nil
}
