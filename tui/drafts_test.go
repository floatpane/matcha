package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/config"
)

func newTestDrafts(n int) *Drafts {
	drafts := make([]config.Draft, n)
	for i := range drafts {
		drafts[i] = config.Draft{ID: fmt.Sprint(i), Subject: fmt.Sprintf("Subject %02d", i), To: "x@example.com"}
	}
	d := NewDrafts(drafts)
	d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return d
}

// TestDraftsClickMapsToVisibleItems clicks the rendered rows: an item row
// selects that draft, while rows below the page's items (pagination, help)
// must neither select anything nor flip pages.
func TestDraftsClickMapsToVisibleItems(t *testing.T) {
	d := newTestDrafts(12)
	lines := strings.Split(ansi.Strip(d.View().Content), "\n")

	row := -1
	for i, line := range lines {
		if strings.Contains(line, "Subject 02") {
			row = i
		}
	}
	if row < 0 {
		t.Fatal("Subject 02 is not rendered")
	}
	d.Update(tea.MouseClickMsg{X: 4, Y: row, Button: tea.MouseLeft})
	if got := d.list.Index(); got != 2 {
		t.Fatalf("clicking Subject 02 selected index %d", got)
	}

	lastItemRow := 0
	for i, line := range lines {
		if strings.Contains(line, "Subject ") || strings.Contains(line, "To: ") {
			lastItemRow = i
		}
	}
	for y := lastItemRow + 1; y < len(lines); y++ {
		d.Update(tea.MouseClickMsg{X: 4, Y: y, Button: tea.MouseLeft})
		if d.list.Paginator.Page != 0 || d.list.Index() != 2 {
			t.Fatalf("click on row %d (below the items) moved to page %d index %d", y, d.list.Paginator.Page, d.list.Index())
		}
	}
}

// TestDraftsClickUsesFilteredItems maps clicks onto the filtered list.
func TestDraftsClickUsesFilteredItems(t *testing.T) {
	d := newTestDrafts(12)
	d.list.SetFilterText("Subject 1")
	lines := strings.Split(ansi.Strip(d.View().Content), "\n")
	for i, line := range lines {
		if strings.Contains(line, "Subject 11") {
			d.Update(tea.MouseClickMsg{X: 4, Y: i, Button: tea.MouseLeft})
			item, ok := d.list.SelectedItem().(draftItem)
			if !ok || item.draft.Subject != "Subject 11" {
				t.Fatalf("clicking Subject 11 selected %#v", d.list.SelectedItem())
			}
			return
		}
	}
	t.Fatal("Subject 11 is not rendered in the filtered list")
}

// TestDraftsEscClearsFilterFirst: with a filter applied, esc clears it and
// stays; only an unfiltered esc leaves Drafts.
func TestDraftsEscClearsFilterFirst(t *testing.T) {
	d := newTestDrafts(5)
	d.list.SetFilterText("Subject 01")
	if d.list.FilterState() != list.FilterApplied {
		t.Fatal("filter not applied")
	}

	_, cmd := d.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		if _, left := cmd().(GoToChoiceMenuMsg); left {
			t.Fatal("esc with a filter applied left Drafts")
		}
	}
	if d.list.FilterState() != list.Unfiltered {
		t.Fatal("esc should clear the applied filter")
	}

	_, cmd = d.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc without a filter should leave Drafts")
	}
	if _, ok := cmd().(GoToChoiceMenuMsg); !ok {
		t.Fatal("esc without a filter should go back to the main menu")
	}
}
