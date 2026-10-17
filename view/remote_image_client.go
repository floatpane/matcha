package view

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/floatpane/matcha/internal/httpclient"
)

const maxRemoteImageRedirects = 5

type remoteImageLookup func(context.Context, string) ([]netip.Addr, error)
type remoteImageDial func(context.Context, string, string) (net.Conn, error)

var blockedRemoteImageRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func publicRemoteImageIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, blocked := range blockedRemoteImageRanges {
		if blocked.Contains(ip) {
			return false
		}
	}
	return true
}

func validRemoteImageURL(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") &&
		u.Hostname() != "" && u.User == nil && u.Opaque == ""
}

func lookupRemoteImageIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// newRemoteImageClient checks every resolved address, then dials one of those
// exact addresses. The transport does not use environment proxies, which could
// resolve the hostname again or make the request from a private network.
func newRemoteImageClient(lookup remoteImageLookup, dial remoteImageDial) *http.Client {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := lookup(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("remote image host could not be resolved: %w", err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("remote image host has no addresses")
			}
			for _, ip := range ips {
				if !publicRemoteImageIP(ip) {
					return nil, fmt.Errorf("remote image host resolves to a non-public address")
				}
			}
			var dialErr error
			for _, ip := range ips {
				conn, err := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
				if err == nil {
					return conn, nil
				}
				dialErr = err
			}
			return nil, dialErr
		},
		TLSHandshakeTimeout: 5 * time.Second,
	}
	return &http.Client{
		Timeout:   httpclient.RemoteImageTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRemoteImageRedirects || !validRemoteImageURL(req.URL) {
				return fmt.Errorf("remote image redirect is not allowed")
			}
			return nil
		},
	}
}

var remoteImageClient = newRemoteImageClient(lookupRemoteImageIPs, (&net.Dialer{}).DialContext)
