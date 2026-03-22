//go:build cgo

package clib

/*
#cgo LDFLAGS: -lm
#include "imgconv.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

// DecodeToPNG takes raw image bytes (JPEG, PNG, BMP, GIF, etc.) and returns
// PNG-encoded bytes along with image dimensions. Uses stb_image for decoding
// and stb_image_write for PNG encoding in C, which is faster than Go's
// image stdlib for large images.
func DecodeToPNG(data []byte) (ImageConvertResult, bool) {
	if len(data) == 0 {
		return ImageConvertResult{}, false
	}

	cData := C.CBytes(data)
	defer C.free(cData)

	result := C.decode_to_png((*C.uchar)(cData), C.size_t(len(data)))
	if result.ok == 0 {
		return ImageConvertResult{}, false
	}
	defer C.free_image_result(&result)

	pngData := C.GoBytes(unsafe.Pointer(result.png_data), C.int(result.png_len))

	return ImageConvertResult{
		PNGData: pngData,
		Width:   int(result.width),
		Height:  int(result.height),
	}, true
}

// ImageDimensions returns the width and height of an image without fully
// decoding pixel data. This is faster than DecodeToPNG when you only need
// the dimensions (e.g. to calculate terminal row count).
func ImageDimensions(data []byte) (width, height int, ok bool) {
	if len(data) == 0 {
		return 0, 0, false
	}

	cData := C.CBytes(data)
	defer C.free(cData)

	var cw, ch C.int
	ret := C.image_dimensions((*C.uchar)(cData), C.size_t(len(data)), &cw, &ch)
	if ret == 0 {
		return 0, 0, false
	}
	return int(cw), int(ch), true
}
