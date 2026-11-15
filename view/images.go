package view

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"
	"sync/atomic"

	"github.com/floatpane/termimage/detect"
	"github.com/floatpane/termimage/render"
	"github.com/floatpane/termimage/sandbox"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/image/draw"
)

// ImageProtocol identifies how inline images are drawn on the terminal.
type ImageProtocol int

const (
	// ImageProtocolNone means the terminal cannot show images; the body shows
	// a link or an "[Image: …]" label instead.
	ImageProtocolNone ImageProtocol = iota
	// ImageProtocolHalfBlock draws images with colored half-block characters.
	// The characters are part of the body text itself, so they scroll, clip
	// and redraw exactly like any other text.
	ImageProtocolHalfBlock
	// ImageProtocolSixel draws DEC Sixel graphics out-of-band.
	ImageProtocolSixel
	// ImageProtocolKitty uses the Kitty graphics protocol out-of-band.
	ImageProtocolKitty
	// ImageProtocolKittyPlaceholder shows Kitty images inside tmux: each
	// image is uploaded through tmux passthrough and drawn with Unicode
	// placeholder cells that are part of the body text, like half-blocks.
	ImageProtocolKittyPlaceholder
)

// String returns a short human-readable protocol name.
func (p ImageProtocol) String() string {
	switch p {
	case ImageProtocolHalfBlock:
		return "halfblock"
	case ImageProtocolSixel:
		return "sixel"
	case ImageProtocolKitty:
		return "kitty"
	case ImageProtocolKittyPlaceholder:
		return "kitty-placeholder"
	case ImageProtocolNone:
		return "none"
	}
	return "none"
}

// OutOfBand reports whether images of this protocol are painted outside the
// cell renderer (and must therefore be positioned and cleared explicitly).
// Placeholder images are uploaded out-of-band but drawn by the text.
func (p ImageProtocol) OutOfBand() bool {
	return p == ImageProtocolKitty || p == ImageProtocolSixel
}

// Uploaded reports whether images of this protocol are transmitted to the
// terminal and referred to by id afterwards.
func (p ImageProtocol) Uploaded() bool {
	return p == ImageProtocolKitty || p == ImageProtocolKittyPlaceholder
}

// ActiveImageProtocol returns the protocol used to draw inline images in the
// current terminal, or ImageProtocolNone when images are not supported.
func ActiveImageProtocol() ImageProtocol {
	if !imageProtocolSupported() {
		return ImageProtocolNone
	}
	switch {
	case zellijSupported():
		// Zellij forwards Sixel but not the Kitty protocol, even when it runs
		// inside a Kitty-capable terminal.
		return ImageProtocolSixel
	case inTmux():
		return tmuxImageProtocol()
	case kittySupported(), ghosttySupported(), weztermSupported():
		return ImageProtocolKitty
	case sixelSupported():
		return ImageProtocolSixel
	}
	switch detect.Best() {
	case detect.Kitty:
		return ImageProtocolKitty
	case detect.Sixel:
		return ImageProtocolSixel
	case detect.HalfBlock:
		return ImageProtocolHalfBlock
	}
	return ImageProtocolHalfBlock
}

// ImageBounds limits the size of an inline image, in terminal cells. Zero
// values fall back to limits derived from the terminal size.
type ImageBounds struct {
	MaxCols int
	MaxRows int
}

func (b ImageBounds) resolve() ImageBounds {
	if b.MaxCols <= 0 {
		b.MaxCols = maxImageCellWidth()
	}
	if b.MaxRows <= 0 {
		b.MaxRows = maxImageCellHeight()
	}
	return b
}

// preparedImage is an image decoded, scaled and encoded for one protocol.
type preparedImage struct {
	protocol ImageProtocol
	cols     int // terminal columns the image occupies
	rows     int // terminal rows the image occupies
	pixelW   int // pixel size of the encoded image
	pixelH   int
	id       uint32 // unique per encoded payload; sent as the Kitty image id
	// data is the Kitty transmit sequence, the Sixel sequence, or the
	// half-block text (rows joined by "\n"), depending on protocol.
	data string
}

const (
	// maxDecodedSide caps the pixel size of decoded images kept in memory.
	// It comfortably exceeds what any terminal can show at native resolution.
	maxDecodedSide = 2048
	// kittyChunkSize is the maximum base64 payload per Kitty escape sequence.
	kittyChunkSize = 4096
)

var (
	decodedImages, _  = lru.New[[sha256.Size]byte, *image.NRGBA](16)
	encodedImages, _  = lru.New[encodedKey, *preparedImage](64)
	nextImageID       atomic.Uint32
	errImageTooSmall  = errors.New("image has no pixels")
	errNoImagePayload = errors.New("empty image payload")
)

type encodedKey struct {
	hash       [sha256.Size]byte
	protocol   ImageProtocol
	cols, rows int
	cellW      int
	cellH      int
}

// cellPixelOverride lets tests pin the cell size.
var cellPixelOverride struct{ w, h int }

// cellPixelSize returns the size of a terminal cell in pixels.
func cellPixelSize() (int, int) {
	if cellPixelOverride.w > 0 && cellPixelOverride.h > 0 {
		return cellPixelOverride.w, cellPixelOverride.h
	}
	if w, h, ok := ttyCellPixels(); ok {
		return w, h
	}
	return 8, 16
}

// decodeImagePayload decodes base64 image bytes in the sandboxed worker and
// caches the (size-capped) result so re-layouts never decode twice.
func decodeImagePayload(payload string) ([sha256.Size]byte, *image.NRGBA, error) {
	var key [sha256.Size]byte
	data, err := decodeBase64Payload(payload)
	if err != nil {
		return key, nil, err
	}
	key = sha256.Sum256(data)
	if img, ok := decodedImages.Get(key); ok {
		return key, img, nil
	}
	img, err := sandbox.DecodeBytes(data)
	if err != nil {
		return key, nil, err
	}
	b := img.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 {
		return key, nil, errImageTooSmall
	}
	if b.Dx() > maxDecodedSide || b.Dy() > maxDecodedSide {
		w, h := fitWithin(b.Dx(), b.Dy(), maxDecodedSide, maxDecodedSide)
		img = scaleImage(img, w, h)
	}
	decodedImages.Add(key, img)
	return key, img, nil
}

func decodeBase64Payload(payload string) ([]byte, error) {
	payload = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, payload)
	if payload == "" {
		return nil, errNoImagePayload
	}
	if data, err := base64.StdEncoding.DecodeString(payload); err == nil {
		return data, nil
	}
	if data, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(payload, "=")); err == nil {
		return data, nil
	}
	return base64.URLEncoding.DecodeString(payload)
}

// prepareImage decodes payload and encodes it for protocol so that it fits
// in bounds. Results are cached per (image, protocol, size).
func prepareImage(payload string, protocol ImageProtocol, bounds ImageBounds) (*preparedImage, error) {
	if protocol == ImageProtocolNone {
		return nil, errors.New("no image protocol")
	}
	bounds = bounds.resolve()
	hash, src, err := decodeImagePayload(payload)
	if err != nil {
		return nil, err
	}
	cellW, cellH := cellPixelSize()
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if protocol == ImageProtocolKittyPlaceholder {
		bounds.MaxCols = min(bounds.MaxCols, maxPlaceholderCells)
		bounds.MaxRows = min(bounds.MaxRows, maxPlaceholderCells)
	}
	cols, rows := imageCellBox(sw, sh, cellW, cellH, bounds.MaxCols, bounds.MaxRows)

	key := encodedKey{hash: hash, protocol: protocol, cols: cols, rows: rows, cellW: cellW, cellH: cellH}
	if p, ok := encodedImages.Get(key); ok {
		return p, nil
	}

	p := &preparedImage{protocol: protocol, cols: cols, rows: rows}
	switch protocol {
	case ImageProtocolKitty:
		// Never send more pixels than the cell box can show; the terminal
		// scales the payload to exactly cols×rows cells on placement.
		tw, th := fitWithin(sw, sh, cols*cellW, rows*cellH)
		scaled := scaleImage(src, tw, th)
		var buf bytes.Buffer
		if err := png.Encode(&buf, scaled); err != nil {
			return nil, fmt.Errorf("encode png: %w", err)
		}
		p.id = nextImageID.Add(1)
		p.pixelW, p.pixelH = tw, th
		p.data = kittyTransmitSequence(p.id, buf.Bytes())
	case ImageProtocolKittyPlaceholder:
		tw, th := fitWithin(sw, sh, cols*cellW, rows*cellH)
		scaled := scaleImage(src, tw, th)
		var buf bytes.Buffer
		if err := png.Encode(&buf, scaled); err != nil {
			return nil, fmt.Errorf("encode png: %w", err)
		}
		p.id = placeholderIDs.id(key)
		p.pixelW, p.pixelH = tw, th
		p.data = kittyPlaceholderUpload(p.id, buf.Bytes(), cols, rows)
	case ImageProtocolSixel:
		tw, th := fitWithin(sw, sh, cols*cellW, rows*cellH)
		scaled := scaleImage(src, tw, th)
		var buf bytes.Buffer
		if err := render.Sixel(&buf, scaled); err != nil {
			return nil, fmt.Errorf("encode sixel: %w", err)
		}
		p.id = nextImageID.Add(1)
		p.pixelW, p.pixelH = tw, th
		// Sixel output occupies ceil(th/cellH) rows.
		p.rows = min(rows, max(1, (th+cellH-1)/cellH))
		p.data = buf.String()
	case ImageProtocolHalfBlock:
		// One cell holds one pixel horizontally and two vertically.
		scaled := scaleImage(src, cols, rows*2)
		var buf bytes.Buffer
		if err := render.HalfBlock(&buf, scaled); err != nil {
			return nil, fmt.Errorf("encode halfblock: %w", err)
		}
		p.pixelW, p.pixelH = cols, rows*2
		p.data = strings.TrimRight(buf.String(), "\n")
	case ImageProtocolNone:
		return nil, errors.New("no image protocol")
	}
	encodedImages.Add(key, p)
	return p, nil
}

// imageCellBox returns the number of terminal cells an image of pw×ph pixels
// occupies when shown at native resolution, shrunk (never enlarged) so that it
// fits within maxCols×maxRows while keeping its aspect ratio.
func imageCellBox(pw, ph, cellW, cellH, maxCols, maxRows int) (cols, rows int) {
	if cellW < 1 {
		cellW = 8
	}
	if cellH < 1 {
		cellH = 16
	}
	maxCols = max(1, maxCols)
	maxRows = max(1, maxRows)
	if pw < 1 || ph < 1 {
		return 1, 1
	}
	tw, th := fitWithin(pw, ph, maxCols*cellW, maxRows*cellH)
	cols = min(maxCols, max(1, (tw+cellW-1)/cellW))
	rows = min(maxRows, max(1, (th+cellH-1)/cellH))
	return cols, rows
}

// fitWithin scales w×h down (never up) to fit inside maxW×maxH, preserving
// the aspect ratio. Both results are at least 1.
func fitWithin(w, h, maxW, maxH int) (int, int) {
	if w < 1 || h < 1 {
		return 1, 1
	}
	maxW = max(1, maxW)
	maxH = max(1, maxH)
	scale := math.Min(1, math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h)))
	return max(1, int(math.Round(float64(w)*scale))), max(1, int(math.Round(float64(h)*scale)))
}

func scaleImage(src *image.NRGBA, w, h int) *image.NRGBA {
	b := src.Bounds()
	if b.Dx() == w && b.Dy() == h {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

// kittyTransmitSequence uploads a PNG to the terminal under id without
// displaying it. q=2 suppresses replies, which would otherwise arrive on
// stdin and be read as key presses.
func kittyTransmitSequence(id uint32, pngData []byte) string {
	enc := base64.StdEncoding.EncodeToString(pngData)
	var b strings.Builder
	b.Grow(len(enc) + len(enc)/kittyChunkSize*24 + 64)
	for i := 0; i < len(enc); i += kittyChunkSize {
		end := min(i+kittyChunkSize, len(enc))
		more := 0
		if end < len(enc) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Ga=t,f=100,t=d,i=%d,q=2,m=%d;%s\x1b\\", id, more, enc[i:end])
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d,q=2;%s\x1b\\", more, enc[i:end])
		}
	}
	return b.String()
}

// KittyPlaceSequence displays a slice of a transmitted image at the given
// 0-based screen cell. The cursor is saved and restored and C=1 keeps the
// terminal from moving it, so placing an image can never scroll the screen.
// srcY/srcH select the visible band of the image in pixels.
func KittyPlaceSequence(id, placement uint32, row, col, cols, rows, srcY, srcW, srcH int) string {
	return fmt.Sprintf("\x1b7\x1b[%d;%dH\x1b_Ga=p,i=%d,p=%d,q=2,C=1,c=%d,r=%d,x=0,y=%d,w=%d,h=%d\x1b\\\x1b8",
		row+1, col+1, id, placement, cols, rows, srcY, srcW, srcH)
}

// KittyDeletePlacementSequence removes one placement but keeps the image
// data in the terminal so it can be placed again without re-uploading.
func KittyDeletePlacementSequence(id, placement uint32) string {
	return fmt.Sprintf("\x1b_Ga=d,d=i,i=%d,p=%d,q=2\x1b\\", id, placement)
}

// KittyFreeImageSequence deletes all placements of an image and frees its data.
func KittyFreeImageSequence(id uint32) string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
}

// KittyDeleteAllSequence removes every visible Kitty image placement.
const KittyDeleteAllSequence = "\x1b_Ga=d,d=a,q=2\x1b\\"

// SixelPaintSequence draws a Sixel image at the given 0-based screen cell
// and restores the cursor afterwards.
func SixelPaintSequence(data string, row, col int) string {
	return fmt.Sprintf("\x1b7\x1b[%d;%dH%s\x1b8", row+1, col+1, data)
}
