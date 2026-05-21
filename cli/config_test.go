package cli

import (
	"errors"
	"os"
	"testing"
)

func TestRunConfigMissingConfigWrapsNotExist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := RunConfig(nil)
	if err == nil {
		t.Fatal("RunConfig() error = nil, want missing config error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("RunConfig() error = %v, want errors.Is(os.ErrNotExist)", err)
	}
}
