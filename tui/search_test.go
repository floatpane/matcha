package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/fetcher"
)

func searchType(o *SearchOverlay, text string) {
	for _, r := range text {
		o.Update(tea.KeyPressMsg{Code: r, Text: string(r)}, MailboxInbox, "acct")
	}
}

func searchEnter(o *SearchOverlay) tea.Msg {
	cmd := o.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, MailboxInbox, "acct")
	if cmd == nil {
		return nil
	}
	return cmd()
}

// TestSearchOverlayEditingQueryDiscardsOldResults: once results arrived,
// changing the query and pressing enter must run a new search instead of
// applying the results of the old query.
func TestSearchOverlayEditingQueryDiscardsOldResults(t *testing.T) {
	o := NewSearchOverlay(80, 24)
	searchType(o, "alice")
	req, ok := searchEnter(o).(SearchRequestedMsg)
	if !ok {
		t.Fatal("enter should request a search")
	}
	o.Update(SearchResultsMsg{Query: req.Query, Emails: []fetcher.Email{{Subject: "old"}}}, MailboxInbox, "acct")

	searchType(o, "x")
	got := searchEnter(o)
	if _, applied := got.(ApplySearchResultsMsg); applied {
		t.Fatal("enter after editing the query applied the old results")
	}
	next, ok := got.(SearchRequestedMsg)
	if !ok || next.Query.Raw != "alicex" {
		t.Fatalf("enter after editing should search for the new query, got %#v", got)
	}
}

// TestSearchOverlayDropsStaleResults ignores results of a search whose query
// was edited while it ran.
func TestSearchOverlayDropsStaleResults(t *testing.T) {
	o := NewSearchOverlay(80, 24)
	searchType(o, "a")
	req := searchEnter(o).(SearchRequestedMsg)
	searchType(o, "b")
	o.Update(SearchResultsMsg{Query: req.Query, Emails: []fetcher.Email{{Subject: "stale"}}}, MailboxInbox, "acct")
	if o.done {
		t.Fatal("results for an edited query must be dropped")
	}
}

// TestSearchOverlayFitsArea keeps the box inside the given width and height,
// with every result on a single truncated line.
func TestSearchOverlayFitsArea(t *testing.T) {
	const width, height = 50, 14
	o := NewSearchOverlay(width, height)
	searchType(o, "invoice")
	req := searchEnter(o).(SearchRequestedMsg)
	emails := make([]fetcher.Email, 15)
	for i := range emails {
		emails[i] = fetcher.Email{From: "Sender <s@example.com>", Subject: strings.Repeat("a very long subject ", 5)}
	}
	o.Update(SearchResultsMsg{Query: req.Query, Emails: emails}, MailboxInbox, "acct")

	lines := strings.Split(ansi.Strip(o.View()), "\n")
	if len(lines) > height {
		t.Fatalf("box is %d rows tall, area is %d", len(lines), height)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > width {
			t.Fatalf("row %d is %d wide, area is %d", i, w, width)
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), "1. Sender") {
		t.Fatal("results should be listed")
	}

	// Resizing narrows the box with the area.
	o.Update(tea.WindowSizeMsg{Width: 40, Height: 10}, MailboxInbox, "acct")
	for i, line := range strings.Split(ansi.Strip(o.View()), "\n") {
		if w := ansi.StringWidth(line); w > 40 {
			t.Fatalf("after resize row %d is %d wide", i, w)
		}
	}
	if h := len(strings.Split(o.View(), "\n")); h > 10 {
		t.Fatalf("after resize the box is %d rows tall", h)
	}
}
