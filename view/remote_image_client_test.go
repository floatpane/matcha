package view

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
)

func TestRemoteImageAddressPolicy(t *testing.T) {
	tests := []struct {
		address string
		allowed bool
	}{
		{"93.184.216.34", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"::ffff:127.0.0.1", false},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"100.64.0.1", false},
		{"192.0.2.1", false},
		{"198.18.0.1", false},
		{"0.0.0.0", false},
		{"::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
		{"64:ff9b::7f00:1", false},
		{"2001:db8::1", false},
		{"2002:7f00:1::1", false},
	}
	for _, tc := range tests {
		t.Run(tc.address, func(t *testing.T) {
			if got := publicRemoteImageIP(netip.MustParseAddr(tc.address)); got != tc.allowed {
				t.Fatalf("publicRemoteImageIP(%s) = %t, want %t", tc.address, got, tc.allowed)
			}
		})
	}
}

func testRemoteImageClient(t *testing.T, server *httptest.Server, lookup remoteImageLookup, dialed *[]string) *http.Client {
	t.Helper()
	return newRemoteImageClient(lookup, func(ctx context.Context, network, address string) (net.Conn, error) {
		*dialed = append(*dialed, address)
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	})
}

func TestRemoteImageClientPinsCheckedAddress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "image")
	}))
	t.Cleanup(server.Close)
	var dialed []string
	client := testRemoteImageClient(t, server, func(_ context.Context, host string) ([]netip.Addr, error) {
		if host != "images.example" {
			return nil, fmt.Errorf("unexpected host %q", host)
		}
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}, &dialed)
	resp, err := client.Get("http://images.example/image.png")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if len(dialed) != 1 || dialed[0] != "93.184.216.34:80" {
		t.Fatalf("dialed %v, want the checked IP and port", dialed)
	}
}

func TestRemoteImageClientRejectsPrivateOrMixedDNS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("blocked address reached the server")
	}))
	t.Cleanup(server.Close)
	for _, addresses := range [][]netip.Addr{
		{netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.1")},
	} {
		var dialed []string
		client := testRemoteImageClient(t, server, func(context.Context, string) ([]netip.Addr, error) {
			return addresses, nil
		}, &dialed)
		resp, err := client.Get("http://images.example/image.png")
		if resp != nil {
			_ = resp.Body.Close()
		}
		if err == nil || len(dialed) != 0 {
			t.Fatalf("addresses %v yielded err %v and dials %v", addresses, err, dialed)
		}
	}
}

func TestRemoteImageClientChecksRedirectTargets(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Redirect(w, r, "http://127.0.0.1/private", http.StatusFound)
	}))
	t.Cleanup(server.Close)
	var dialed []string
	client := testRemoteImageClient(t, server, func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "images.example" {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}
		return lookupRemoteImageIPs(context.Background(), host)
	}, &dialed)
	resp, err := client.Get("http://images.example/redirect")
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil || requests != 1 || len(dialed) != 1 {
		t.Fatalf("redirect yielded err %v, %d requests and dials %v", err, requests, dialed)
	}
}

func TestRemoteImageClientCapsRedirects(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		w.Header().Set("Location", fmt.Sprintf("/%d", n+1))
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)
	var dialed []string
	client := testRemoteImageClient(t, server, func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}, &dialed)
	resp, err := client.Get("http://images.example/0")
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil || requests != maxRemoteImageRedirects+1 {
		t.Fatalf("redirect chain yielded err %v after %d requests", err, requests)
	}
}

func TestRemoteImageFetchBlocksLocalServer(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	t.Cleanup(server.Close)
	if got := fetchRemoteBase64(server.URL + "/image.png"); got != "" || requests != 0 {
		t.Fatalf("private image fetch returned %q and made %d requests", got, requests)
	}
}
