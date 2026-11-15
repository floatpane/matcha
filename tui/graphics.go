package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/view"
)

// ScreenImage is an out-of-band (Kitty or Sixel) image placement expressed in
// absolute, 0-based screen cells. Placeholder images (inside tmux) are drawn
// by the text; they are listed only so that the painter uploads them.
type ScreenImage struct {
	ID        uint32 // image identity (Kitty image id)
	Placement uint32 // placement id, unique per image occurrence
	Protocol  view.ImageProtocol
	Row, Col  int // top-left screen cell
	Cols      int // displayed size in cells
	Rows      int
	// SrcY/SrcW/SrcH select the visible band of a Kitty image in pixels so
	// images that are partially scrolled out of their pane are clipped.
	SrcY, SrcW, SrcH int
	// Data is the Kitty transmit sequence (wrapped for tmux for placeholder
	// images) or the Sixel sequence.
	Data string
}

type screenImageKey struct {
	id, placement uint32
}

func (s ScreenImage) key() screenImageKey {
	return screenImageKey{id: s.ID, placement: s.Placement}
}

// sameSpot reports whether two placements draw the same pixels at the same
// screen position. Data is compared too: placeholder image ids are reused
// for other images (comparing the same string is cheap).
func (s ScreenImage) sameSpot(o ScreenImage) bool {
	return s.ID == o.ID && s.Placement == o.Placement && s.Protocol == o.Protocol &&
		s.Row == o.Row && s.Col == o.Col && s.Cols == o.Cols && s.Rows == o.Rows &&
		s.SrcY == o.SrcY && s.SrcW == o.SrcW && s.SrcH == o.SrcH && s.Data == o.Data
}

func (s ScreenImage) intersects(row, col, rows, cols int) bool {
	return s.Row < row+rows && row < s.Row+s.Rows && s.Col < col+cols && col < s.Col+s.Cols
}

// ImageProvider is implemented by views that display out-of-band images.
// VisibleImages returns the placements that are fully laid out on screen
// right now; the GraphicsPainter keeps the terminal in sync with it.
type ImageProvider interface {
	VisibleImages() []ScreenImage
}

// GraphicsPaintMsg asks the GraphicsPainter to draw pending placements.
type GraphicsPaintMsg struct {
	gen uint64
}

// graphicsPaintDelay postpones painting until after the renderer flushed the
// frame that matches the new layout (it flushes at 60 fps). Painting earlier
// would let the frame's scroll optimizations shift or overwrite the images.
const graphicsPaintDelay = 35 * time.Millisecond

// maxTransmittedImages bounds how many Kitty images stay uploaded before
// unused ones are freed.
const maxTransmittedImages = 48

// GraphicsPainter keeps out-of-band terminal images in sync with the UI.
//
// Kitty and Sixel images cannot go through Bubble Tea's cell renderer, so
// they are written as raw escape sequences. Doing that from View() races the
// renderer's own output and draws images before the frame they belong to.
// The painter instead routes every sequence through tea.Raw (serialized with
// the renderer), removes stale placements immediately, and paints new ones a
// frame later, once the text layout they align with is on screen.
type GraphicsPainter struct {
	desired     []ScreenImage
	painted     map[screenImageKey]ScreenImage
	transmitted map[uint32]upload
	gen         uint64
}

// upload is an image the terminal holds under an id.
type upload struct {
	data        string
	placeholder bool // sent through tmux passthrough
}

// NewGraphicsPainter returns an empty painter.
func NewGraphicsPainter() *GraphicsPainter {
	return &GraphicsPainter{
		painted:     make(map[screenImageKey]ScreenImage),
		transmitted: make(map[uint32]upload),
	}
}

func samePlacements(a, b []ScreenImage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].sameSpot(b[i]) {
			return false
		}
	}
	return true
}

// Sync reconciles the terminal with the desired placements. Call it after
// every update; it returns nil when nothing changed.
func (g *GraphicsPainter) Sync(desired []ScreenImage) tea.Cmd {
	raw, clearScreen, paint, changed := g.plan(desired)
	if !changed {
		return nil
	}
	var cmds []tea.Cmd
	if raw != "" {
		cmds = append(cmds, tea.Raw(raw))
	}
	if clearScreen {
		cmds = append(cmds, tea.ClearScreen)
	}
	if paint {
		gen := g.gen
		cmds = append(cmds, tea.Tick(graphicsPaintDelay, func(time.Time) tea.Msg {
			return GraphicsPaintMsg{gen: gen}
		}))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Sequence(cmds...)
}

// plan updates the painter for a new set of desired placements. It returns
// the escape sequences to write right away (deletions and uploads), whether
// the screen must be redrawn to erase Sixel pixels, and whether a delayed
// paint is needed.
func (g *GraphicsPainter) plan(desired []ScreenImage) (raw string, clearScreen, paint, changed bool) {
	if samePlacements(desired, g.desired) {
		return "", false, false, false
	}
	g.desired = desired
	g.gen++

	want := make(map[screenImageKey]ScreenImage, len(desired))
	wantIDs := make(map[uint32]bool, len(desired))
	for _, d := range desired {
		want[d.key()] = d
		wantIDs[d.ID] = true
	}

	var b strings.Builder
	for k, p := range g.painted {
		if d, ok := want[k]; ok && d.sameSpot(p) {
			continue
		}
		switch p.Protocol {
		case view.ImageProtocolKitty:
			b.WriteString(view.KittyDeletePlacementSequence(p.ID, p.Placement))
		case view.ImageProtocolSixel:
			clearScreen = true
		case view.ImageProtocolNone, view.ImageProtocolHalfBlock, view.ImageProtocolKittyPlaceholder:
		}
		delete(g.painted, k)
	}
	if clearScreen {
		// Sixel pixels live in the text cells; the only reliable way to
		// remove them is a full redraw, which also wipes the other sixels.
		for k, p := range g.painted {
			if p.Protocol == view.ImageProtocolSixel {
				delete(g.painted, k)
			}
		}
	}

	if len(g.transmitted) > maxTransmittedImages {
		for id, u := range g.transmitted {
			if !wantIDs[id] {
				b.WriteString(u.free(id))
				delete(g.transmitted, id)
			}
		}
	}
	for _, d := range desired {
		if d.Protocol.Uploaded() && g.transmitted[d.ID].data != d.Data {
			// Uploading does not draw anything, so it can go out right away.
			// An id given to another image is uploaded again.
			b.WriteString(d.Data)
			g.transmitted[d.ID] = upload{data: d.Data, placeholder: d.Protocol == view.ImageProtocolKittyPlaceholder}
		}
	}
	return b.String(), clearScreen, g.needsPaint(), true
}

// free returns the sequence that frees the upload stored under id.
func (u upload) free(id uint32) string {
	if u.placeholder {
		return view.KittyPlaceholderFreeSequence(id)
	}
	return view.KittyFreeImageSequence(id)
}

func (g *GraphicsPainter) needsPaint() bool {
	for _, d := range g.desired {
		if !d.Protocol.OutOfBand() {
			continue // placeholder images are drawn by the text
		}
		if p, ok := g.painted[d.key()]; !ok || !p.sameSpot(d) {
			return true
		}
	}
	return false
}

// Paint draws the placements requested by the latest Sync. Stale paint
// requests (superseded by a newer layout) are ignored.
func (g *GraphicsPainter) Paint(msg GraphicsPaintMsg) tea.Cmd {
	if msg.gen != g.gen {
		return nil
	}
	var raw strings.Builder
	for _, d := range g.desired {
		k := d.key()
		if p, ok := g.painted[k]; ok && p.sameSpot(d) {
			continue
		}
		switch d.Protocol {
		case view.ImageProtocolKitty:
			raw.WriteString(view.KittyPlaceSequence(d.ID, d.Placement, d.Row, d.Col, d.Cols, d.Rows, d.SrcY, d.SrcW, d.SrcH))
		case view.ImageProtocolSixel:
			raw.WriteString(view.SixelPaintSequence(d.Data, d.Row, d.Col))
		case view.ImageProtocolNone, view.ImageProtocolHalfBlock, view.ImageProtocolKittyPlaceholder:
			continue
		}
		g.painted[k] = d
	}
	if raw.Len() == 0 {
		return nil
	}
	return tea.Raw(raw.String())
}

// Reset forgets what is on screen, removing visible Kitty placements. Use it
// when the renderer redraws everything (resize). When the terminal may have
// dropped uploaded images too (after running an external program), pass
// forgetUploads so they are transmitted again.
func (g *GraphicsPainter) Reset(forgetUploads bool) tea.Cmd {
	hadKitty := false
	for _, p := range g.painted {
		if p.Protocol == view.ImageProtocolKitty {
			hadKitty = true
			break
		}
	}
	g.painted = make(map[screenImageKey]ScreenImage)
	g.desired = nil
	g.gen++
	if forgetUploads {
		g.transmitted = make(map[uint32]upload)
	}
	if !hadKitty {
		return nil
	}
	return tea.Raw(view.KittyDeleteAllSequence)
}

// Shutdown removes matcha's images before the program exits: the visible
// placements, and the images uploaded for placeholders, which tmux panes
// would otherwise keep in the terminal (they are freed by id, as other
// panes may show images of their own).
func (g *GraphicsPainter) Shutdown() tea.Cmd {
	var b strings.Builder
	for id, u := range g.transmitted {
		if u.placeholder {
			b.WriteString(u.free(id))
			delete(g.transmitted, id)
		}
	}
	reset := g.Reset(false)
	if b.Len() == 0 {
		return reset
	}
	return tea.Sequence(reset, tea.Raw(b.String()))
}

// HasPainted reports whether any out-of-band image is currently on screen.
func (g *GraphicsPainter) HasPainted() bool {
	return len(g.painted) > 0
}

// FilterImagesOutside drops the placements that overlap the given screen
// rectangle (for example a notification drawn on top of the view).
func FilterImagesOutside(images []ScreenImage, row, col, rows, cols int) []ScreenImage {
	if len(images) == 0 || rows <= 0 || cols <= 0 {
		return images
	}
	out := images[:0:0]
	for _, img := range images {
		if !img.intersects(row, col, rows, cols) {
			out = append(out, img)
		}
	}
	return out
}
