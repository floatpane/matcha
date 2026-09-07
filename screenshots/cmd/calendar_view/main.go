// calendar_view renders a sample invite using the real email view for screenshots.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/tui"
)

// snapshot keeps the demo at a fixed size so the invite and body fit the tape.
// EmailView's resize handler currently does not reserve height for invite cards.
type snapshot struct {
	*tui.EmailView
}

func (s snapshot) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		return s, nil
	}
	if _, ok := msg.(tea.KeyPressMsg); ok {
		return s, tea.Quit
	}
	_, cmd := s.EmailView.Update(msg)
	return s, cmd
}

func (s snapshot) View() tea.View {
	v := s.EmailView.View()
	v.AltScreen = true
	return v
}

func main() {
	ics := strings.ReplaceAll(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Matcha//Screenshot Demo//EN
METHOD:REQUEST
BEGIN:VEVENT
UID:weekly-standup@example.com
DTSTAMP:20260417T090000Z
DTSTART:20260420T100000Z
DTEND:20260420T103000Z
SUMMARY:Weekly Standup
LOCATION:Conference Room B
ORGANIZER:mailto:alice@example.com
ATTENDEE:mailto:team@example.com
DESCRIPTION:Share progress and plan the week ahead.
END:VEVENT
END:VCALENDAR
`, "\n", "\r\n")
	email := fetcher.Email{
		UID:       1002,
		From:      "Alice Morgan <alice@example.com>",
		To:        []string{"team@example.com"},
		Subject:   "Invitation: Weekly Standup",
		Date:      time.Date(2026, time.April, 17, 9, 0, 0, 0, time.UTC),
		MessageID: "<weekly-standup@example.com>",
		AccountID: "demo-user",
		Body:      "Hi team,\n\nJoin us on Monday to share progress and plan the week ahead.\n\nSee you there!\nAlice",
		Attachments: []fetcher.Attachment{{
			Filename:         "invite.ics",
			MIMEType:         "text/calendar",
			IsCalendarInvite: true,
			Data:             []byte(ics),
		}},
	}

	p := tea.NewProgram(snapshot{tui.NewEmailView(email, 0, 140, 32, tui.MailboxInbox, true)})
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
