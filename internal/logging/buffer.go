package logging

import (
	"strings"
	"sync"
)

type Buffer struct {
	mu         sync.Mutex
	maxEntries int
	entries    []Entry
	subs       []chan Entry
}

func NewBuffer(maxEntries int) *Buffer {
	if maxEntries < 1 {
		maxEntries = DefaultMaxEntries
	}
	return &Buffer{maxEntries: maxEntries}
}

func (b *Buffer) MaxEntries() int {
	return b.maxEntries
}

func (b *Buffer) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line == "" {
			continue
		}
		b.append(Entry{Text: line})
	}
	return len(p), nil
}

func (b *Buffer) Subscribe() <-chan Entry {
	ch := make(chan Entry, 64)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

func (b *Buffer) Tail(n int) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n <= 0 || len(b.entries) == 0 {
		return nil
	}
	if n > len(b.entries) {
		n = len(b.entries)
	}

	start := len(b.entries) - n
	entries := make([]Entry, n)
	copy(entries, b.entries[start:])
	return entries
}

func (b *Buffer) append(entry Entry) {
	b.mu.Lock()
	if len(b.entries) >= b.maxEntries {
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = entry
	} else {
		b.entries = append(b.entries, entry)
	}

	subs := append([]chan Entry(nil), b.subs...)
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
}
