package idledaemon

import (
	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/daemonrpc"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/tui"
)

// ListenForIdleUpdates blocks until an IDLE update arrives, then returns it as a tea.Msg.
func ListenForIdleUpdates(ch <-chan fetcher.IdleUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return nil
		}
		return tui.IdleNewMailMsg{
			AccountID:  update.AccountID,
			FolderName: update.FolderName,
		}
	}
}

// ListenForDaemonEvents blocks until a daemon event arrives, then returns it as a tea.Msg.
func ListenForDaemonEvents(ch <-chan *daemonrpc.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return tui.DaemonEventMsg{Event: ev}
	}
}
