package view

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// Inside tmux only text reaches the real terminal reliably. tmux drops
// graphics escape sequences unless they are wrapped for passthrough, and it
// draws panes itself, so an image placed at "the cursor" lands wherever
// tmux's own cursor happens to be. Terminals that implement the Kitty
// protocol's Unicode placeholders (kitty, Ghostty) solve both: the image is
// uploaded once through passthrough as a virtual placement, and the body
// text holds placeholder cells that tmux moves, scrolls, clips and redraws
// like any other text, while the terminal draws the image where they are.

// inTmux reports whether matcha runs inside tmux.
func inTmux() bool {
	return os.Getenv("TMUX") != ""
}

// tmuxImageProtocol picks the image protocol inside tmux: Unicode
// placeholders when the attached terminal supports them and tmux lets the
// upload through, half-block text otherwise (it works everywhere tmux does).
func tmuxImageProtocol() ImageProtocol {
	if os.Getenv("NO_COLOR") != "" {
		// Both placeholders (their colour names the image) and half-blocks
		// need colours, which the renderer then strips.
		return ImageProtocolNone
	}
	pane := currentTmux()
	if !pane.passthrough {
		return ImageProtocolHalfBlock
	}
	// The environment describes the terminal that started the tmux server,
	// which need not be the one attached now; it decides only when tmux
	// knows of no attached terminal.
	supported := tmuxClientHasPlaceholders()
	if !pane.attached {
		supported = kittySupported() || ghosttySupported()
	}
	if supported {
		return ImageProtocolKittyPlaceholder
	}
	return ImageProtocolHalfBlock
}

// tmuxClientHasPlaceholders reports whether the terminal attached to tmux
// supports Unicode placeholders, by its TERM or by the name it gave tmux
// (XTVERSION), which still says ghostty when TERM is xterm-256color.
func tmuxClientHasPlaceholders() bool {
	term := currentTmux().clientTerm
	return strings.Contains(term, "kitty") || strings.Contains(term, termGhostty)
}

// tmuxPane is what tmux reports about the pane matcha runs in.
type tmuxPane struct {
	attached    bool   // a terminal is attached to the pane's session
	clientTerm  string // its TERM and name, such as "xterm-256color ghostty 1.2.3"
	passthrough bool   // the pane may pass escape sequences through
	// passthroughHidden: tmux forwards passthrough even while the pane is
	// not on screen (allow-passthrough all).
	passthroughHidden bool
}

var (
	tmuxMu    sync.Mutex
	tmuxState tmuxPane
	tmuxAsked time.Time
	// tmuxFormat expands a tmux format for matcha's pane (a seam for tests).
	tmuxFormat = runTmuxFormat
	// tmuxAnswerTTL is how long an answer is used: another terminal may
	// attach to the session at any time.
	tmuxAnswerTTL = time.Second
)

// currentTmux asks tmux about the pane, at most once per tmuxAnswerTTL.
func currentTmux() tmuxPane {
	tmuxMu.Lock()
	defer tmuxMu.Unlock()
	if !tmuxAsked.IsZero() && time.Since(tmuxAsked) < tmuxAnswerTTL {
		return tmuxState
	}
	tmuxAsked = time.Now()
	tmuxState = tmuxPane{}
	out, err := tmuxFormat("#{client_termname}|#{allow-passthrough}|#{client_termtype}")
	if err != nil {
		debugImageProtocol("tmux query failed: %v", err)
		return tmuxState
	}
	// Formats tmux does not know expand to nothing: allow-passthrough
	// before 3.3 (which always passed sequences through), client_termtype
	// in old versions.
	fields := append(strings.SplitN(strings.TrimSpace(out), "|", 3), "", "")
	term, passthrough, termType := fields[0], fields[1], fields[2]
	tmuxState = tmuxPane{
		attached:   term != "" || termType != "",
		clientTerm: strings.ToLower(term + " " + termType),
		// "on", "all", or "1" (a flag in tmux 3.3 and 3.3a).
		passthrough:       passthrough != "off" && passthrough != "0",
		passthroughHidden: passthrough == "all" || passthrough == "",
	}
	debugImageProtocol("tmux client=%q type=%q allow-passthrough=%q", term, termType, passthrough)
	return tmuxState
}

// RefreshImageDetection makes the next image protocol check ask tmux again,
// for example when another terminal may have attached.
func RefreshImageDetection() {
	tmuxMu.Lock()
	tmuxAsked = time.Time{}
	tmuxMu.Unlock()
}

func runTmuxFormat(format string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	args := []string{"display-message", "-p"}
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		args = append(args, "-t", pane)
	}
	out, err := exec.CommandContext(ctx, "tmux", append(args, format)...).Output()
	return string(out), err
}

// PlaceholderUploadsReachTerminal reports whether an image uploaded through
// tmux passthrough right now reaches a terminal. tmux drops passthrough
// while no terminal is attached and, unless allow-passthrough is "all",
// while matcha's pane is not on screen (another window, or another pane
// zoomed).
func PlaceholderUploadsReachTerminal() bool {
	if !inTmux() {
		return true
	}
	if currentTmux().passthroughHidden {
		return true
	}
	out, err := tmuxFormat("#{session_attached}|#{window_active}|#{window_zoomed_flag}|#{pane_active}")
	if err != nil {
		return true // cannot tell; send it anyway
	}
	f := append(strings.Split(strings.TrimSpace(out), "|"), "", "", "", "")
	attached, windowActive, zoomed, paneActive := f[0], f[1], f[2], f[3]
	return attached != "0" && windowActive == "1" && (zoomed != "1" || paneActive == "1")
}

// PrimeImageDetection does the slow part of image protocol detection (asking
// tmux about the pane) ahead of time; call it in the background at startup.
func PrimeImageDetection() {
	if inTmux() {
		currentTmux()
	}
}

// tmuxPassthrough wraps each escape sequence of seq (a series of complete
// APC sequences) for tmux passthrough. Each sequence is wrapped on its own:
// tmux discards a passthrough sequence longer than its input buffer.
func tmuxPassthrough(seq string) string {
	var b strings.Builder
	for seq != "" {
		end := strings.Index(seq, "\x1b\\")
		if end < 0 {
			b.WriteString(ansi.TmuxPassthrough(seq))
			break
		}
		b.WriteString(ansi.TmuxPassthrough(seq[:end+2]))
		seq = seq[end+2:]
	}
	return b.String()
}

// Placeholder image ids. A placeholder cell names its image through its
// foreground colour, plus an optional third diacritic for the id's most
// significant byte. The colour is a 256-colour palette index, because tmux
// turns truecolour into the nearest palette colour when the outer terminal's
// RGB support is not enabled; indexes below 16 are avoided, as renderers may
// rewrite those as basic ANSI colours. The high byte is chosen at random per
// process so matcha's ids do not collide with images of other programs in
// other panes, which mostly use small ids.
const (
	minPlaceholderLow = 16
	maxPlaceholderLow = 255
	// maxPlaceholderCells is the number of row/column diacritics, which
	// bounds the size of a placeholder image in cells.
	maxPlaceholderCells = 297
)

var placeholderHigh = uint32(rand.IntN(255) + 1) //nolint:gosec // not security sensitive

// placeholderIDs hands out the low bytes, keyed by prepared image. An image
// keeps its id while it is among the most recently used ones, so the
// images of one email never share an id; the least recently used image
// gives its id up when the pool runs out.
var placeholderIDs = newPlaceholderIDPool()

type placeholderIDPool struct {
	mu    sync.Mutex
	byKey *simplelru.LRU[encodedKey, uint32]
	free  []uint32
}

func newPlaceholderIDPool() *placeholderIDPool {
	p := &placeholderIDPool{}
	for low := minPlaceholderLow; low <= maxPlaceholderLow; low++ {
		p.free = append(p.free, uint32(low))
	}
	byKey, err := simplelru.NewLRU(len(p.free), func(_ encodedKey, id uint32) {
		p.free = append(p.free, id&0xff) // runs under p.mu, inside the LRU call
	})
	if err != nil {
		panic(err) // only fails on size <= 0
	}
	p.byKey = byKey
	return p
}

// id returns the placeholder image id for key.
func (p *placeholderIDPool) id(key encodedKey) uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id, ok := p.byKey.Get(key); ok {
		return id
	}
	if len(p.free) == 0 {
		p.byKey.RemoveOldest()
	}
	low := p.free[len(p.free)-1]
	p.free = p.free[:len(p.free)-1]
	id := placeholderHigh<<24 | low
	p.byKey.Add(key, id)
	return id
}

// kittyPlaceholderUpload uploads a PNG under id through tmux passthrough and
// creates the virtual placement that placeholder cells refer to.
func kittyPlaceholderUpload(id uint32, pngData []byte, cols, rows int) string {
	virtual := fmt.Sprintf("\x1b_Ga=p,U=1,i=%d,c=%d,r=%d,q=2\x1b\\", id, cols, rows)
	return tmuxPassthrough(kittyTransmitSequence(id, pngData) + virtual)
}

// KittyPlaceholderFreeSequence frees a placeholder image in the terminal.
func KittyPlaceholderFreeSequence(id uint32) string {
	return tmuxPassthrough(KittyFreeImageSequence(id))
}

// kittyPlaceholderRows returns the text that shows image id over cols×rows
// cells: one line per row, each cell a placeholder carrying its row, column
// and the id's high byte as diacritics.
func kittyPlaceholderRows(id uint32, cols, rows int) []string {
	low, high := id&0xff, id>>24
	lines := make([]string, rows)
	for r := range rows {
		var b strings.Builder
		fmt.Fprintf(&b, "\x1b[38;5;%dm", low)
		for c := range cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
			b.WriteRune(kitty.Diacritic(int(high)))
		}
		b.WriteString("\x1b[39m")
		lines[r] = b.String()
	}
	return lines
}
