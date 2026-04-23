package macos

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed file_picker.swift
var filePickerSwift string

// OpenFilePicker launches the native macOS file picker.
// It returns a list of selected absolute file paths.
func OpenFilePicker(initialPath string) ([]string, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("OpenFilePicker is only supported on macOS")
	}

	tmpDir, err := os.MkdirTemp("", "matcha-filepicker")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	swiftFile := filepath.Join(tmpDir, "file_picker.swift")
	if err := os.WriteFile(swiftFile, []byte(filePickerSwift), 0644); err != nil {
		return nil, err
	}

	binFile := filepath.Join(tmpDir, "file_picker")

	// Compile
	cmd := exec.Command("swiftc", swiftFile, "-o", binFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to compile file picker helper: %w\n%s", err, string(out))
	}

	// Run
	args := []string{}
	if initialPath != "" {
		args = append(args, initialPath)
	}
	out, err := exec.Command(binFile, args...).Output()
	if err != nil {
		// Exit code 1 usually means user cancelled
		return nil, nil
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}

	paths := strings.Split(trimmed, "\n")
	return paths, nil
}
