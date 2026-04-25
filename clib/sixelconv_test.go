package clib

import (
	"encoding/base64"
	"testing"
)

// 1x1 red PNG
const testPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="

func TestEncodePNGToSixel(t *testing.T) {
	pngData, err := base64.StdEncoding.DecodeString(testPNG)
	if err != nil {
		t.Fatal(err)
	}

	sixel, rows, err := EncodePNGToSixel(pngData, 18)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	if rows < 1 {
		t.Errorf("Expected rows >= 1, got %d", rows)
	}

	if len(sixel) == 0 {
		t.Error("Expected non-empty Sixel output")
	}
}
