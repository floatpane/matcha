package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/floatpane/matcha/config"
)

func TestRunConfigPath(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	want, err := config.GetConfigFile()
	if err != nil {
		t.Fatalf("GetConfigFile failed: %v", err)
	}

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	os.Stdout = w

	runErr := RunConfig([]string{"path"})

	w.Close()
	os.Stdout = orig
	out, _ := io.ReadAll(r)

	if runErr != nil {
		t.Fatalf("RunConfig(path) returned error: %v", runErr)
	}

	got := strings.TrimSpace(string(out))
	if got != want {
		t.Errorf("RunConfig(path) printed %q, want %q", got, want)
	}
	if filepath.Base(got) != "config.json" {
		t.Errorf("printed path %q does not end in config.json", got)
	}
}
