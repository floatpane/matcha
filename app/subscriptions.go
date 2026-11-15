package app

import (
	"maps"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/daemonclient"
)

// folderSubscriptions keeps the daemon's new-mail subscriptions in line with
// the folder on screen. The UI goroutine only records the wanted state; the
// subscribe/unsubscribe RPCs run in commands. Each command applies the latest
// wanted state under a lock, so commands that run late or out of order can
// never leave a stale subscription behind, and the UI never waits on the
// daemon.
type folderSubscriptions struct {
	wantMu sync.Mutex
	want   map[string]string // accountID -> folder

	applyMu sync.Mutex
	have    map[string]string
}

func newFolderSubscriptions() *folderSubscriptions {
	return &folderSubscriptions{want: map[string]string{}, have: map[string]string{}}
}

// set records the folder each account should be subscribed to ("" or a
// missing account means none).
func (s *folderSubscriptions) set(want map[string]string) {
	s.wantMu.Lock()
	s.want = maps.Clone(want)
	s.wantMu.Unlock()
}

// syncCmd applies the wanted subscriptions through svc.
func (s *folderSubscriptions) syncCmd(svc daemonclient.Service) tea.Cmd {
	if svc == nil {
		return nil
	}
	return func() tea.Msg {
		s.apply(svc)
		return nil
	}
}

func (s *folderSubscriptions) apply(svc daemonclient.Service) {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()

	s.wantMu.Lock()
	want := maps.Clone(s.want)
	s.wantMu.Unlock()

	for account, folder := range s.have {
		if want[account] == folder {
			continue
		}
		_ = svc.Unsubscribe(account, folder)
		delete(s.have, account)
	}
	for account, folder := range want {
		if folder == "" || s.have[account] == folder {
			continue
		}
		// Recorded even when the RPC fails: the daemon client keeps the
		// subscription and replays it once it reconnects, so it must be
		// unsubscribed when the user moves on.
		_ = svc.Subscribe(account, folder)
		s.have[account] = folder
	}
}
