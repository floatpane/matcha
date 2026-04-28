package main

import (
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/tui"
)

type wrapper struct {
	inbox *tui.Inbox
}

func (w wrapper) Init() tea.Cmd {
	return w.inbox.Init()
}

func (w wrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := w.inbox.Update(msg)
	if inbox, ok := m.(*tui.Inbox); ok {
		w.inbox = inbox
	}
	return w, cmd
}

func (w wrapper) View() tea.View {
	v := w.inbox.View()
	v.AltScreen = true
	return v
}

func main() {
	now := time.Now()
	account := config.Account{
		ID:         "demo-user",
		Name:       "Matcha Demo",
		Email:      "demo@floatpane.com",
		FetchEmail: "demo@floatpane.com",
	}

	emails := []fetcher.Email{
		{
			UID:        304,
			From:       "Priya Shah <priya@example.com>",
			To:         []string{"demo@floatpane.com"},
			Subject:    "Re: Release checklist for 1.8",
			Date:       now.Add(-8 * time.Minute),
			MessageID:  "<release-304@example.com>",
			References: []string{"<release-301@example.com>", "<release-302@example.com>"},
			AccountID:  account.ID,
		},
		{
			UID:       303,
			From:      "Buildkite <buildkite@example.com>",
			To:        []string{"demo@floatpane.com"},
			Subject:   "main passed",
			Date:      now.Add(-20 * time.Minute),
			MessageID: "<build-303@example.com>",
			AccountID: account.ID,
			IsRead:    true,
		},
		{
			UID:        302,
			From:       "Noah Reed <noah@example.com>",
			To:         []string{"demo@floatpane.com"},
			Subject:    "Re: Release checklist for 1.8",
			Date:       now.Add(-33 * time.Minute),
			MessageID:  "<release-302@example.com>",
			References: []string{"<release-301@example.com>"},
			AccountID:  account.ID,
			IsRead:     true,
		},
		{
			UID:       301,
			From:      "Avery Stone <avery@example.com>",
			To:        []string{"demo@floatpane.com"},
			Subject:   "Release checklist for 1.8",
			Date:      now.Add(-52 * time.Minute),
			MessageID: "<release-301@example.com>",
			AccountID: account.ID,
			IsRead:    true,
		},
		{
			UID:       300,
			From:      "Finance <finance@example.com>",
			To:        []string{"demo@floatpane.com"},
			Subject:   "Invoice approvals",
			Date:      now.Add(-2 * time.Hour),
			MessageID: "<invoice-300@example.com>",
			AccountID: account.ID,
			IsRead:    true,
		},
	}

	inbox := tui.NewInbox(emails, []config.Account{account})
	inbox.SetFolderName("INBOX")

	p := tea.NewProgram(wrapper{inbox: inbox})
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
