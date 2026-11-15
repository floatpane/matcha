package view

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/floatpane/matcha/clib"
	"github.com/floatpane/matcha/internal/httpclient"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

const (
	// remoteImageCacheSize and remoteImageCacheBytes bound the cache of
	// downloaded images, by count and by total payload size.
	remoteImageCacheSize  = 64
	remoteImageCacheBytes = 64 << 20

	// maxRemoteImageBytes caps a download, so a malicious or very large
	// image cannot exhaust memory.
	maxRemoteImageBytes = 10 << 20
	// maxRemoteImageSide caps either dimension declared in an image header.
	maxRemoteImageSide = 16384

	// remoteImageWorkers downloads run at once; remoteImageDeadline bounds a
	// whole load, so many slow hosts cannot keep the view waiting.
	remoteImageWorkers  = 6
	remoteImageDeadline = 30 * time.Second

	// Some hosts (e.g. upload.wikimedia.org) refuse Go's default agent.
	remoteImageUserAgent = "matcha (+https://github.com/floatpane/matcha)"
	remoteImageAccept    = "image/png,image/jpeg,image/gif,image/webp,image/bmp;q=0.9,*/*;q=0.5"
)

// remoteImageStall is how long a download may go without progress (a
// variable so tests can shorten it).
var remoteImageStall = httpclient.RemoteImageTimeout

// remoteImageCache maps image URLs to base64 payloads ready for
// prepareImage. Failures are not cached, so a retry fetches them again.
var remoteImageCache = newImageCache(remoteImageCacheSize, remoteImageCacheBytes)

// imageCache is an LRU cache bounded by entry count and by total size.
type imageCache struct {
	mu       sync.Mutex
	entries  *simplelru.LRU[string, string]
	bytes    int
	maxBytes int
}

func newImageCache(size, maxBytes int) *imageCache {
	c := &imageCache{maxBytes: maxBytes}
	entries, err := simplelru.NewLRU(size, func(_ string, payload string) {
		c.bytes -= len(payload) // runs under c.mu, inside the LRU call
	})
	if err != nil {
		panic(err) // only fails on size <= 0
	}
	c.entries = entries
	return c
}

func (c *imageCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries.Get(key)
}

// Add stores payload, evicting the oldest entries while the cache holds more
// than its byte budget. Payloads larger than a quarter of the budget are not
// kept.
func (c *imageCache) Add(key, payload string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries.Remove(key) // replacing a value does not report the old one
	if len(payload) > c.maxBytes/4 {
		return
	}
	c.bytes += len(payload)
	c.entries.Add(key, payload)
	for c.bytes > c.maxBytes && c.entries.Len() > 1 {
		c.entries.RemoveOldest()
	}
}

func (c *imageCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries.Len()
}

// Bytes returns the total size of the cached payloads.
func (c *imageCache) Bytes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

func (c *imageCache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries.Purge()
}

func cacheRemoteImage(rawURL, payload string) {
	remoteImageCache.Add(rawURL, payload)
}

// fetchRemoteImages downloads urls concurrently and returns the payload of
// each one that could be loaded. Missing entries failed (the reason is
// logged when image debugging is on).
func fetchRemoteImages(urls []string) map[string]string {
	payloads := make(map[string]string, len(urls))
	if len(urls) == 0 {
		return payloads
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteImageDeadline)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan string)
	for range min(remoteImageWorkers, len(urls)) {
		wg.Go(func() {
			for rawURL := range jobs {
				payload, err := fetchRemoteImage(ctx, rawURL)
				if err != nil {
					debugImageProtocol("remote image failed url=%s err=%v", rawURL, err)
					continue
				}
				mu.Lock()
				payloads[rawURL] = payload
				mu.Unlock()
			}
		})
	}
	for _, rawURL := range urls {
		jobs <- rawURL
	}
	close(jobs)
	wg.Wait()
	return payloads
}

// fetchRemoteImage downloads one image (or takes it from the cache) and
// returns it as a base64 payload for prepareImage.
func fetchRemoteImage(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !validRemoteImageURL(parsed) {
		return "", errors.New("not an allowed remote image URL")
	}
	if cached, ok := remoteImageCache.Get(rawURL); ok {
		debugImageProtocol("remote cache hit url=%s", rawURL)
		return cached, nil
	}

	// Give up on a download that stops making progress: no connection,
	// no response, or no body data for remoteImageStall.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stalled := time.AfterFunc(remoteImageStall, cancel)
	defer stalled.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", remoteImageUserAgent)
	req.Header.Set("Accept", remoteImageAccept)
	resp, err := remoteImageClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	body := progressReader{r: resp.Body, progress: func() { stalled.Reset(remoteImageStall) }}
	data, err := io.ReadAll(io.LimitReader(body, maxRemoteImageBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxRemoteImageBytes {
		return "", fmt.Errorf("image is larger than %d MB", maxRemoteImageBytes>>20)
	}
	payload, err := remoteImagePayload(data)
	if err != nil {
		return "", err
	}
	debugImageProtocol("remote fetch ok url=%s len=%d", rawURL, len(payload))
	cacheRemoteImage(rawURL, payload)
	return payload, nil
}

// progressReader calls progress after every read that returned data.
type progressReader struct {
	r        io.Reader
	progress func()
}

func (p progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.progress()
	}
	return n, err
}

// remoteImagePayload checks downloaded bytes and encodes them for
// prepareImage. Only the header is parsed here: formats the sandboxed
// decoder reads (PNG, JPEG, GIF, BMP) are passed through untouched, and
// WebP, which it cannot read, is converted to PNG with Go's memory-safe
// decoder.
func remoteImagePayload(data []byte) (string, error) {
	format, w, h, ok := clib.ImageFormat(data)
	if !ok {
		return "", errors.New("unsupported image format")
	}
	if w < 1 || h < 1 || w > maxRemoteImageSide || h > maxRemoteImageSide ||
		int64(w)*int64(h) > clib.MaxDecodePixels {
		return "", fmt.Errorf("%s image is %dx%d pixels, which is too large", format, w, h)
	}
	switch format {
	case "png", "jpeg", "gif", "bmp":
		return base64.StdEncoding.EncodeToString(data), nil
	}
	converted, ok := clib.DecodeToPNG(data)
	if !ok {
		return "", fmt.Errorf("could not decode %s image", format)
	}
	return base64.StdEncoding.EncodeToString(converted.PNGData), nil
}
