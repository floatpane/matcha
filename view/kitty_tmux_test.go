package view

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// useTmux pretends to run inside a tmux pane about which tmux answers out
// ("client-term|allow-passthrough|client-type") and err.
func useTmux(t *testing.T, out string, err error) {
	t.Helper()
	t.Setenv("TMUX", "/private/tmp/tmux-501/default,1234,0")
	t.Setenv("TMUX_PANE", "%0")
	t.Setenv("TERM", "tmux-256color")
	t.Setenv("NO_COLOR", "")
	previous := tmuxFormat
	tmuxFormat = func(string) (string, error) { return out, err }
	reset := func() {
		tmuxAsked = time.Time{}
		tmuxState = tmuxPane{}
	}
	reset()
	t.Cleanup(func() {
		tmuxFormat = previous
		reset()
	})
}

func TestTmuxImageProtocol(t *testing.T) {
	ghostty := map[string]string{"GHOSTTY_RESOURCES_DIR": "/g"}
	cases := []struct {
		name string
		env  map[string]string
		out  string
		err  error
		want ImageProtocol
	}{
		{"ghostty with passthrough", ghostty, "xterm-ghostty|on\n", nil, ImageProtocolKittyPlaceholder},
		{"kitty with passthrough all", map[string]string{"KITTY_WINDOW_ID": "1"}, "xterm-kitty|all\n", nil, ImageProtocolKittyPlaceholder},
		{"ghostty known only to tmux", nil, "xterm-ghostty|on\n", nil, ImageProtocolKittyPlaceholder},
		{"passthrough off", ghostty, "xterm-ghostty|off\n", nil, ImageProtocolHalfBlock},
		{"tmux before 3.3 always passes through", ghostty, "xterm-ghostty|\n", nil, ImageProtocolKittyPlaceholder},
		{"tmux unreachable", ghostty, "", errors.New("exit status 1"), ImageProtocolHalfBlock},
		{"wezterm has no placeholders", map[string]string{"WEZTERM_EXECUTABLE": "/w"}, "xterm-256color|on\n", nil, ImageProtocolHalfBlock},
		{"plain terminal", nil, "xterm-256color|on\n", nil, ImageProtocolNone},
		{"server started from Ghostty, iTerm2 attached", ghostty, "xterm-256color|on|iTerm2 3.5.10\n", nil, ImageProtocolHalfBlock},
		{"Ghostty named by XTVERSION", nil, "xterm-256color|on|ghostty 1.2.3\n", nil, ImageProtocolKittyPlaceholder},
		{"no terminal attached: the environment decides", ghostty, "|on|\n", nil, ImageProtocolKittyPlaceholder},
		{"tmux 3.3 reports the flag as 1", ghostty, "xterm-ghostty|1\n", nil, ImageProtocolKittyPlaceholder},
		{"tmux 3.3 reports off as 0", ghostty, "xterm-ghostty|0\n", nil, ImageProtocolHalfBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useImageTerminal(t, tc.env)
			useTmux(t, tc.out, tc.err)
			if got := ActiveImageProtocol(); got != tc.want {
				t.Fatalf("ActiveImageProtocol() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTmuxPassthroughWrapsEachSequence(t *testing.T) {
	got := tmuxPassthrough("\x1b_Ga=t,m=1;AAAA\x1b\\\x1b_Gm=0;BBBB\x1b\\")
	want := "\x1bPtmux;\x1b\x1b_Ga=t,m=1;AAAA\x1b\x1b\\\x1b\\" + "\x1bPtmux;\x1b\x1b_Gm=0;BBBB\x1b\x1b\\\x1b\\"
	if got != want {
		t.Fatalf("tmuxPassthrough = %q\nwant %q", got, want)
	}
}

func TestPlaceholderImagesAreDrawnAsText(t *testing.T) {
	useImageTerminal(t, map[string]string{"GHOSTTY_RESOURCES_DIR": "/g"})
	useTmux(t, "xterm-ghostty|on\n", nil)
	body := `<p>before</p><img src="data:image/png;base64,` + onePixelPNG + `" alt="pic"><p>after</p>`
	style := lipgloss.NewStyle()
	rendered, err := RenderBody(body, BodyMIMETypeHTML, nil, style, style, style, RenderOptions{Bounds: ImageBounds{MaxCols: 20, MaxRows: 10}})
	if err != nil {
		t.Fatal(err)
	}
	text, placements := rendered.Finalize(rendered.Text)
	if len(placements) != 1 || placements[0].Protocol != ImageProtocolKittyPlaceholder {
		t.Fatalf("placements = %+v, want one placeholder image to upload", placements)
	}
	p := placements[0]
	if low, high := p.ID&0xff, p.ID>>24; low < minPlaceholderLow || high != placeholderHigh {
		t.Fatalf("image id %#x: low byte %d, high byte %d (want >= %d and %d)", p.ID, low, high, minPlaceholderLow, placeholderHigh)
	}

	// The upload goes through tmux passthrough, one wrapped sequence per
	// command, and ends with the virtual placement the cells refer to.
	if strings.Contains(strings.ReplaceAll(p.Encoded, "\x1b\x1b", ""), "\x1b_G") {
		t.Fatalf("a graphics command is not wrapped for tmux: %q", p.Encoded)
	}
	if want := fmt.Sprintf("a=p,U=1,i=%d,c=%d,r=%d", p.ID, p.Cols, p.Rows); !strings.Contains(p.Encoded, want) {
		t.Fatalf("upload %q has no virtual placement %q", p.Encoded, want)
	}

	// The image itself is text: rows of placeholder cells.
	var rows []string
	for _, line := range strings.Split(text, "\n") {
		if strings.ContainsRune(line, kitty.Placeholder) {
			rows = append(rows, line)
		}
	}
	if len(rows) != p.Rows {
		t.Fatalf("%d placeholder rows, want %d", len(rows), p.Rows)
	}
	for r, line := range rows {
		if w := ansi.StringWidth(line); w != p.Cols {
			t.Fatalf("row %d is %d cells wide, want %d", r, w, p.Cols)
		}
		first := fmt.Sprintf("\x1b[38;5;%dm%c%c%c%c", p.ID&0xff, kitty.Placeholder, kitty.Diacritic(r), kitty.Diacritic(0), kitty.Diacritic(int(p.ID>>24)))
		if !strings.HasPrefix(line, first) {
			t.Fatalf("row %d starts with %q, want %q", r, line, first)
		}
	}
	if !strings.Contains(text, "before") || !strings.Contains(text, "after") {
		t.Fatalf("surrounding text lost: %q", ansi.Strip(text))
	}
}

func TestPlaceholderIDs(t *testing.T) {
	pool := newPlaceholderIDPool()
	size := maxPlaceholderLow - minPlaceholderLow + 1
	ids := make([]uint32, size)
	seen := map[uint32]bool{}
	for i := range size {
		id := pool.id(encodedKey{cols: i})
		if low := id & 0xff; low < minPlaceholderLow || id>>24 != placeholderHigh || seen[id] {
			t.Fatalf("image %d got id %#x (duplicate=%v)", i, id, seen[id])
		}
		ids[i], seen[id] = id, true
	}
	// Images keep their ids; using 0 and 3 again makes 1 the least
	// recently used image, whose id goes to the next new image.
	if pool.id(encodedKey{cols: 0}) != ids[0] || pool.id(encodedKey{cols: 3}) != ids[3] {
		t.Fatal("an image did not keep its id")
	}
	if fresh := pool.id(encodedKey{cols: size}); fresh != ids[1] {
		t.Fatalf("new image got id %#x, want the least recently used image's %#x", fresh, ids[1])
	}
	if pool.id(encodedKey{cols: 0}) != ids[0] || pool.id(encodedKey{cols: 3}) != ids[3] {
		t.Fatal("a recently used image lost its id")
	}
}

func TestTmuxNoColorShowsNoImages(t *testing.T) {
	useImageTerminal(t, map[string]string{"GHOSTTY_RESOURCES_DIR": "/g"})
	useTmux(t, "xterm-ghostty|on\n", nil)
	t.Setenv("NO_COLOR", "1") // the renderer strips the colours images need
	if got := ActiveImageProtocol(); got != ImageProtocolNone {
		t.Fatalf("ActiveImageProtocol() = %v with NO_COLOR, want none", got)
	}
}

func TestTmuxAnswerIsAskedAgain(t *testing.T) {
	useImageTerminal(t, map[string]string{"GHOSTTY_RESOURCES_DIR": "/g"}) // server started from Ghostty
	useTmux(t, "xterm-ghostty|on\n", nil)
	if got := ActiveImageProtocol(); got != ImageProtocolKittyPlaceholder {
		t.Fatalf("Ghostty attached: %v", got)
	}
	tmuxFormat = func(string) (string, error) { return "xterm-256color|on|iTerm2 3.5.10\n", nil }
	if got := ActiveImageProtocol(); got != ImageProtocolKittyPlaceholder {
		t.Fatalf("the answer was not reused within its lifetime: %v", got)
	}
	RefreshImageDetection() // another terminal attached
	if got := ActiveImageProtocol(); got != ImageProtocolHalfBlock {
		t.Fatalf("iTerm2 attached: %v, want half-blocks", got)
	}
}

func TestPlaceholderUploadsReachTerminal(t *testing.T) {
	cases := []struct {
		name, passthrough, visibility string
		want                          bool
	}{
		{"pane on screen", "on", "1|1|0|1", true},
		{"window in the background", "on", "1|0|0|1", false},
		{"no terminal attached", "on", "0|1|0|1", false},
		{"another pane zoomed", "on", "1|1|1|0", false},
		{"this pane zoomed", "on", "1|1|1|1", true},
		{"allow-passthrough all", "all", "1|0|0|1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useTmux(t, "", nil)
			tmuxFormat = func(format string) (string, error) {
				if strings.Contains(format, "session_attached") {
					return tc.visibility + "\n", nil
				}
				return "xterm-ghostty|" + tc.passthrough + "\n", nil
			}
			if got := PlaceholderUploadsReachTerminal(); got != tc.want {
				t.Fatalf("PlaceholderUploadsReachTerminal() = %v, want %v", got, tc.want)
			}
		})
	}
}

// colorPNG returns a 2x2 PNG of one colour as a base64 payload. Its pixels
// are put in the decoded-image cache, so preparing it starts no sandboxed
// decoder (hundreds of decodes would make the test slow, worse with -race).
func colorPNG(t *testing.T, c int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = byte(c), byte(c>>8), byte(c>>16), 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	decodedImages.Add(sha256.Sum256(buf.Bytes()), img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestRecurringImageKeepsItsPlaceholderID(t *testing.T) {
	useImageTerminal(t, map[string]string{"GHOSTTY_RESOURCES_DIR": "/g"})
	useTmux(t, "xterm-ghostty|on\n", nil)
	previous := placeholderIDs
	placeholderIDs = newPlaceholderIDPool()
	encodedImages.Purge()
	t.Cleanup(func() {
		placeholderIDs = previous
		encodedImages.Purge()
		decodedImages.Purge()
	})
	bounds := ImageBounds{MaxCols: 4, MaxRows: 2}
	logo := colorPNG(t, 0)
	first, err := prepareImage(logo, ImageProtocolKittyPlaceholder, bounds)
	if err != nil {
		t.Fatal(err)
	}
	// Every email shows the logo (served from the cache) and a new picture.
	for i := 1; i <= maxPlaceholderLow-minPlaceholderLow+20; i++ {
		logo = colorPNG(t, 0) // keep its pixels cached too
		again, err := prepareImage(logo, ImageProtocolKittyPlaceholder, bounds)
		if err != nil {
			t.Fatal(err)
		}
		picture, err := prepareImage(colorPNG(t, i), ImageProtocolKittyPlaceholder, bounds)
		if err != nil {
			t.Fatal(err)
		}
		if picture.id == again.id {
			t.Fatalf("email %d: the logo and a new picture share id %#x (the logo first had %#x)", i, picture.id, first.id)
		}
	}
}

func TestTmuxAnswerExpires(t *testing.T) {
	useImageTerminal(t, map[string]string{"GHOSTTY_RESOURCES_DIR": "/g"})
	useTmux(t, "xterm-ghostty|on\n", nil)
	previous := tmuxAnswerTTL
	tmuxAnswerTTL = 20 * time.Millisecond
	t.Cleanup(func() { tmuxAnswerTTL = previous })
	if got := ActiveImageProtocol(); got != ImageProtocolKittyPlaceholder {
		t.Fatalf("Ghostty attached: %v", got)
	}
	tmuxFormat = func(string) (string, error) { return "xterm-256color|on|iTerm2 3.5.10\n", nil }
	time.Sleep(40 * time.Millisecond) // another terminal attached meanwhile
	if got := ActiveImageProtocol(); got != ImageProtocolHalfBlock {
		t.Fatalf("an expired answer was used: %v, want half-blocks for iTerm2", got)
	}
}
