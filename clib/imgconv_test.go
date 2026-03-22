package clib

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// createTestPNG generates a small PNG image in memory for testing.
func createTestPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestDecodeToPNGValid(t *testing.T) {
	input := createTestPNG(16, 32)

	result, ok := DecodeToPNG(input)
	if !ok {
		t.Fatal("DecodeToPNG failed for valid PNG")
	}
	if result.Width != 16 {
		t.Errorf("expected width=16, got %d", result.Width)
	}
	if result.Height != 32 {
		t.Errorf("expected height=32, got %d", result.Height)
	}
	if len(result.PNGData) == 0 {
		t.Error("expected non-empty PNG data")
	}

	// Verify output is valid PNG by decoding with Go
	img, err := png.Decode(bytes.NewReader(result.PNGData))
	if err != nil {
		t.Fatalf("output is not valid PNG: %v", err)
	}
	if img.Bounds().Dx() != 16 || img.Bounds().Dy() != 32 {
		t.Errorf("decoded dimensions mismatch: got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestDecodeToPNGEmpty(t *testing.T) {
	_, ok := DecodeToPNG(nil)
	if ok {
		t.Error("expected failure for nil input")
	}

	_, ok = DecodeToPNG([]byte{})
	if ok {
		t.Error("expected failure for empty input")
	}
}

func TestDecodeToPNGInvalid(t *testing.T) {
	_, ok := DecodeToPNG([]byte("not an image"))
	if ok {
		t.Error("expected failure for invalid image data")
	}
}

func TestImageDimensionsValid(t *testing.T) {
	input := createTestPNG(64, 48)

	w, h, ok := ImageDimensions(input)
	if !ok {
		t.Fatal("ImageDimensions failed for valid PNG")
	}
	if w != 64 {
		t.Errorf("expected width=64, got %d", w)
	}
	if h != 48 {
		t.Errorf("expected height=48, got %d", h)
	}
}

func TestImageDimensionsEmpty(t *testing.T) {
	_, _, ok := ImageDimensions(nil)
	if ok {
		t.Error("expected failure for nil input")
	}
}

func TestImageDimensionsInvalid(t *testing.T) {
	_, _, ok := ImageDimensions([]byte("garbage"))
	if ok {
		t.Error("expected failure for invalid data")
	}
}
