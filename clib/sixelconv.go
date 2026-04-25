package clib

import (
	"bytes"
	"image/png"

	"github.com/mattn/go-sixel"
)

// EncodePNGToSixel converts PNG bytes to Sixel format
// Returns Sixel sequence and row count needed for terminal spacing
func EncodePNGToSixel(pngData []byte, cellHeightPx int) (string, int, error) {
	// Decode PNG
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return "", 0, err
	}

	// Encode to Sixel
	var buf bytes.Buffer
	enc := sixel.NewEncoder(&buf)
	if err := enc.Encode(img); err != nil {
		return "", 0, err
	}

	// Calculate rows: image height / cell height
	bounds := img.Bounds()
	rows := (bounds.Dy() + cellHeightPx - 1) / cellHeightPx

	return buf.String(), rows, nil
}
