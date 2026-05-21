package pgp

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

type failingLengthWriter struct{}

func (failingLengthWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("simulated length write failure")
}

// TestParseASN1Signature_TruncatedDoesNotPanic covers the bounds-check path
// added for #613. Each input would have panicked in the original parser
// with "index out of range"; here we expect a typed error instead.
func TestParseASN1Signature_TruncatedDoesNotPanic(t *testing.T) {
	cases := []struct {
		name    string
		der     []byte
		wantErr string
	}{
		{
			// Length byte declares 0x10 bytes of R but only 1 byte follows.
			name:    "R length overruns buffer",
			der:     []byte{0x30, 0x06, 0x02, 0x10, 0xAA, 0x00},
			wantErr: "R length overflow",
		},
		{
			// Length byte declares 0x10 bytes of S but only 1 byte follows.
			name:    "S length overruns buffer",
			der:     []byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x02, 0x10, 0xAA},
			wantErr: "S length overflow",
		},
		{
			// Valid R, then no S block at all.
			name:    "missing S after R",
			der:     []byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x00},
			wantErr: "expected INTEGER tag for S",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// The test must not panic: the fix replaces panics with errors.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("parseASN1Signature panicked: %v", r)
				}
			}()
			_, _, err := parseASN1Signature(tc.der)
			if err == nil {
				t.Fatalf("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestParseASN1Signature_WellFormed guards against regressions in the
// happy path: a minimal SEQUENCE { INTEGER, INTEGER } must still decode
// to the original r and s bytes.
func TestParseASN1Signature_WellFormed(t *testing.T) {
	// SEQUENCE (6 bytes) { INTEGER 0x01, INTEGER 0x02 }
	der := []byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x02, 0x01, 0x02}

	r, s, err := parseASN1Signature(der)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r) != 1 || r[0] != 0x01 {
		t.Errorf("r = %x, want 01", r)
	}
	if len(s) != 1 || s[0] != 0x02 {
		t.Errorf("s = %x, want 02", s)
	}
}

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

func TestWriteNewFormatLengthEncodings(t *testing.T) {
	cases := []struct {
		length int
		want   []byte
	}{
		{length: 191, want: []byte{0xbf}},
		{length: 192, want: []byte{0xc0, 0x00}},
		{length: 8383, want: []byte{0xdf, 0xff}},
		{length: 8384, want: []byte{0xff, 0x00, 0x00, 0x20, 0xc0}},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.length), func(t *testing.T) {
			var got strings.Builder
			if err := writeNewFormatLength(&got, tc.length); err != nil {
				t.Fatalf("writeNewFormatLength() returned error: %v", err)
			}
			if string(tc.want) != got.String() {
				t.Fatalf("writeNewFormatLength(%d) = %x, want %x", tc.length, got.String(), tc.want)
			}
		})
	}
}

func TestWriteNewFormatLengthPropagatesWriteError(t *testing.T) {
	err := writeNewFormatLength(failingLengthWriter{}, 8384)
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !strings.Contains(err.Error(), "OpenPGP packet length") {
		t.Fatalf("expected length write context, got %v", err)
	}
}

var _ io.Writer = failingLengthWriter{}
