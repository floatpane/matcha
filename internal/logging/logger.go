package logging

import "io"

const DefaultMaxEntries = 10

type Entry struct {
	Text string
}

type Logger interface {
	io.Writer
	MaxEntries() int
	Tail(n int) []Entry
	Subscribe() <-chan Entry
}
