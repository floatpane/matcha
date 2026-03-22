# clib

C implementations of performance-critical operations, integrated via [cgo](https://pkg.go.dev/cmd/cgo).

## Why C?

Some operations in Matcha benefit from dropping into C for speed and lower memory usage. The biggest wins are in image dimension queries (30,000x faster than Go's stdlib) and base64 wrapping for large attachments (3-4x faster).

## Components

### base64wrap

MIME-compliant base64 line-wrapping (RFC 2045). Wraps base64-encoded strings at 76 characters with `\r\n` separators. Used by `sender/` for every attachment, inline image, and S/MIME signature.

Files: `base64wrap.c`, `base64wrap.h`, `base64wrap.go`

### imgconv

Image decoding and PNG re-encoding using [stb_image](https://github.com/nothings/stb) (vendored single-header C libraries). No external system dependencies required.

- `DecodeToPNG()` — decodes any image format (JPEG, PNG, BMP, GIF) and re-encodes to PNG. Used by `view/html.go` for inline email images.
- `ImageDimensions()` — reads image width and height from the header only, without decoding pixel data. Used to calculate terminal row spacing for inline images.

Files: `imgconv.c`, `imgconv.h`, `imgconv.go`, `stb_image.h`, `stb_image_write.h`


## Adding new C code

1. Create `yourmodule.c` and `yourmodule.h` in this directory
2. Create `yourmodule.go` with cgo bindings (see `base64wrap.go` for a minimal example)
3. Add tests in `yourmodule_test.go`
4. If your C code uses `libm` or other system libraries, add `#cgo LDFLAGS: -lm` in the Go file
