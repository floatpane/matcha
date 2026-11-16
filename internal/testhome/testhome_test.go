package testhome

import (
	"os"
	"testing"
)

func TestIsolateCoversEveryHomeVariable(t *testing.T) {
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME"} {
		t.Setenv(key, "/real/"+key)
	}
	cleanup, err := Isolate()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	home, err := os.UserHomeDir()
	if err != nil || home == "/real/HOME" || home == "/real/USERPROFILE" {
		t.Fatalf("os.UserHomeDir() = %q, %v; want the temporary home", home, err)
	}
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
		if v := os.Getenv(key); v == "/real/"+key {
			t.Errorf("%s still points at the real profile", key)
		}
	}
	if os.Getenv("XDG_CONFIG_HOME") != "" {
		t.Error("XDG_CONFIG_HOME was not cleared")
	}
}
