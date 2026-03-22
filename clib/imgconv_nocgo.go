//go:build !cgo

package clib

import (
	"bytes"
	"image"
	"image/png"

	_ "image/gif"
	_ "image/jpeg"
)

// DecodeToPNG takes raw image bytes (JPEG, PNG, BMP, GIF, etc.) and returns
// PNG-encoded bytes along with image dimensions.
// This is the pure Go fallback used when cgo is not available.
func DecodeToPNG(data []byte) (ImageConvertResult, bool) {
	if len(data) == 0 {
		return ImageConvertResult{}, false
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ImageConvertResult{}, false
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ImageConvertResult{}, false
	}

	bounds := img.Bounds()
	return ImageConvertResult{
		PNGData: buf.Bytes(),
		Width:   bounds.Dx(),
		Height:  bounds.Dy(),
	}, true
}

// ImageDimensions returns the width and height of an image without fully
// decoding pixel data.
// This is the pure Go fallback — it must fully decode the image since Go's
// stdlib does not support header-only reads.
func ImageDimensions(data []byte) (width, height int, ok bool) {
	if len(data) == 0 {
		return 0, 0, false
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0, false
	}

	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy(), true
}
