package view

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// onePixelWebP is a 1x1 lossless WebP, a format the sandboxed decoder
// cannot read.
const onePixelWebP = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

// useRemoteImageServer routes every remote image request to handler, as if
// the image hosts were public.
func useRemoteImageServer(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("TERM", "xterm-kitty")
	remoteImageCache.Purge()
	t.Cleanup(remoteImageCache.Purge)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previous := remoteImageClient
	remoteImageClient = newRemoteImageClient(
		func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
		func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	)
	t.Cleanup(func() { remoteImageClient = previous })
}

func mustBase64(t *testing.T, s string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func renderRemote(t *testing.T, body string, opts RenderOptions) (*RenderedBody, []ImagePlacement) {
	t.Helper()
	style := lipgloss.NewStyle()
	opts.AllowRemoteImages = true
	rendered, err := RenderBody(body, BodyMIMETypeHTML, nil, style, style, style, opts)
	if err != nil {
		t.Fatal(err)
	}
	_, placements := rendered.Finalize(rendered.Text)
	return rendered, placements
}

func TestRemoteImageURLReachesServerUnchanged(t *testing.T) {
	png := mustBase64(t, onePixelPNG)
	var mu sync.Mutex
	var gotURI, gotUA string
	useRemoteImageServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotURI, gotUA = r.RequestURI, r.UserAgent()
		mu.Unlock()
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))

	// Bodies reach the renderer transfer-decoded, so "=16" is literal URL
	// text, and the sanitizer writes "&" in attributes as "&amp;".
	body := `<p>x</p><img src="http://images.example/p.png?v=1612345678&amp;s=200" alt="remote">`
	rendered, placements := renderRemote(t, body, RenderOptions{})
	mu.Lock()
	defer mu.Unlock()
	if gotURI != "/p.png?v=1612345678&s=200" || len(placements) != 1 {
		t.Fatalf("server got %q and the render made %d placements, want /p.png?v=1612345678&s=200 and 1 image", gotURI, len(placements))
	}
	if strings.HasPrefix(gotUA, "Go-http-client") {
		t.Fatalf("remote image request used Go's default User-Agent %q", gotUA)
	}
	if rendered.RemoteImages != 1 || len(rendered.FailedRemoteImages) != 0 {
		t.Fatalf("RemoteImages = %d, FailedRemoteImages = %v, want 1 and none", rendered.RemoteImages, rendered.FailedRemoteImages)
	}
}

func TestRenderBodyKeepsEqualsHexSequences(t *testing.T) {
	clearAllTerminalEnv()
	style := lipgloss.NewStyle()
	// Already transfer-decoded text; "=20" and "=3D" must not be decoded again.
	rendered, err := RenderBody("<p>a=20 b=3D c=1A</p>", BodyMIMETypeHTML, nil, style, style, style, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if text := ansi.Strip(rendered.Text); !strings.Contains(text, "a=20 b=3D c=1A") {
		t.Fatalf("body text was corrupted: %q", text)
	}
}

func TestImageAltTextIsUnescaped(t *testing.T) {
	clearAllTerminalEnv()
	style := lipgloss.NewStyle()
	rendered, err := RenderBody(`<img src="cid:none" alt="Tom &amp; Jerry">`, BodyMIMETypeHTML, nil, style, style, style, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if text := ansi.Strip(rendered.Text); !strings.Contains(text, "Tom & Jerry") {
		t.Fatalf("alt text not unescaped: %q", text)
	}
}

func TestRemoteImageFailuresAreReported(t *testing.T) {
	png := mustBase64(t, onePixelPNG)
	var requests sync.Map
	useRemoteImageServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Store(r.URL.Path, true)
		switch r.URL.Path {
		case "/ok.png":
			_, _ = w.Write(png)
		case "/notimage":
			_, _ = w.Write([]byte("<svg xmlns='http://www.w3.org/2000/svg'/>"))
		default:
			http.NotFound(w, r)
		}
	}))
	body := `<img src="http://a.example/ok.png"><img src="http://a.example/missing.png">` +
		`<img src="http://a.example/notimage"><img src="http://a.example/ok.png"><img src="http://a.example/skipped.png">`
	rendered, placements := renderRemote(t, body, RenderOptions{
		SkipRemoteImages: map[string]bool{"http://a.example/skipped.png": true},
	})
	if len(placements) != 2 {
		t.Fatalf("got %d placements, want the two copies of ok.png", len(placements))
	}
	want := []string{"http://a.example/missing.png", "http://a.example/notimage", "http://a.example/skipped.png"}
	if rendered.RemoteImages != 4 || strings.Join(rendered.FailedRemoteImages, " ") != strings.Join(want, " ") {
		t.Fatalf("RemoteImages = %d, FailedRemoteImages = %v; want 4 and %v", rendered.RemoteImages, rendered.FailedRemoteImages, want)
	}
	if _, asked := requests.Load("/skipped.png"); asked {
		t.Fatal("a skipped remote image was downloaded")
	}
}

func TestRemoteImagesDownloadInParallel(t *testing.T) {
	png := mustBase64(t, onePixelPNG)
	var inFlight atomic.Int32
	bothArrived := make(chan struct{})
	var once sync.Once
	useRemoteImageServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if inFlight.Add(1) == 2 {
			once.Do(func() { close(bothArrived) })
		}
		// Serially fetched images would never be in flight together.
		select {
		case <-bothArrived:
			_, _ = w.Write(png)
		case <-time.After(3 * time.Second):
			http.Error(w, "fetched one at a time", http.StatusServiceUnavailable)
		}
	}))
	rendered, placements := renderRemote(t, `<img src="http://a.example/1.png"><img src="http://b.example/2.png">`, RenderOptions{})
	if len(placements) != 2 || len(rendered.FailedRemoteImages) != 0 {
		t.Fatalf("got %d placements and failures %v; remote images were not fetched in parallel", len(placements), rendered.FailedRemoteImages)
	}
}

func TestRemoteWebPImagesAreShown(t *testing.T) {
	webp := mustBase64(t, onePixelWebP)
	useRemoteImageServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(webp)
	}))
	rendered, placements := renderRemote(t, `<img src="http://images.example/pic.webp">`, RenderOptions{})
	if len(placements) != 1 || len(rendered.FailedRemoteImages) != 0 {
		t.Fatalf("WebP image: %d placements, failures %v", len(placements), rendered.FailedRemoteImages)
	}
}

// pngHeader returns a PNG signature and header chunk that declares w×h
// pixels, with no image data.
func pngHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	_ = binary.Write(&b, binary.BigEndian, uint32(len(ihdr)))
	chunk := append([]byte("IHDR"), ihdr...)
	b.Write(chunk)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}

func TestRemoteImagePayloadRejectsHugeDimensions(t *testing.T) {
	if _, err := remoteImagePayload(pngHeader(100000, 100000)); err == nil {
		t.Fatal("an image declaring 100000x100000 pixels was accepted")
	}
	if _, err := remoteImagePayload(pngHeader(9000, 9000)); err == nil {
		t.Fatal("an image declaring 81 megapixels was accepted")
	}
	payload, err := remoteImagePayload(mustBase64(t, onePixelPNG))
	if err != nil || payload != onePixelPNG {
		t.Fatalf("a small PNG should pass through unchanged, got %q, %v", payload, err)
	}
}

func TestRemoteImageCacheIsBoundedBySize(t *testing.T) {
	remoteImageCache.Purge()
	t.Cleanup(remoteImageCache.Purge)
	big := strings.Repeat("x", remoteImageCacheBytes/8)
	for i := range 12 {
		cacheRemoteImage("https://example.com/"+string(rune('a'+i)), big)
	}
	if total := remoteImageCache.Bytes(); total > remoteImageCacheBytes {
		t.Fatalf("cache holds %d bytes, want at most %d", total, remoteImageCacheBytes)
	}
	if _, ok := remoteImageCache.Get("https://example.com/l"); !ok {
		t.Fatal("the newest image was evicted")
	}
	cacheRemoteImage("https://example.com/l", "small") // replacing an entry
	if total, want := remoteImageCache.Bytes(), 7*len(big)+len("small"); total != want {
		t.Fatalf("cache accounts for %d bytes, want %d", total, want)
	}
	remoteImageCache.Purge()
	if total := remoteImageCache.Bytes(); total != 0 {
		t.Fatalf("an empty cache accounts for %d bytes", total)
	}
}
