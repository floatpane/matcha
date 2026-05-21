package logging

import "testing"

func TestBufferStoresLines(t *testing.T) {
	buffer := NewBuffer(DefaultMaxEntries)

	if _, err := buffer.Write([]byte("first\nsecond\n")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	got := buffer.Tail(DefaultMaxEntries)
	if len(got) != 2 {
		t.Fatalf("Tail returned %d entries, want 2", len(got))
	}
	if got[0].Text != "first" || got[1].Text != "second" {
		t.Fatalf("unexpected entries: %+v", got)
	}
}

func TestBufferKeepsLastMaxEntries(t *testing.T) {
	buffer := NewBuffer(DefaultMaxEntries)

	for i := 0; i < DefaultMaxEntries+2; i++ {
		if _, err := buffer.Write([]byte{byte('a' + i), '\n'}); err != nil {
			t.Fatalf("Write returned error: %v", err)
		}
	}

	got := buffer.Tail(DefaultMaxEntries)
	if len(got) != DefaultMaxEntries {
		t.Fatalf("Tail returned %d entries, want %d", len(got), DefaultMaxEntries)
	}
	if got[0].Text != "c" {
		t.Fatalf("first retained entry = %q, want %q", got[0].Text, "c")
	}
}

func TestBufferTailReturnsRequestedCount(t *testing.T) {
	buffer := NewBuffer(DefaultMaxEntries)

	for _, line := range []string{"first\n", "second\n", "third\n"} {
		if _, err := buffer.Write([]byte(line)); err != nil {
			t.Fatalf("Write returned error: %v", err)
		}
	}

	got := buffer.Tail(2)
	if len(got) != 2 {
		t.Fatalf("Tail returned %d entries, want 2", len(got))
	}
	if got[0].Text != "second" || got[1].Text != "third" {
		t.Fatalf("unexpected entries: %+v", got)
	}
}

func TestBufferTailReturnsNilForNonPositiveCount(t *testing.T) {
	buffer := NewBuffer(DefaultMaxEntries)

	if _, err := buffer.Write([]byte("first\n")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if got := buffer.Tail(0); got != nil {
		t.Fatalf("Tail(0) returned %+v, want nil", got)
	}
}

func TestNewBufferUsesDefaultForInvalidMax(t *testing.T) {
	buffer := NewBuffer(0)
	if got := buffer.MaxEntries(); got != DefaultMaxEntries {
		t.Fatalf("MaxEntries = %d, want %d", got, DefaultMaxEntries)
	}
}
