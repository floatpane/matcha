package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func TestIconPath() {
	home, _ := os.UserHomeDir()
	fmt.Println(filepath.Join(home, ".local", "share", "icons", "hicolor", "512x512", "apps"))
}
