package clib

import (
	"bytes"
	"image"
	"image/png"

	_ "image/gif"  // register GIF decoder for image.Decode
	_ "image/jpeg" // register JPEG decoder for image.Decode

	_ "golang.org/x/image/bmp"  // register BMP decoder for image.Decode
	_ "golang.org/x/image/webp" // register WebP decoder for image.Decode
)

// MaxDecodePixels caps the pixel count DecodeToPNG will decode. Image headers
// are checked first, so a small file that declares huge dimensions cannot
// make the decoder allocate gigabytes.
const MaxDecodePixels = 40_000_000

// DecodeToPNG takes raw image bytes (PNG, JPEG, GIF, BMP or WebP) and returns
// PNG-encoded bytes along with image dimensions.
// This is the pure Go fallback used when cgo is not available.
func DecodeToPNG(data []byte) (ImageConvertResult, bool) {
	if len(data) == 0 {
		return ImageConvertResult{}, false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > MaxDecodePixels {
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

// ImageDimensions returns the width and height of an image without decoding
// its pixel data.
func ImageDimensions(data []byte) (width, height int, ok bool) {
	if len(data) == 0 {
		return 0, 0, false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// ImageFormat reports the format ("png", "jpeg", "gif", "bmp" or "webp") and
// dimensions of an image from its header, without decoding pixel data.
func ImageFormat(data []byte) (format string, width, height int, ok bool) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, false
	}
	return format, cfg.Width, cfg.Height, true
}
