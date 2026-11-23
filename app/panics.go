package app

import (
	"log"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"
)

// logPanic, deferred, writes a panic and its stack to the log and then
// re-raises it for Bubble Tea, which restores the terminal and exits. Bubble
// Tea prints panics while the terminal is being torn down (and a panic in a
// command races the program's exit), so its report is often lost.
func logPanic(where string) {
	if r := recover(); r != nil {
		log.Printf("panic in %s: %v\n%s", where, r, debug.Stack())
		panic(r)
	}
}

// guardCmd logs panics of cmd and of the commands it batches.
func guardCmd(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		defer logPanic("command")
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			guarded := make(tea.BatchMsg, 0, len(batch))
			for _, c := range batch {
				if c != nil {
					guarded = append(guarded, guardCmd(c))
				}
			}
			return guarded
		}
		return msg
	}
}
