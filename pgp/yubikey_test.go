package pgp

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// The OpenPGP signature packet construction and its ASN.1 / MPI helpers now
// live in github.com/floatpane/go-openpgp-card-hl and are tested there. What
// remains here is matcha's own MIME multipart/signed framing.

func TestGenerateMIMEBoundaryUsesCryptoRandomBytes(t *testing.T) {
	oldRandRead := randRead
	defer func() { randRead = oldRandRead }()

	randRead = func(p []byte) (int, error) {
		for i := range p {
			p[i] = byte(i)
		}
		return len(p), nil
	}

	got := generateMIMEBoundary()
	want := "----=_Part_000102030405060708090a0b0c0d0e0f"
	if got != want {
		t.Fatalf("boundary = %q, want %q", got, want)
	}
}

func TestGenerateMIMEBoundaryFallsBackToUnixNano(t *testing.T) {
	oldRandRead := randRead
	defer func() { randRead = oldRandRead }()

	randRead = func(_ []byte) (int, error) {
		return 0, errors.New("random source unavailable")
	}

	const prefix = "----=_Part_"
	got := generateMIMEBoundary()
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("boundary = %q, want prefix %q", got, prefix)
	}
	if _, err := strconv.ParseInt(strings.TrimPrefix(got, prefix), 10, 64); err != nil {
		t.Fatalf("fallback boundary suffix is not a UnixNano timestamp: %v", err)
	}
}
