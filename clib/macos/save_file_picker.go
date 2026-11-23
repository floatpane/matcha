package macos

import (
	_ "embed"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

//go:embed save_file_picker.swift
var saveFilePickerSwift string

// SaveFilePicker launches the native macOS save panel (NSSavePanel).
// It returns the selected save path, or empty string if the user cancelled.
func SaveFilePicker(initialPath string, suggestedFilename string) (string, error) {
	if runtime.GOOS != goosDarwin {
		return "", fmt.Errorf("SaveFilePicker is only supported on macOS")
	}

	binFile, err := compiledHelper("save_file_picker", saveFilePickerSwift)
	if err != nil {
		return "", err
	}

	// Run
	args := []string{}
	if initialPath != "" {
		args = append(args, initialPath)
	}
	if suggestedFilename != "" {
		args = append(args, suggestedFilename)
	}
	out, err := exec.Command(binFile, args...).Output() //nolint:noctx
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Exit code non-zero usually means user cancelled
			return "", nil
		}
		return "", fmt.Errorf("failed to run picker helper: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return "", nil
	}

	return trimmed, nil
}
