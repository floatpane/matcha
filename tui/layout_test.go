package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/view"
)

// assertFits fails when a rendered view is wider or taller than the screen.
// Overflowing lines are clipped (or wrap) in a real terminal, which shifts
// everything below them and breaks mouse hit-testing.
func assertFits(t *testing.T, name, content string, width, height int) {
	t.Helper()
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		t.Errorf("%s: %d lines, want at most %d", name, len(lines), height)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("%s: line %d is %d cells wide, want at most %d: %q", name, i, w, width, ansi.Strip(line))
		}
	}
}

func layoutTestEmails(n int, account string) []fetcher.Email {
	emails := make([]fetcher.Email, n)
	for i := range emails {
		emails[i] = fetcher.Email{
			UID:       uint32(i + 1),
			From:      fmt.Sprintf("Sender %d <s%d@example.com>", i+1, i+1),
			To:        []string{"me@example.com"},
			Subject:   fmt.Sprintf("Subject number %d", i+1),
			Date:      time.Now().Add(-time.Duration(i) * time.Hour),
			AccountID: account,
			MessageID: fmt.Sprintf("<m%d-%s@example.com>", i, account),
		}
	}
	return emails
}

func newLayoutFolderInbox(t *testing.T, folders []string, accounts []config.Account, emails []fetcher.Email, width, height int) *FolderInbox {
	t.Helper()
	RebuildStyles()
	fi := NewFolderInbox(folders, accounts)
	fi.Update(tea.WindowSizeMsg{Width: width, Height: height})
	fi.SetEmails(emails, accounts)
	return fi
}

func openLayoutPreview(fi *FolderInbox, email fetcher.Email, body string) {
	fi.OpenSplitPreview(email.UID, email.AccountID, nil)
	fi.Update(PreviewBodyFetchedMsg{UID: email.UID, AccountID: email.AccountID, Body: body, BodyMIMEType: "text/plain"})
}

var layoutSizes = [][2]int{{200, 60}, {120, 30}, {80, 24}, {60, 20}, {40, 12}}

func TestFolderInboxFitsScreen(t *testing.T) {
	one := []config.Account{{ID: "a1", Email: "me@example.com", Name: "Me"}}
	two := []config.Account{
		{ID: "a1", Email: "me@example.com", Name: "Me"},
		{ID: "a2", Email: "someone.with.a.very.long.address@example.com"},
	}
	folders := []string{"INBOX", "Sent", "Drafts", "Trash", "Archive"}
	many := make([]string, 60)
	for i := range many {
		many[i] = fmt.Sprintf("Folder %02d", i)
	}

	for _, sz := range layoutSizes {
		w, h := sz[0], sz[1]
		name := func(s string) string { return fmt.Sprintf("%s %dx%d", s, w, h) }

		fi := newLayoutFolderInbox(t, folders, one, layoutTestEmails(40, "a1"), w, h)
		assertFits(t, name("single account"), fi.View().Content, w, h)

		fi = newLayoutFolderInbox(t, folders, two, append(layoutTestEmails(20, "a1"), layoutTestEmails(20, "a2")...), w, h)
		assertFits(t, name("two accounts"), fi.View().Content, w, h)

		fi = newLayoutFolderInbox(t, many, one, layoutTestEmails(5, "a1"), w, h)
		fi.activeFolderIdx = 50
		assertFits(t, name("many folders"), fi.View().Content, w, h)

		for _, orientation := range []string{config.SplitPaneHorizontal, config.SplitPaneVertical} {
			emails := layoutTestEmails(40, "a1")
			fi = newLayoutFolderInbox(t, folders, one, emails, w, h)
			fi.SetSplitOrientation(orientation)
			fi.Update(tea.WindowSizeMsg{Width: w, Height: h})
			fi.OpenSplitPreview(emails[0].UID, "a1", nil)
			assertFits(t, name("loading preview "+orientation), fi.View().Content, w, h)
			openLayoutPreview(fi, emails[0], strings.Repeat("A long line of body text that keeps going. ", 40))
			assertFits(t, name("split preview "+orientation), fi.View().Content, w, h)
		}
	}
}

func TestFolderInboxSidebarFollowsActiveFolder(t *testing.T) {
	folders := make([]string, 60)
	for i := range folders {
		folders[i] = fmt.Sprintf("Folder %02d", i)
	}
	one := []config.Account{{ID: "a1", Email: "me@example.com"}}
	fi := newLayoutFolderInbox(t, folders, one, nil, 100, 20)
	fi.activeFolderIdx = 50
	content := ansi.Strip(fi.View().Content)
	if !strings.Contains(content, "Folder 50") {
		t.Fatalf("active folder scrolled out of the sidebar:\n%s", content)
	}

	// Clicking a sidebar row selects the folder drawn on that row.
	rows := strings.Split(content, "\n")
	target := -1
	for i, row := range rows {
		if strings.Contains(row[:min(len(row), sidebarWidth*3)], "Folder 45") {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("Folder 45 not visible:\n%s", content)
	}
	fi.Update(tea.MouseClickMsg{X: 3, Y: target, Button: tea.MouseLeft})
	if got := fi.GetCurrentFolder(); got != "Folder 45" {
		t.Fatalf("click on row %d switched to %q, want Folder 45", target, got)
	}
}

func TestFolderInboxSidebarWheelWithoutFolders(t *testing.T) {
	fi := NewFolderInbox(nil, nil)
	fi.folders = nil
	fi.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	// Used to panic with an integer divide by zero.
	fi.Update(tea.MouseWheelMsg{X: 2, Y: 5, Button: tea.MouseWheelDown})
	fi.Update(tea.MouseWheelMsg{X: 2, Y: 5, Button: tea.MouseWheelUp})
}

// rowOf returns the screen row whose text contains needle, or -1.
func rowOf(content, needle string) int {
	for i, line := range strings.Split(ansi.Strip(content), "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func TestInboxClickSelectsRenderedRow(t *testing.T) {
	one := []config.Account{{ID: "a1", Email: "me@example.com", Name: "Me"}}
	two := []config.Account{
		{ID: "a1", Email: "me@example.com", Name: "Me"},
		{ID: "a2", Email: "you@example.com"},
	}
	cases := []struct {
		name     string
		accounts []config.Account
		emails   []fetcher.Email
		split    bool
	}{
		{"single account", one, layoutTestEmails(40, "a1"), false},
		{"two accounts", two, append(layoutTestEmails(20, "a1"), layoutTestEmails(20, "a2")...), false},
		{"split preview", one, layoutTestEmails(40, "a1"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fi := newLayoutFolderInbox(t, []string{"INBOX", "Archive"}, tc.accounts, tc.emails, 140, 30)
			if tc.split {
				openLayoutPreview(fi, tc.emails[0], "body")
			}
			content := fi.View().Content
			row := rowOf(content, "Sender 5 ·")
			if row < 0 {
				t.Fatalf("item not rendered:\n%s", ansi.Strip(content))
			}
			fi.Update(tea.MouseClickMsg{X: sidebarWidth + 6, Y: row, Button: tea.MouseLeft})
			selected, ok := fi.inbox.list.SelectedItem().(item)
			if !ok || selected.uid != 5 {
				t.Fatalf("click on row %d selected %q, want Subject number 5", row, selected.title)
			}

			// Clicks below the last item of the page (pagination, help) must
			// not jump to another item or page.
			before := fi.inbox.list.Index()
			fi.Update(tea.MouseClickMsg{X: sidebarWidth + 6, Y: 29, Button: tea.MouseLeft})
			if after := fi.inbox.list.Index(); after != before {
				t.Fatalf("click on the help row moved the selection from %d to %d", before, after)
			}
		})
	}
}

func TestInboxDoubleClickOpensEmail(t *testing.T) {
	one := []config.Account{{ID: "a1", Email: "me@example.com"}}
	emails := layoutTestEmails(10, "a1")
	fi := newLayoutFolderInbox(t, []string{"INBOX"}, one, emails, 120, 30)
	row := rowOf(fi.View().Content, "Subject number 3")
	click := tea.MouseClickMsg{X: sidebarWidth + 6, Y: row, Button: tea.MouseLeft}
	// Run every command, as Bubble Tea does: the first click's command
	// used to be a wrapped nil that panicked when run.
	if _, first := fi.Update(click); first != nil {
		first()
	}
	_, cmd := fi.Update(click)
	if cmd == nil {
		t.Fatal("double-click did not open the email")
	}
	msg := cmd()
	view, ok := msg.(ViewEmailMsg)
	if !ok || view.UID != 3 {
		t.Fatalf("double-click produced %#v, want ViewEmailMsg for UID 3", msg)
	}
}

// runAll runs cmd and any batches it returns, the way Bubble Tea would.
func runAll(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, runAll(t, c)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func TestFolderInboxMouseCmdsAreRunnable(t *testing.T) {
	one := []config.Account{{ID: "a1", Email: "me@example.com"}}
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprintf("split=%v", split), func(t *testing.T) {
			emails := layoutTestEmails(100, "a1")
			fi := newLayoutFolderInbox(t, []string{"INBOX", "Archive"}, one, emails, 140, 30)
			if split {
				openLayoutPreview(fi, emails[0], "body")
			}
			row := rowOf(fi.View().Content, "Sender 3 ·")
			if row < 0 {
				t.Fatalf("item not rendered:\n%s", ansi.Strip(fi.View().Content))
			}
			x := sidebarWidth + 6
			for _, msg := range []tea.Msg{
				tea.MouseClickMsg{X: x, Y: row, Button: tea.MouseLeft},      // first half of a double-click
				tea.MouseClickMsg{X: x, Y: 29, Button: tea.MouseLeft},       // help row
				tea.MouseClickMsg{X: x, Y: row, Button: tea.MouseRight},     // ignored button
				tea.MouseWheelMsg{X: x, Y: row, Button: tea.MouseWheelDown}, // scroll the list
				tea.MouseWheelMsg{X: x, Y: row, Button: tea.MouseWheelUp},
				tea.MouseWheelMsg{X: 2, Y: 3, Button: tea.MouseWheelDown}, // folder sidebar
				tea.MouseClickMsg{X: 135, Y: 10, Button: tea.MouseLeft},   // right edge (preview when split)
				tea.MouseWheelMsg{X: 135, Y: 10, Button: tea.MouseWheelDown},
			} {
				_, cmd := fi.Update(msg)
				runAll(t, cmd) // used to panic: wrapInboxCmd wrapped a nil Cmd
			}
		})
	}
}

func TestFolderInboxFetchMoreKeepsFolderOfTheScroll(t *testing.T) {
	one := []config.Account{{ID: "a1", Email: "me@example.com"}}
	fi := newLayoutFolderInbox(t, []string{"INBOX", "Archive"}, one, layoutTestEmails(3, "a1"), 120, 30)
	row := rowOf(fi.View().Content, "Subject number 1")
	var cmd tea.Cmd
	for range 5 {
		if _, c := fi.Update(tea.MouseWheelMsg{X: sidebarWidth + 6, Y: row, Button: tea.MouseWheelDown}); c != nil {
			cmd = c
		}
	}
	if cmd == nil {
		t.Fatal("scrolling to the end did not request more emails")
	}
	fi.currentFolder = "Archive" // the user switched folders before the command ran
	var found bool
	for _, msg := range runAll(t, cmd) {
		if more, ok := msg.(FetchFolderMoreEmailsMsg); ok {
			found = true
			if more.FolderName != "INBOX" {
				t.Fatalf("fetch-more for the INBOX scroll asked for folder %q", more.FolderName)
			}
		}
	}
	if !found {
		t.Fatal("the fetch-more request was not turned into a folder-aware request")
	}
}

func TestFolderInboxFetchMoreForEveryAccountKeepsFolder(t *testing.T) {
	two := []config.Account{
		{ID: "a1", Email: "me@example.com"},
		{ID: "a2", Email: "you@example.com"},
	}
	fi := newLayoutFolderInbox(t, []string{"INBOX", "Archive"}, two, append(layoutTestEmails(2, "a1"), layoutTestEmails(2, "a2")...), 120, 30)
	row := rowOf(fi.View().Content, "Subject number 1")
	var cmd tea.Cmd
	for range 8 {
		if _, c := fi.Update(tea.MouseWheelMsg{X: sidebarWidth + 6, Y: row, Button: tea.MouseWheelDown}); c != nil {
			cmd = c
		}
	}
	if cmd == nil {
		t.Fatal("scrolling to the end of the ALL tab did not request more emails")
	}
	fi.currentFolder = "Archive" // switched before the batch ran
	accounts := map[string]bool{}
	for _, msg := range runAll(t, cmd) {
		if more, ok := msg.(FetchFolderMoreEmailsMsg); ok {
			if more.FolderName != "INBOX" {
				t.Fatalf("fetch-more for %s asked for folder %q", more.AccountID, more.FolderName)
			}
			accounts[more.AccountID] = true
		}
		if _, ok := msg.(FetchMoreEmailsMsg); ok {
			t.Fatal("a fetch-more request in the batch lost its folder")
		}
	}
	if !accounts["a1"] || !accounts["a2"] {
		t.Fatalf("folder-aware fetch-more requests for %v, want both accounts", accounts)
	}
}

func TestInboxTabClickSwitchesAccount(t *testing.T) {
	two := []config.Account{
		{ID: "a1", Email: "me@example.com"},
		{ID: "a2", Email: "you@example.com"},
	}
	fi := newLayoutFolderInbox(t, []string{"INBOX"}, two, append(layoutTestEmails(3, "a1"), layoutTestEmails(3, "a2")...), 120, 30)
	line := strings.Split(ansi.Strip(fi.View().Content), "\n")[0]
	col := strings.Index(line, "you@example.com")
	if col < 0 {
		t.Fatalf("tab not rendered: %q", line)
	}
	fi.Update(tea.MouseClickMsg{X: ansi.StringWidth(line[:col]) + 2, Y: 0, Button: tea.MouseLeft})
	if got := fi.inbox.GetCurrentAccountID(); got != "a2" {
		t.Fatalf("tab click selected account %q, want a2", got)
	}
}

func TestInboxWheelFetchesMoreAtBottom(t *testing.T) {
	one := []config.Account{{ID: "a1", Email: "me@example.com"}}
	inbox := NewInbox(layoutTestEmails(3, "a1"), one)
	inbox.SetSize(100, 30)
	var cmd tea.Cmd
	for range 5 {
		_, cmd = inbox.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	}
	if cmd == nil {
		t.Fatal("scrolling to the end with the wheel did not request more emails")
	}
}

func layoutEmailWithExtras() fetcher.Email {
	return fetcher.Email{
		UID: 1, AccountID: "a1",
		From:         "A Very Long Sender Name <sender.with.a.long.address@example.com>",
		To:           []string{"me@example.com", "other@example.com"},
		Subject:      "This is a long subject line that will certainly not fit in a narrow terminal window",
		Body:         strings.Repeat("Line of body text that goes on.\n", 60),
		BodyMIMEType: "text/plain",
		Attachments: []fetcher.Attachment{
			{Filename: "quarterly-report-with-an-extremely-long-file-name-2026.pdf", PartID: "2"},
			{Filename: "image.png", PartID: "3"},
		},
	}
}

func TestEmailViewFitsScreen(t *testing.T) {
	RebuildStyles()
	for _, sz := range layoutSizes {
		w, h := sz[0], sz[1]
		ev := NewEmailView(layoutEmailWithExtras(), 0, w, h, MailboxInbox, true)
		assertFits(t, fmt.Sprintf("email view %dx%d", w, h), ev.View().Content, w, h)
		if got := len(strings.Split(ev.View().Content, "\n")); got != h {
			t.Errorf("email view %dx%d renders %d lines, want exactly %d", w, h, got, h)
		}

		ev.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		assertFits(t, fmt.Sprintf("email view focused attachments %dx%d", w, h), ev.View().Content, w, h)

		ev.Update(tea.WindowSizeMsg{Width: w / 2, Height: h / 2})
		assertFits(t, fmt.Sprintf("email view resized %dx%d", w/2, h/2), ev.View().Content, w/2, h/2)
	}
}

func TestEmailViewAttachmentClick(t *testing.T) {
	RebuildStyles()
	ev := NewEmailView(layoutEmailWithExtras(), 0, 100, 30, MailboxInbox, true)
	row := rowOf(ev.View().Content, "image.png")
	if row < 0 {
		t.Fatal("attachment not rendered")
	}
	ev.Update(tea.MouseClickMsg{X: 5, Y: row, Button: tea.MouseLeft})
	if !ev.focusOnAttachments || ev.attachmentCursor != 1 {
		t.Fatalf("click on image.png: focus=%v cursor=%d, want focus on attachment 1", ev.focusOnAttachments, ev.attachmentCursor)
	}
	ev.Update(tea.MouseClickMsg{X: 5, Y: 6, Button: tea.MouseLeft})
	if ev.focusOnAttachments {
		t.Fatal("clicking the body should return focus to the body")
	}
}

func TestReplySplitViewFitsScreen(t *testing.T) {
	RebuildStyles()
	email := layoutEmailWithExtras()
	for _, orientation := range []string{config.SplitPaneHorizontal, config.SplitPaneVertical} {
		for _, sz := range [][2]int{{160, 50}, {120, 40}, {80, 24}, {60, 20}} {
			w, h := sz[0], sz[1]
			composer := NewComposer("me@example.com", "you@example.com", "Re: hi", "", true)
			rs := NewReplySplitView(email, composer, orientation, true, w, h)
			rs.Update(tea.WindowSizeMsg{Width: w, Height: h})
			assertFits(t, fmt.Sprintf("reply split %s %dx%d", orientation, w, h), rs.View().Content, w, h)
		}
	}
}

func TestReplySplitViewPrintableKeysStayInComposer(t *testing.T) {
	RebuildStyles()
	email := layoutEmailWithExtras()
	composer := NewComposer("me@example.com", "you@example.com", "Re: hi", "", true)
	rs := NewReplySplitView(email, composer, config.SplitPaneHorizontal, true, 120, 40)
	rs.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// "[" used to move focus to the email, after which "d" deleted it.
	for _, r := range "see [d" {
		_, cmd := rs.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		if cmd != nil {
			if _, deleted := cmd().(DeleteEmailMsg); deleted {
				t.Fatal("typing in the reply deleted the original email")
			}
		}
	}
	if !rs.focusComposer {
		t.Fatal("a printable key moved focus away from the composer")
	}

	rs.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
	if rs.focusComposer {
		t.Fatal("ctrl+] should focus the email pane")
	}
	for _, key := range []string{"d", "a", "r", "f"} {
		_, cmd := rs.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		if cmd != nil {
			switch cmd().(type) {
			case DeleteEmailMsg, ArchiveEmailMsg, ReplyToEmailMsg, ForwardEmailMsg:
				t.Fatalf("key %q triggered an action on the read-only email pane", key)
			}
		}
	}
	rs.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !rs.focusComposer {
		t.Fatal("esc should return focus to the composer")
	}
}

func TestEmailViewVisibleImagesClipToViewport(t *testing.T) {
	RebuildStyles()
	email := fetcher.Email{Body: strings.Repeat("line\n\n", 80), BodyMIMEType: "text/plain"}
	ev := NewEmailView(email, 0, 80, 20, MailboxInbox, false)
	ev.SetRowOffset(3)
	ev.SetColumnOffset(7)
	ev.imagePlacements = []view.ImagePlacement{
		{Line: 2, Rows: 6, Cols: 10, ID: 9, Protocol: view.ImageProtocolKitty, PixelW: 80, PixelH: 96},
		{Line: 30, Rows: 4, Cols: 10, ID: 10, Protocol: view.ImageProtocolSixel, PixelW: 80, PixelH: 64},
	}
	top := ev.bodyTop()

	images := ev.VisibleImages()
	if len(images) != 1 {
		t.Fatalf("got %d visible images, want only the kitty image in view", len(images))
	}
	img := images[0]
	if img.Row != 3+top+2 || img.Col != 7 || img.Rows != 6 || img.SrcY != 0 || img.SrcH != 96 {
		t.Fatalf("unclipped placement = %+v", img)
	}

	// Scroll so the first 4 rows of the image are above the viewport.
	ev.viewport.SetYOffset(6)
	images = ev.VisibleImages()
	if len(images) != 1 {
		t.Fatalf("got %d images after scrolling, want 1", len(images))
	}
	img = images[0]
	if img.Row != 3+top || img.Rows != 2 || img.SrcY != 64 || img.SrcH != 32 {
		t.Fatalf("clipped placement = %+v, want the bottom 2 rows (y=64,h=32) at the viewport top", img)
	}

	// The sixel image is only shown when it is fully visible: at offset 31
	// its first row (line 30) is scrolled out.
	ev.viewport.SetYOffset(31)
	for _, img := range ev.VisibleImages() {
		if img.Protocol == view.ImageProtocolSixel {
			t.Fatalf("partially visible sixel image was placed: %+v", img)
		}
	}
	ev.viewport.SetYOffset(30)
	found := false
	for _, img := range ev.VisibleImages() {
		if img.Protocol == view.ImageProtocolSixel {
			found = true
		}
	}
	if !found {
		t.Fatal("fully visible sixel image was not placed")
	}

	// Hiding images removes every placement.
	ev.showImages = false
	if len(ev.VisibleImages()) != 0 {
		t.Fatal("images still placed after toggling them off")
	}
}

func TestGraphicsPainterKitty(t *testing.T) {
	g := NewGraphicsPainter()
	img := ScreenImage{ID: 5, Placement: 1, Protocol: view.ImageProtocolKitty, Row: 4, Col: 2, Cols: 10, Rows: 3, SrcW: 80, SrcH: 48, Data: "<upload>"}

	raw, clearScreen, paint, changed := g.plan([]ScreenImage{img})
	if !changed || clearScreen || !paint {
		t.Fatalf("plan = changed:%v clear:%v paint:%v", changed, clearScreen, paint)
	}
	if raw != "<upload>" {
		t.Fatalf("first sync should only upload the image, got %q", raw)
	}
	paintCmd := g.Paint(GraphicsPaintMsg{gen: g.gen})
	if paintCmd == nil {
		t.Fatal("paint produced nothing")
	}
	seq := fmt.Sprint(paintCmd().(tea.RawMsg).Msg)
	for _, want := range []string{"\x1b7", "\x1b[5;3H", "a=p,i=5,p=1", "C=1", "c=10,r=3", "\x1b8"} {
		if !strings.Contains(seq, want) {
			t.Errorf("placement %q missing %q", seq, want)
		}
	}
	if strings.Contains(seq, "\n") {
		t.Errorf("placement must not contain newlines (they scroll the screen): %q", seq)
	}

	// Same layout again: nothing to do, and the image is not re-uploaded.
	if _, _, _, changed := g.plan([]ScreenImage{img}); changed {
		t.Fatal("unchanged layout should be a no-op")
	}

	// Scrolled: the old placement is removed immediately, the new one is
	// painted after the frame.
	moved := img
	moved.Row = 2
	raw, _, paint, _ = g.plan([]ScreenImage{moved})
	if !strings.Contains(raw, "a=d,d=i,i=5,p=1") || strings.Contains(raw, "<upload>") || !paint {
		t.Fatalf("scroll plan = %q paint=%v; want a placement delete and no re-upload", raw, paint)
	}
	if g.Paint(GraphicsPaintMsg{gen: g.gen - 1}) != nil {
		t.Fatal("stale paint request should be ignored")
	}

	// Leaving the view removes the placement.
	raw, _, paint, _ = g.plan(nil)
	if !strings.Contains(raw, "a=d,d=i,i=5,p=1") && g.HasPainted() {
		t.Fatalf("clearing plan = %q", raw)
	}
	if paint {
		t.Fatal("nothing to paint after leaving the view")
	}
}

func TestGraphicsPainterSixelRedraws(t *testing.T) {
	g := NewGraphicsPainter()
	img := ScreenImage{ID: 7, Placement: 1, Protocol: view.ImageProtocolSixel, Row: 1, Col: 0, Cols: 4, Rows: 2, Data: "\x1bPq#0~\x1b\\"}
	g.plan([]ScreenImage{img})
	paintCmd := g.Paint(GraphicsPaintMsg{gen: g.gen})
	if paintCmd == nil || !strings.Contains(fmt.Sprint(paintCmd().(tea.RawMsg).Msg), "\x1bPq") {
		t.Fatal("sixel image was not painted")
	}
	_, clearScreen, _, _ := g.plan(nil)
	if !clearScreen {
		t.Fatal("removing a sixel image must redraw the screen to erase its pixels")
	}
}

func TestFilterImagesOutside(t *testing.T) {
	images := []ScreenImage{
		{ID: 1, Row: 0, Col: 70, Rows: 3, Cols: 10},
		{ID: 2, Row: 10, Col: 0, Rows: 3, Cols: 10},
	}
	kept := FilterImagesOutside(images, 0, 60, 5, 40)
	if len(kept) != 1 || kept[0].ID != 2 {
		t.Fatalf("kept %+v, want only the image outside the notification", kept)
	}
}

func TestFolderInboxMoveOverlayMouse(t *testing.T) {
	folders := []string{"INBOX", "Archive", "Projects", "Receipts", "Travel"}
	one := []config.Account{{ID: "a1", Email: "me@example.com"}}
	emails := layoutTestEmails(5, "a1")
	fi := newLayoutFolderInbox(t, folders, one, emails, 100, 30)
	fi.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !fi.movingEmail {
		t.Fatal("move overlay did not open")
	}
	content := fi.View().Content
	assertFits(t, "move overlay", content, 100, 30)

	// The overlay lists the other folders; click "Receipts" (the sidebar
	// lists it too, so look right of the sidebar).
	row, col := -1, -1
	for i, line := range strings.Split(ansi.Strip(content), "\n") {
		if idx := strings.LastIndex(line, "Receipts"); idx >= 0 {
			if c := ansi.StringWidth(line[:idx]); c > sidebarWidth {
				row, col = i, c
				break
			}
		}
	}
	if row < 0 {
		t.Fatalf("Receipts not in the overlay:\n%s", ansi.Strip(content))
	}
	fi.Update(tea.MouseClickMsg{X: col, Y: row, Button: tea.MouseLeft})
	choices := fi.moveFolderChoices()
	if choices[fi.moveTargetIdx] != "Receipts" {
		t.Fatalf("click selected %q, want Receipts", choices[fi.moveTargetIdx])
	}
	fi.View()
	// Clicking the selected folder moves the email there.
	_, cmd := fi.Update(tea.MouseClickMsg{X: col, Y: row, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("clicking the selected folder did not move the email")
	}
	if move, ok := cmd().(MoveEmailToFolderMsg); !ok || move.DestFolder != "Receipts" {
		t.Fatalf("got %#v, want a move to Receipts", cmd())
	}

	// Many folders: the overlay scrolls instead of growing past the screen.
	many := make([]string, 80)
	for i := range many {
		many[i] = fmt.Sprintf("Folder %02d", i)
	}
	fi = newLayoutFolderInbox(t, many, one, emails, 80, 20)
	fi.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	for range 50 {
		fi.Update(tea.MouseWheelMsg{X: 40, Y: 10, Button: tea.MouseWheelDown})
	}
	content = fi.View().Content
	assertFits(t, "move overlay with many folders", content, 80, 20)
	if want := fi.moveFolderChoices()[fi.moveTargetIdx]; rowOf(content, want) < 0 {
		t.Fatalf("selected folder %q scrolled out of the overlay", want)
	}
	// Clicking outside the box closes it.
	fi.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if fi.movingEmail {
		t.Fatal("clicking outside the overlay did not close it")
	}
}

func TestTruncateEmailKeepsCharactersWhole(t *testing.T) {
	got := truncateEmail("пользователь.почты@пример.рф")
	if !utf8.ValidString(got) {
		t.Fatalf("truncateEmail produced invalid UTF-8: %q", got)
	}
	if got != "пользова...@пример.рф" {
		t.Fatalf("truncateEmail = %q", got)
	}
}

func TestGraphicsPainterPlaceholderImagesAreUploadedOnly(t *testing.T) {
	g := NewGraphicsPainter()
	img := ScreenImage{ID: 0xF80000FF, Placement: 1, Protocol: view.ImageProtocolKittyPlaceholder, Row: 4, Col: 2, Cols: 10, Rows: 3, Data: "<upload A>"}
	raw, clearScreen, paint, changed := g.plan([]ScreenImage{img})
	if !changed || clearScreen || paint || raw != "<upload A>" {
		t.Fatalf("plan = %q clear:%v paint:%v changed:%v; want only the upload (the text draws the image)", raw, clearScreen, paint, changed)
	}
	scrolled := img
	scrolled.Row = 2
	if raw, _, paint, _ = g.plan([]ScreenImage{scrolled}); raw != "" || paint {
		t.Fatalf("scrolling re-sent %q (paint=%v); placeholder cells move with the text", raw, paint)
	}
	// The id now names another image: upload it again.
	other := scrolled
	other.Data = "<upload B>"
	if raw, _, _, _ = g.plan([]ScreenImage{other}); raw != "<upload B>" {
		t.Fatalf("a reused id was not uploaded again: %q", raw)
	}
	if g.HasPainted() {
		t.Fatal("placeholder images are never placed by the painter")
	}

	// Quitting frees the image through tmux passthrough, by id.
	msgs := runAll(t, g.Shutdown())
	var out string
	for _, msg := range msgs {
		if raw, ok := msg.(tea.RawMsg); ok {
			out += fmt.Sprint(raw.Msg)
		}
	}
	if want := view.KittyPlaceholderFreeSequence(img.ID); !strings.Contains(out, want) || strings.Contains(out, "d=a") {
		t.Fatalf("shutdown wrote %q; want only %q", out, want)
	}
}

func TestEmailViewListsPlaceholderImagesForUpload(t *testing.T) {
	ev := NewEmailView(layoutEmailWithExtras(), 0, 100, 30, MailboxInbox, false)
	ev.showImages = true
	ev.imagePlacements = []view.ImagePlacement{
		{Line: 1, Rows: 4, Cols: 10, ID: 0xF8000080, Protocol: view.ImageProtocolKittyPlaceholder, Encoded: "<upload>"},
	}
	images := ev.VisibleImages()
	if len(images) != 1 || images[0].Protocol != view.ImageProtocolKittyPlaceholder || images[0].Data != "<upload>" {
		t.Fatalf("visible images = %+v; a placeholder image on screen must be uploaded", images)
	}
	ev.viewport.SetYOffset(20) // scrolled past it
	if len(ev.VisibleImages()) != 0 {
		t.Fatal("an image scrolled out of view is still listed")
	}
}

func TestGraphicsPainterResendsPlaceholdersOnFocus(t *testing.T) {
	g := NewGraphicsPainter()
	img := ScreenImage{ID: 0x130000ff, Placement: 1, Protocol: view.ImageProtocolKittyPlaceholder, Row: 4, Col: 2, Cols: 10, Rows: 3, Data: "<upload A>"}
	other := ScreenImage{ID: 0x130000fe, Placement: 2, Protocol: view.ImageProtocolKittyPlaceholder, Row: 9, Cols: 5, Rows: 2, Data: "<upload B>"}
	g.plan([]ScreenImage{img, other})
	g.plan([]ScreenImage{img}) // the other image scrolled out of view

	g.ResendPlaceholders() // a terminal attached
	if raw, _, _, _ := g.plan([]ScreenImage{img}); raw != "<upload A>" {
		t.Fatalf("after focus-in: plan = %q, want the visible image uploaded again", raw)
	}
	if raw, _, _, _ := g.plan([]ScreenImage{img, other}); raw != "<upload B>" {
		t.Fatalf("an image coming into view after focus-in: plan = %q, want it uploaded again", raw)
	}
	if raw, _, _, _ := g.plan([]ScreenImage{img}); raw != "" {
		t.Fatalf("uploaded twice: %q", raw)
	}
}

func TestGraphicsPainterHoldsUploadsWhileHidden(t *testing.T) {
	g := NewGraphicsPainter()
	shown := false
	g.placeholdersReach = func() bool { return shown }
	img := ScreenImage{ID: 0x130000ff, Placement: 1, Protocol: view.ImageProtocolKittyPlaceholder, Row: 4, Col: 2, Cols: 10, Rows: 3, Data: "<upload>"}
	var retry bool
	for _, msg := range runAll(t, g.Sync([]ScreenImage{img})) {
		if raw, ok := msg.(tea.RawMsg); ok && strings.Contains(fmt.Sprint(raw.Msg), "<upload>") {
			t.Fatal("uploaded while tmux would drop it")
		}
		if _, ok := msg.(GraphicsRetryMsg); ok {
			retry = true
		}
	}
	if !retry {
		t.Fatal("no retry was scheduled for the held-back upload")
	}
	g.Retry() // still hidden
	if raw, _, _, _ := g.plan([]ScreenImage{img}); raw != "" || !g.held {
		t.Fatalf("hidden retry = %q held=%v", raw, g.held)
	}
	shown = true
	g.Retry()
	if raw, _, _, _ := g.plan([]ScreenImage{img}); raw != "<upload>" || g.held {
		t.Fatalf("visible retry = %q held=%v", raw, g.held)
	}
}

func TestEmailViewRedrawsWhenTheImageProtocolChanges(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	email := fetcher.Email{Body: `<p>hi</p><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg==">`, BodyMIMEType: "text/html"}
	ev := NewEmailView(email, 0, 80, 24, MailboxInbox, false)
	if !ev.showImages || len(ev.imagePlacements) != 1 {
		t.Fatalf("setup: showImages=%v placements=%d", ev.showImages, len(ev.imagePlacements))
	}
	ev.Update(ImageProtocolChangedMsg{}) // nothing changed
	if len(ev.imagePlacements) != 1 {
		t.Fatal("redrew although the protocol is the same")
	}
	t.Setenv("TERM", "xterm-256color") // a terminal without images attached
	ev.Update(ImageProtocolChangedMsg{})
	if len(ev.imagePlacements) != 0 || ev.renderedProtocol != view.ImageProtocolNone {
		t.Fatalf("after the change: %d placements drawn with %v", len(ev.imagePlacements), ev.renderedProtocol)
	}
}
