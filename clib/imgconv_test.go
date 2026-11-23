package clib

import (
	"encoding/base64"
	"testing"
)

func TestDecodeToPNGWebP(t *testing.T) {
	// 1x1 lossless WebP.
	data, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	res, ok := DecodeToPNG(data)
	if !ok || res.Width != 1 || res.Height != 1 || len(res.PNGData) == 0 {
		t.Fatalf("DecodeToPNG(webp) = %dx%d ok=%v, want a 1x1 PNG", res.Width, res.Height, ok)
	}
	if format, w, h, ok := ImageFormat(data); !ok || format != "webp" || w != 1 || h != 1 {
		t.Fatalf("ImageFormat(webp) = %q %dx%d ok=%v", format, w, h, ok)
	}
}
