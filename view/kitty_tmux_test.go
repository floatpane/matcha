package view

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// useTmux pretends to run inside a tmux pane about which tmux answers out
// ("client-term|allow-passthrough") and err.
func useTmux(t *testing.T, out string, err error) {
	t.Helper()
	t.Setenv("TMUX", "/private/tmp/tmux-501/default,1234,0")
	t.Setenv("TMUX_PANE", "%0")
	t.Setenv("TERM", "tmux-256color")
	previous := tmuxQuery
	tmuxQuery = func() (string, error) { return out, err }
	reset := func() {
		tmuxOnce = sync.Once{}
		tmuxState = tmuxPane{}
	}
	reset()
	t.Cleanup(func() {
		tmuxQuery = previous
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
