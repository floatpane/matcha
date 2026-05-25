// inbox_view is a small helper that renders a mock inbox with realistic emails
// for screenshot generation. It wraps the real Inbox component in a model
// that forwards window size events properly.
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

const (
	demoUserID    = "demo-user"
	demoUserEmail = "matcha@floatpane.com"
)

// wrapper forwards all messages to the FolderInbox and ensures it renders correctly.
type wrapper struct {
	folderInbox *tui.FolderInbox
}

func (w wrapper) Init() tea.Cmd {
	return w.folderInbox.Init()
}

func (w wrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := w.folderInbox.Update(msg)
	if fi, ok := m.(*tui.FolderInbox); ok {
		w.folderInbox = fi
	}
	return w, cmd
}

func (w wrapper) View() tea.View {
	v := w.folderInbox.View()
	v.AltScreen = true
	return v
}

func main() {
	now := time.Now()

	accounts := []config.Account{
		{
			ID:         demoUserID,
			Name:       "Matcha Client",
			Email:      demoUserEmail,
			FetchEmail: demoUserEmail,
		},
	}

	emails := []fetcher.Email{
		{
			UID:       1012,
			From:      "Alice Park <alice.park@example.com>",
			To:        []string{demoUserEmail},
			Subject:   "Quick sync on the API migration?",
			Date:      now.Add(-12 * time.Minute),
			MessageID: "<api-migration-012@example.com>",
			AccountID: demoUserID,
		},
		{
			UID:       1011,
			From:      "GitHub <notifications@github.com>",
			To:        []string{demoUserEmail},
			Subject:   "[floatpane/matcha] Fix: resolve inbox pagination issue (#281)",
			Date:      now.Add(-47 * time.Minute),
			MessageID: "<gh-notif-281@github.com>",
			AccountID: demoUserID,
		},
		{
			UID:       1010,
			From:      "Sarah Chen <sarah.chen@example.com>",
			To:        []string{"team@example.com"},
			Subject:   "New Dashboard Redesign - Preview & Feedback",
			Date:      now.Add(-2 * time.Hour),
			MessageID: "<dashboard-redesign-001@example.com>",
			AccountID: demoUserID,
			Attachments: []fetcher.Attachment{
				{Filename: "dashboard-mockup.png", MIMEType: "image/png"},
			},
		},
		{
			UID:       1009,
			From:      "David Kim <david.kim@example.com>",
			To:        []string{demoUserEmail},
			Subject:   "Re: Quarterly budget review notes",
			Date:      now.Add(-5 * time.Hour),
			MessageID: "<budget-review-009@example.com>",
			AccountID: demoUserID,
		},
		{
			UID:       1008,
			From:      "Stripe <receipts@stripe.com>",
			To:        []string{demoUserEmail},
			Subject:   "Your receipt from Acme Corp - Invoice #4821",
			Date:      now.Add(-23 * time.Hour),
			MessageID: "<stripe-receipt-4821@stripe.com>",
			AccountID: demoUserID,
		},
		{
			UID:       1007,
			From:      "Maria Gonzalez <maria.g@example.com>",
			To:        []string{demoUserEmail},
			Subject:   "Design system tokens - final version attached",
			Date:      now.Add(-1*24*time.Hour - 6*time.Hour),
			MessageID: "<design-tokens-007@example.com>",
			AccountID: demoUserID,
			Attachments: []fetcher.Attachment{
				{Filename: "design-tokens-v3.json", MIMEType: "application/json"},
			},
		},
		{
			UID:       1006,
			From:      "Linear <notifications@linear.app>",
			To:        []string{demoUserEmail},
			Subject:   "MAT-342: Implement keyboard shortcuts for compose view",
			Date:      now.Add(-2*24*time.Hour - 3*time.Hour),
			MessageID: "<linear-342@linear.app>",
			AccountID: demoUserID,
		},
		{
			UID:       1005,
			From:      "James Wright <j.wright@example.com>",
			To:        []string{demoUserEmail},
			Subject:   "Onboarding docs are ready for review",
			Date:      now.Add(-3*24*time.Hour - 1*time.Hour),
			MessageID: "<onboarding-005@example.com>",
			AccountID: demoUserID,
		},
		{
			UID:       1004,
			From:      "Vercel <notifications@vercel.com>",
			To:        []string{demoUserEmail},
			Subject:   "Deployment successful: matcha-docs-8f3a2b1",
			Date:      now.Add(-4*24*time.Hour - 8*time.Hour),
			MessageID: "<vercel-deploy-004@vercel.com>",
			AccountID: demoUserID,
		},
		{
			UID:       1003,
			From:      "Lena Muller <lena.m@example.com>",
			To:        []string{demoUserEmail},
			Subject:   "Conference talk proposal - Rethinking TUI Design",
			Date:      now.Add(-5*24*time.Hour - 2*time.Hour),
			MessageID: "<conference-003@example.com>",
			AccountID: demoUserID,
		},
		{
			UID:       1002,
			From:      "GitHub <notifications@github.com>",
			To:        []string{demoUserEmail},
			Subject:   "[floatpane/matcha] Release v1.4.0 published",
			Date:      now.Add(-5*24*time.Hour - 14*time.Hour),
			MessageID: "<gh-release-140@github.com>",
			AccountID: demoUserID,
		},
		{
			UID:       1001,
			From:      "Omar Hassan <omar.h@example.com>",
			To:        []string{demoUserEmail},
			Subject:   "Re: Open source contribution guidelines",
			Date:      now.Add(-6*24*time.Hour - 5*time.Hour),
			MessageID: "<oss-contrib-001@example.com>",
			AccountID: demoUserID,
		},
	}

	folders := []string{
		"INBOX",
		"Drafts",
		"Sent",
		"Archive",
		"Receipts",
		"GitHub",
		"Trash",
	}

	folderInbox := tui.NewFolderInbox(folders, accounts)
	folderInbox.SetEmails(emails, accounts)

	p := tea.NewProgram(wrapper{folderInbox: folderInbox})
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
