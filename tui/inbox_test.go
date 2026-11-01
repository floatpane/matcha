package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}

	// Try type assertion to see if it's a BatchMsg
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, m := range batch {
			msgs = append(msgs, collectMsgs(m)...)
		}
		return msgs
	}

	// Otherwise it's a regular message
	return []tea.Msg{msg}
}

// TestInboxUpdate verifies the state transitions in the inbox view.
func TestInboxUpdate(t *testing.T) {
	// Create sample accounts
	accounts := []config.Account{
		{ID: "account-1", Email: "test1@example.com", Name: "Test User 1"},
		{ID: "account-2", Email: "test2@example.com", Name: "Test User 2"},
	}

	// Create a sample list of emails.
	sampleEmails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Email 1", Date: time.Now(), AccountID: "account-1"},
		{UID: 2, From: "b@example.com", Subject: "Email 2", Date: time.Now().Add(-time.Hour), AccountID: "account-1"},
		{UID: 3, From: "c@example.com", Subject: "Email 3", Date: time.Now().Add(-2 * time.Hour), AccountID: "account-2"},
	}

	inbox := NewInbox(sampleEmails, accounts)

	t.Run("Select email to view", func(t *testing.T) {
		// By default, the first item is selected (index 0).
		// Move down to the second item (index 1).
		inbox.list, _ = inbox.list.Update(tea.KeyPressMsg{Code: tea.KeyDown})

		// Simulate pressing Enter to view the selected email.
		_, cmd := inbox.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Expected a command, but got nil.")
		}

		// Check the resulting message.
		msg := cmd()
		viewMsg, ok := msg.(ViewEmailMsg)
		if !ok {
			t.Fatalf("Expected a ViewEmailMsg, but got %T", msg)
		}

		// The index should match the selected item in the list.
		expectedIndex := 1
		if viewMsg.Index != expectedIndex {
			t.Errorf("Expected index %d, but got %d", expectedIndex, viewMsg.Index)
		}

		// Verify UID and AccountID are passed correctly
		expectedUID := uint32(2) // Second email has UID 2
		if viewMsg.UID != expectedUID {
			t.Errorf("Expected UID %d, but got %d", expectedUID, viewMsg.UID)
		}

		expectedAccountID := "account-1" // Second email belongs to account-1
		if viewMsg.AccountID != expectedAccountID {
			t.Errorf("Expected AccountID %q, but got %q", expectedAccountID, viewMsg.AccountID)
		}
	})
}

// TestInboxMultiAccountTabs verifies that tabs are created for multiple accounts.
func TestInboxMultiAccountTabs(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "mail.example.com", FetchEmail: "test1@example.com", Name: "User 1"},
		{ID: "account-2", Email: "mail.example.com", FetchEmail: "test2@example.com", Name: "User 2"},
	}

	emails := []fetcher.Email{
		{UID: 1, From: "sender@example.com", Subject: "Test", AccountID: "account-1"},
	}

	inbox := NewInbox(emails, accounts)

	// Should have 3 tabs: ALL + 2 accounts
	if len(inbox.tabs) != 3 {
		t.Errorf("Expected 3 tabs, got %d", len(inbox.tabs))
	}

	// First tab should be "ALL"
	if inbox.tabs[0].ID != "" {
		t.Errorf("Expected first tab ID to be empty (ALL), got %q", inbox.tabs[0].ID)
	}
	if inbox.tabs[0].Label != "ALL" {
		t.Errorf("Expected first tab label to be 'ALL', got %q", inbox.tabs[0].Label)
	}
	if inbox.tabs[1].Label != "test1@example.com" {
		t.Errorf("Expected first account tab to use FetchEmail, got %q", inbox.tabs[1].Label)
	}

	inbox.SetEmails(emails, accounts)
	if inbox.tabs[1].Label != "test1@example.com" || inbox.tabs[1].Email != "test1@example.com" {
		t.Errorf("Expected SetEmails to preserve FetchEmail tab display, got label=%q email=%q", inbox.tabs[1].Label, inbox.tabs[1].Email)
	}
}

func TestInboxSearchResultsFilterByActiveAccountTab(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "mail.example.com", FetchEmail: "first@example.com"},
		{ID: "account-2", Email: "mail.example.com", FetchEmail: "second@example.com"},
	}

	inbox := NewInbox(nil, accounts)
	query := backend.ParseSearchQuery("quarterly")
	results := []fetcher.Email{
		{UID: 1, From: "a@example.com", To: []string{"first@example.com"}, Subject: "First", AccountID: "account-1"},
		{UID: 2, From: "b@example.com", To: []string{"second@example.com"}, Subject: "Second", AccountID: "account-2"},
	}

	model, _ := inbox.Update(ApplySearchResultsMsg{Query: query, Emails: results})
	inbox = model.(*Inbox)
	if got := len(inbox.list.Items()); got != 2 {
		t.Fatalf("expected all search results initially, got %d", got)
	}

	model, _ = inbox.Update(tea.KeyPressMsg{Code: tea.KeyRight, Text: "right"})
	inbox = model.(*Inbox)
	if got := len(inbox.list.Items()); got != 1 {
		t.Fatalf("expected account-filtered search results after tab switch, got %d", got)
	}
	item, ok := inbox.list.Items()[0].(item)
	if !ok {
		t.Fatalf("expected inbox item, got %T", inbox.list.Items()[0])
	}
	if item.accountID != "account-1" {
		t.Fatalf("expected account-1 result after first account tab, got %q", item.accountID)
	}

	email := inbox.GetEmailAtIndex(0)
	if email == nil || email.UID != 1 {
		t.Fatalf("GetEmailAtIndex should use filtered search results, got %#v", email)
	}
}

func TestInboxAllAccountsDedupesSharedMailboxByMessageID(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "mail.example.com", FetchEmail: "edu@andrinoff.com"},
		{ID: "account-2", Email: "mail.example.com", FetchEmail: "me@andrinoff.com"},
		{ID: "account-3", Email: "mail.example.com", FetchEmail: "business@andrinoff.com"},
	}
	emails := []fetcher.Email{
		{UID: 81, MessageID: "<shared@example.com>", From: "drew@example.com", To: []string{"business@andrinoff.com"}, Subject: "Hey", AccountID: "account-1"},
		{UID: 82, MessageID: "<shared@example.com>", From: "drew@example.com", To: []string{"business@andrinoff.com"}, Subject: "Hey", AccountID: "account-2"},
		{UID: 83, MessageID: "<shared@example.com>", From: "drew@example.com", To: []string{"business@andrinoff.com"}, Subject: "Hey", AccountID: "account-3"},
	}

	inbox := NewInbox(emails, accounts)
	if got := len(inbox.allEmails); got != 1 {
		t.Fatalf("expected all accounts view to dedupe shared mailbox copies, got %d", got)
	}
	if got := len(inbox.emailsByAccount["account-1"]); got != 1 {
		t.Fatalf("expected per-account bucket to remain unchanged, got %d", got)
	}
	row := inbox.list.Items()[0].(item)
	if row.accountEmail != "business@andrinoff.com" {
		t.Fatalf("expected deduped row label to match recipient account, got %q", row.accountEmail)
	}
	if row.accountID != "account-3" {
		t.Fatalf("expected canonical row to use matching account copy, got %q", row.accountID)
	}
}

func TestInboxSearchResultsDedupedAcrossAccounts(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "mail.example.com", FetchEmail: "edu@andrinoff.com"},
		{ID: "account-2", Email: "mail.example.com", FetchEmail: "business@andrinoff.com"},
	}
	inbox := NewInbox(nil, accounts)
	query := backend.ParseSearchQuery("osc8")
	results := []fetcher.Email{
		{UID: 81, MessageID: "<shared@example.com>", From: "drew@example.com", To: []string{"business@andrinoff.com"}, Subject: "Hey", AccountID: "account-1"},
		{UID: 82, MessageID: "<shared@example.com>", From: "drew@example.com", To: []string{"business@andrinoff.com"}, Subject: "Hey", AccountID: "account-2"},
	}

	model, _ := inbox.Update(ApplySearchResultsMsg{Query: query, Emails: results})
	inbox = model.(*Inbox)
	if got := len(inbox.searchResults); got != 1 {
		t.Fatalf("expected search results to dedupe shared mailbox copies, got %d", got)
	}
	row := inbox.list.Items()[0].(item)
	if row.accountEmail != "business@andrinoff.com" {
		t.Fatalf("expected search result label to match recipient account, got %q", row.accountEmail)
	}
}

func TestInboxAllAccountsDoesNotDedupeWhenMessageIDDiffers(t *testing.T) {
	date := time.Now()
	accounts := []config.Account{
		{ID: "account-1", Email: "mail.example.com", FetchEmail: "first@example.com"},
		{ID: "account-2", Email: "mail.example.com", FetchEmail: "second@example.com"},
	}
	emails := []fetcher.Email{
		{UID: 1, MessageID: "<one@example.com>", From: "sender@example.com", To: []string{"first@example.com"}, Subject: "Same", Date: date, AccountID: "account-1"},
		{UID: 2, MessageID: "<two@example.com>", From: "sender@example.com", To: []string{"second@example.com"}, Subject: "Same", Date: date, AccountID: "account-2"},
	}

	inbox := NewInbox(emails, accounts)
	if got := len(inbox.allEmails); got != 2 {
		t.Fatalf("expected distinct Message-ID emails to remain visible, got %d", got)
	}
}

func TestInboxAccountLabelUsesMatchingRecipient(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "mail.example.com", FetchEmail: "first@example.com"},
		{ID: "account-2", Email: "mail.example.com", FetchEmail: "second@example.com"},
	}
	emails := []fetcher.Email{
		{UID: 1, MessageID: "<first@example.com>", From: "a@example.com", To: []string{"Shared <shared@example.com>", "Second <second@example.com>"}, Subject: "First", AccountID: "account-1"},
		{UID: 2, From: "b@example.com", To: []string{"shared@example.com"}, Subject: "Fallback", AccountID: "account-2"},
	}

	inbox := NewInbox(emails, accounts)
	first := inbox.list.Items()[0].(item)
	if first.accountEmail != "second@example.com" {
		t.Fatalf("expected cross-account matching To recipient for account label, got %q", first.accountEmail)
	}
	second := inbox.list.Items()[1].(item)
	if second.accountEmail != "second@example.com" {
		t.Fatalf("expected FetchEmail fallback for unmatched recipient, got %q", second.accountEmail)
	}
}

func TestInboxOpenSearchResultEmbedsEmailInViewMsg(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "mail.example.com", FetchEmail: "first@example.com"},
	}
	inbox := NewInbox(nil, accounts)
	searchResult := fetcher.Email{UID: 42, MessageID: "<search@example.com>", From: "sender@example.com", To: []string{"first@example.com"}, Subject: "Search", AccountID: "account-1"}
	model, _ := inbox.Update(ApplySearchResultsMsg{Query: backend.ParseSearchQuery("search"), Emails: []fetcher.Email{searchResult}})
	inbox = model.(*Inbox)

	_, cmd := inbox.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected open command")
	}
	msg := cmd()
	viewMsg, ok := msg.(ViewEmailMsg)
	if !ok {
		t.Fatalf("expected ViewEmailMsg, got %T", msg)
	}
	if viewMsg.Email == nil {
		t.Fatal("expected search result email to be embedded")
	}
	if viewMsg.Email.UID != searchResult.UID || viewMsg.Email.MessageID != searchResult.MessageID {
		t.Fatalf("embedded email mismatch: %#v", viewMsg.Email)
	}
}

func TestInboxClientSideFilterKeyStartsListFilter(t *testing.T) {
	accounts := []config.Account{{ID: "account-1", Email: "test@example.com"}}
	emails := []fetcher.Email{{UID: 1, From: "sender@example.com", Subject: "Test", AccountID: "account-1"}}

	inbox := NewInbox(emails, accounts)
	model, _ := inbox.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	inbox = model.(*Inbox)

	if inbox.list.FilterState() != list.Filtering {
		t.Fatalf("expected client-side filter state %s, got %s", list.Filtering, inbox.list.FilterState())
	}
}

// TestInboxSingleAccount verifies behavior with a single account.
func TestInboxSingleAccount(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	emails := []fetcher.Email{
		{UID: 1, From: "sender@example.com", Subject: "Test", AccountID: "account-1"},
	}

	inbox := NewInbox(emails, accounts)

	// Should have 0 tabs (visually)
	if len(inbox.tabs) != 1 {
		t.Errorf("Expected 1 phantom tab, got %d", len(inbox.tabs))
	}
}

// TestInboxNoAccounts verifies behavior with no accounts (legacy/edge case).
func TestInboxNoAccounts(t *testing.T) {
	emails := []fetcher.Email{
		{UID: 1, From: "sender@example.com", Subject: "Test"},
	}

	inbox := NewInbox(emails, nil)

	// Should have 1 tab: ALL only
	if len(inbox.tabs) != 1 {
		t.Errorf("Expected 1 tab, got %d", len(inbox.tabs))
	}
}

// TestInboxDeleteEmailMsg verifies that delete messages include account ID.
func TestInboxDeleteEmailMsg(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	emails := []fetcher.Email{
		{UID: 123, From: "sender@example.com", Subject: "Test", AccountID: "account-1"},
	}

	inbox := NewInbox(emails, accounts)

	// Simulate pressing 'd' to delete
	_, cmd := inbox.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if cmd == nil {
		t.Fatal("Expected a command, but got nil.")
	}

	msg := cmd()
	deleteMsg, ok := msg.(DeleteEmailMsg)
	if !ok {
		t.Fatalf("Expected a DeleteEmailMsg, but got %T", msg)
	}

	if deleteMsg.UID != 123 {
		t.Errorf("Expected UID 123, got %d", deleteMsg.UID)
	}

	if deleteMsg.AccountID != "account-1" {
		t.Errorf("Expected AccountID 'account-1', got %q", deleteMsg.AccountID)
	}
}

// TestInboxArchiveEmailMsg verifies that archive messages include account ID.
func TestInboxArchiveEmailMsg(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	emails := []fetcher.Email{
		{UID: 456, From: "sender@example.com", Subject: "Test", AccountID: "account-1"},
	}

	inbox := NewInbox(emails, accounts)

	// Simulate pressing 'a' to archive
	_, cmd := inbox.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if cmd == nil {
		t.Fatal("Expected a command, but got nil.")
	}

	msg := cmd()
	archiveMsg, ok := msg.(ArchiveEmailMsg)
	if !ok {
		t.Fatalf("Expected an ArchiveEmailMsg, but got %T", msg)
	}

	if archiveMsg.UID != 456 {
		t.Errorf("Expected UID 456, got %d", archiveMsg.UID)
	}

	if archiveMsg.AccountID != "account-1" {
		t.Errorf("Expected AccountID 'account-1', got %q", archiveMsg.AccountID)
	}
}

// TestInboxRemoveEmail verifies that emails can be removed from the inbox.
func TestInboxRemoveEmail(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	emails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Email 1", AccountID: "account-1"},
		{UID: 2, From: "b@example.com", Subject: "Email 2", AccountID: "account-1"},
	}

	inbox := NewInbox(emails, accounts)

	// Remove the first email
	inbox.RemoveEmail(1, "account-1")

	// Check that only one email remains
	if len(inbox.allEmails) != 1 {
		t.Errorf("Expected 1 email after removal, got %d", len(inbox.allEmails))
	}

	if inbox.allEmails[0].UID != 2 {
		t.Errorf("Expected remaining email UID to be 2, got %d", inbox.allEmails[0].UID)
	}
}

// TestInboxGetEmailAtIndex verifies retrieving emails by index.
func TestInboxGetEmailAtIndex(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	emails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Email 1", AccountID: "account-1"},
		{UID: 2, From: "b@example.com", Subject: "Email 2", AccountID: "account-1"},
	}

	inbox := NewInbox(emails, accounts)

	// Get email at index 0
	email := inbox.GetEmailAtIndex(0)
	if email == nil {
		t.Fatal("Expected email at index 0, got nil")
	}
	if email.UID != 1 {
		t.Errorf("Expected UID 1 at index 0, got %d", email.UID)
	}

	// Get email at invalid index
	email = inbox.GetEmailAtIndex(999)
	if email != nil {
		t.Error("Expected nil for invalid index, got non-nil")
	}

	// Get email at negative index
	email = inbox.GetEmailAtIndex(-1)
	if email != nil {
		t.Error("Expected nil for negative index, got non-nil")
	}
}

func TestFetchMoreTriggeredAtListEnd(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	emails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Email 1", AccountID: "account-1", Date: time.Now()},
		{UID: 2, From: "b@example.com", Subject: "Email 2", AccountID: "account-1", Date: time.Now().Add(-time.Minute)},
	}

	inbox := NewInbox(emails, accounts)

	_, cmd := inbox.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	msgs := collectMsgs(cmd)

	var fetchMsg FetchMoreEmailsMsg
	for _, m := range msgs {
		if msg, ok := m.(FetchMoreEmailsMsg); ok {
			fetchMsg = msg
			break
		}
	}

	if fetchMsg.AccountID == "" {
		t.Fatal("expected a FetchMoreEmailsMsg when reaching end of the list")
	}

	if fetchMsg.Offset != uint32(len(emails)) {
		t.Fatalf("expected offset %d, got %d", len(emails), fetchMsg.Offset)
	}
	if fetchMsg.AccountID != "account-1" {
		t.Fatalf("expected account ID 'account-1', got %q", fetchMsg.AccountID)
	}
	if fetchMsg.Mailbox != MailboxInbox {
		t.Fatalf("expected MailboxInbox, got %s", fetchMsg.Mailbox)
	}

	// Default list height is 14, but our minimum limit is 20
	expectedLimit := uint32(20)
	if fetchMsg.Limit != expectedLimit {
		t.Fatalf("expected Limit %d, got %d", expectedLimit, fetchMsg.Limit)
	}
}

func TestTruncateEmailKeepsDomain(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{
			name:  "long local part keeps full domain",
			email: "verylongemail@gmail.com",
			want:  "verylong...@gmail.com",
		},
		{
			name:  "short email unchanged",
			email: "abc@gmail.com",
			want:  "abc@gmail.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateEmail(tt.email)
			if got != tt.want {
				t.Fatalf("truncateEmail(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

// TestThreadedExpandShowsHeaderAndClickableEmails verifies that when a thread
// is expanded, the list contains a thread header row (for closing) followed by
// all individual emails as clickable items.
func TestThreadedExpandShowsHeaderAndClickableEmails(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test1@example.com", Name: "Test User 1"},
	}

	// Three emails in a thread: root, reply, reply-to-reply
	base := time.Now()
	emails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Thread topic", Date: base.Add(-2 * time.Hour), AccountID: "account-1", MessageID: "<m1@example.com>"},
		{UID: 2, From: "b@example.com", Subject: "Re: Thread topic", Date: base.Add(-1 * time.Hour), AccountID: "account-1", MessageID: "<m2@example.com>", InReplyTo: "<m1@example.com>"},
		{UID: 3, From: "c@example.com", Subject: "Re: Thread topic", Date: base, AccountID: "account-1", MessageID: "<m3@example.com>", InReplyTo: "<m2@example.com>"},
	}

	inbox := NewInbox(emails, accounts)
	inbox.SetDefaultThreaded(true)
	inbox.updateList()

	// Collapsed: should have 1 item (the thread root)
	items := inbox.list.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 collapsed thread item, got %d", len(items))
	}
	rootItem := items[0].(item)
	if !rootItem.threadRoot || rootItem.threadCount != 3 {
		t.Fatalf("expected thread root with count 3, got root=%v count=%d", rootItem.threadRoot, rootItem.threadCount)
	}

	// Expand the thread
	inbox.expanded[rootItem.threadKey] = true
	inbox.updateList()

	// Expanded: should have 4 items: header + 3 clickable emails
	items = inbox.list.Items()
	if len(items) != 4 {
		t.Fatalf("expected 4 items (header + 3 emails) when expanded, got %d", len(items))
	}

	// First item should be the thread header (not clickable)
	header := items[0].(item)
	if !header.threadHeader {
		t.Fatalf("expected first expanded item to be thread header, got threadHeader=%v", header.threadHeader)
	}
	if header.uid != 0 {
		t.Fatalf("thread header should have uid 0, got %d", header.uid)
	}

	// Remaining 3 items should be clickable emails with non-zero UIDs
	for i := 1; i < 4; i++ {
		emailItem := items[i].(item)
		if emailItem.uid == 0 {
			t.Fatalf("expanded email item %d should have non-zero uid", i)
		}
		if emailItem.threadHeader {
			t.Fatalf("expanded email item %d should not be a thread header", i)
		}
	}
}

// TestThreadedStatusBarCountsActualEmails verifies that totalEmailCount tracks
// the actual number of emails, not the number of thread items.
func TestThreadedStatusBarCountsActualEmails(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test1@example.com", Name: "Test User 1"},
	}

	base := time.Now()
	emails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Thread topic", Date: base.Add(-2 * time.Hour), AccountID: "account-1", MessageID: "<m1@example.com>"},
		{UID: 2, From: "b@example.com", Subject: "Re: Thread topic", Date: base.Add(-1 * time.Hour), AccountID: "account-1", MessageID: "<m2@example.com>", InReplyTo: "<m1@example.com>"},
		{UID: 3, From: "c@example.com", Subject: "Re: Thread topic", Date: base, AccountID: "account-1", MessageID: "<m3@example.com>", InReplyTo: "<m2@example.com>"},
		{UID: 4, From: "d@example.com", Subject: "Standalone email", Date: base, AccountID: "account-1", MessageID: "<m4@example.com>"},
	}

	inbox := NewInbox(emails, accounts)
	inbox.SetDefaultThreaded(true)
	inbox.updateList()

	// 4 actual emails, but only 2 list items (1 thread + 1 standalone)
	if inbox.totalEmailCount != 4 {
		t.Fatalf("expected totalEmailCount=4, got %d", inbox.totalEmailCount)
	}
	if len(inbox.list.Items()) != 2 {
		t.Fatalf("expected 2 list items (thread + standalone), got %d", len(inbox.list.Items()))
	}

	// Expand the thread — still 4 emails, but now 5 list items (header + 3 + 1)
	inbox.expanded["<m1@example.com>"] = true
	inbox.updateList()
	if inbox.totalEmailCount != 4 {
		t.Fatalf("expected totalEmailCount=4 after expand, got %d", inbox.totalEmailCount)
	}
}

// TestItemDelegateKeepsSubjectWithinRowWidth renders the inbox delegate directly
// and checks that a row never draws past the list width.
func TestItemDelegateKeepsSubjectWithinRowWidth(t *testing.T) {
	RebuildStyles()

	when := time.Date(2020, time.January, 2, 15, 4, 5, 0, time.Local)
	const layout = "2006-01-02"
	dateStr := formatInboxDate(when, layout, true)
	inbox := &Inbox{
		dateFormat:    layout,
		detailedDates: true,
		selectedUIDs:  map[uint32]string{},
	}

	render := func(t *testing.T, width, index int, it item, selected, visual bool) string {
		t.Helper()
		inbox.visualMode = visual
		if visual {
			inbox.selectedUIDs = map[uint32]string{it.uid: "acct"}
		} else {
			inbox.selectedUIDs = map[uint32]string{}
		}
		l := list.New([]list.Item{it}, itemDelegate{inbox: inbox}, max(width, 1), 10)
		l.SetWidth(width)
		if selected {
			l.Select(index)
		} else if l.Index() == index {
			l.Select(index + 1)
		}
		var buf bytes.Buffer
		itemDelegate{inbox: inbox}.Render(&buf, l, index, it)
		return buf.String()
	}

	assertBounded := func(t *testing.T, width int, rendered string) {
		t.Helper()
		if width <= 0 {
			if rendered != "" {
				t.Fatalf("width %d: want no visible content, got %q", width, rendered)
			}
			return
		}
		if !utf8.ValidString(rendered) {
			t.Fatalf("width %d: invalid UTF-8 %q", width, rendered)
		}
		if strings.ContainsAny(rendered, "\r\n\t") {
			t.Fatalf("width %d: embedded newline or tab in %q", width, rendered)
		}
		if got := lipgloss.Width(rendered); got > width {
			t.Fatalf("width %d: display width %d exceeds list width: %q", width, got, ansi.Strip(rendered))
		}
	}

	shortSubject := "Ship"
	longSubject := strings.Repeat("A", 180)
	base := func(title, from string) item {
		return item{
			title:  title,
			desc:   from,
			uid:    7,
			date:   when,
			isRead: true,
		}
	}

	t.Run("short and long subjects align dates", func(t *testing.T) {
		for _, width := range []int{40, 80, 120} {
			short := render(t, width, 0, base(shortSubject, "Ada <ada@example.com>"), false, false)
			long := render(t, width, 0, base(longSubject, "Ada <ada@example.com>"), false, false)
			assertBounded(t, width, short)
			assertBounded(t, width, long)

			shortPlain := ansi.Strip(short)
			longPlain := ansi.Strip(long)
			if !strings.Contains(shortPlain, shortSubject) || strings.Contains(shortPlain, "…") {
				t.Fatalf("width %d: short subject rendered %q", width, shortPlain)
			}
			if strings.Contains(longPlain, longSubject) || !strings.Contains(longPlain, "…") {
				t.Fatalf("width %d: long subject rendered %q", width, longPlain)
			}
			if !strings.HasSuffix(shortPlain, dateStr) || !strings.HasSuffix(longPlain, dateStr) {
				t.Fatalf("width %d: date %q not flush right in %q / %q", width, dateStr, shortPlain, longPlain)
			}
			shortEnd := lipgloss.Width(short)
			longEnd := lipgloss.Width(long)
			if shortEnd != width || longEnd != width {
				t.Fatalf("width %d: date end columns %d and %d, want both flush at %d (%q / %q)", width, shortEnd, longEnd, width, shortPlain, longPlain)
			}
			if !strings.Contains(shortPlain, "·") {
				t.Fatalf("width %d: missing separator in %q", width, shortPlain)
			}
		}
	})

	t.Run("narrow widths and exact fit", func(t *testing.T) {
		it := base("SubjectAlpha", "Pat <pat@example.com>")
		icon := "\uf2b6"
		prefix := "1. "
		separator := " · "
		exact := itemStyle.GetHorizontalFrameSize() +
			ansi.StringWidth(prefix) + ansi.StringWidth(icon) + 1 +
			ansi.StringWidth("Pat") + ansi.StringWidth(separator) +
			ansi.StringWidth("SubjectAlpha") + 2 + ansi.StringWidth(dateStr)

		for _, width := range []int{0, 1, 2, 4, 8, 16, 24, exact, exact - 1} {
			got := render(t, width, 0, it, false, false)
			assertBounded(t, width, got)
		}

		fit := ansi.Strip(render(t, exact, 0, it, false, false))
		if !strings.Contains(fit, "SubjectAlpha") || !strings.HasSuffix(fit, dateStr) || strings.Contains(fit, "…") {
			t.Fatalf("exact width %d rendered %q", exact, fit)
		}
		over := ansi.Strip(render(t, exact-1, 0, it, false, false))
		if strings.Contains(over, "SubjectAlpha") && strings.HasSuffix(over, dateStr) && !strings.Contains(over, "…") {
			t.Fatalf("one cell over width %d still shows a full row %q", exact-1, over)
		}
	})

	t.Run("date wider than the row", func(t *testing.T) {
		inbox.dateFormat = "Monday, 02 January 2006 15:04:05 -0700"
		t.Cleanup(func() { inbox.dateFormat = layout })
		it := base("Hi", "Ada <ada@example.com>")
		fullDate := formatInboxDate(it.date, inbox.dateFormat, true)
		if ansi.StringWidth(fullDate) <= 24 {
			t.Fatalf("fixture date %q is not wider than the row", fullDate)
		}
		for _, width := range []int{0, 1, 2, 4, 8, 16, 24} {
			got := render(t, width, 0, it, false, false)
			assertBounded(t, width, got)
			if strings.Contains(ansi.Strip(got), fullDate) {
				t.Fatalf("width %d kept wide date %q in %q", width, fullDate, ansi.Strip(got))
			}
		}
	})

	t.Run("selection markers stay inside the budget", func(t *testing.T) {
		it := base("SubjectAlpha", "Pat <pat@example.com>")
		icon := "\uf2b6"
		exact := itemStyle.GetHorizontalFrameSize() +
			ansi.StringWidth("1. ") + ansi.StringWidth(icon) + 1 +
			ansi.StringWidth("Pat") + ansi.StringWidth(" · ") +
			ansi.StringWidth("SubjectAlpha") + 2 + ansi.StringWidth(dateStr)

		modes := []struct {
			name     string
			selected bool
			visual   bool
			marker   string
		}{
			{name: "unselected", marker: ""},
			{name: "selected", selected: true, marker: "> "},
			{name: "visual", visual: true, marker: "* "},
			{name: "cursor and visual", selected: true, visual: true, marker: ">*"},
		}
		for _, mode := range modes {
			for _, width := range []int{0, 1, 2, 4, 8, 16, 24, 40, exact} {
				got := render(t, width, 0, it, mode.selected, mode.visual)
				assertBounded(t, width, got)
			}
			plain := ansi.Strip(render(t, exact, 0, it, mode.selected, mode.visual))
			frame := strings.Repeat(" ", itemStyle.GetHorizontalFrameSize())
			if !strings.HasPrefix(plain, frame+mode.marker) {
				t.Fatalf("%s: marker %q not reserved in %q", mode.name, mode.marker, plain)
			}
			if mode.marker != "" && strings.Contains(plain, "SubjectAlpha") && !strings.Contains(plain, "…") {
				t.Fatalf("%s: marker did not consume subject budget in %q", mode.name, plain)
			}
		}
	})

	t.Run("unicode thread account and long sender", func(t *testing.T) {
		items := []item{
			base(strings.Repeat("件名", 40), "Ada <ada@example.com>"),
			base(strings.Repeat("e\u0301", 40), "Ada <ada@example.com>"),
			base(strings.Repeat("👨‍👩‍👧‍👦", 16)+" party", "Ada <ada@example.com>"),
			base(strings.Repeat("A", 400), "Ada <ada@example.com>"),
			base("Hello", strings.Repeat("B", 200)+" <long@example.com>"),
			{
				title: "All accounts", desc: "Ada <ada@example.com>", uid: 7, date: when, isRead: true,
				accountEmail: "verylongemail@gmail.com",
			},
			base("Standalone", "Ada <ada@example.com>"),
			{
				title: "Topic", desc: "Ada <ada@example.com>", uid: 7, date: when, isRead: true,
				threadRoot: true, threadCount: 3,
			},
			{
				title: "Topic", desc: "Ada <ada@example.com>", uid: 8, date: when, isRead: true,
				threadRoot: true, threadHeader: true, threadCount: 3, expanded: true,
			},
			{
				title: "DeepChild", desc: "Ada <ada@example.com>", uid: 9, date: when, isRead: true,
				threadChild: true, threadDepth: 12,
			},
		}
		for _, it := range items {
			for _, width := range []int{0, 1, 2, 4, 8, 16, 24, 40, 80, 120} {
				assertBounded(t, width, render(t, width, 0, it, false, false))
			}
		}

		cjk := ansi.Strip(render(t, 40, 0, items[0], false, false))
		if !strings.Contains(cjk, "…") {
			t.Fatalf("cjk subject was not ellipsized: %q", cjk)
		}
		combining := ansi.Strip(render(t, 40, 0, items[1], false, false))
		if !strings.Contains(combining, "…") {
			t.Fatalf("combining subject was not ellipsized: %q", combining)
		}
		emoji := ansi.Strip(render(t, 40, 0, items[2], false, false))
		if !strings.Contains(emoji, "…") {
			t.Fatalf("emoji subject was not ellipsized: %q", emoji)
		}
		unbroken := ansi.Strip(render(t, 40, 0, items[3], false, false))
		if strings.Contains(unbroken, items[3].title) || !strings.Contains(unbroken, "…") {
			t.Fatalf("unbroken subject rendered %q", unbroken)
		}
		sender := ansi.Strip(render(t, 80, 0, items[4], false, false))
		if strings.Contains(sender, strings.Repeat("B", 200)) || !strings.Contains(sender, "…") {
			t.Fatalf("long sender rendered %q", sender)
		}

		account := ansi.Strip(render(t, 120, 0, items[5], false, false))
		if !strings.Contains(account, truncateEmail("verylongemail@gmail.com")) {
			t.Fatalf("account indicator rendered %q", account)
		}
		numbered := ansi.Strip(render(t, 80, 99, base("Tenth", "Ada <ada@example.com>"), false, false))
		if !strings.Contains(numbered, "100.") {
			t.Fatalf("multi-digit row rendered %q", numbered)
		}
		root := ansi.Strip(render(t, 120, 0, items[7], false, false))
		if !strings.Contains(root, "Topic (3)") {
			t.Fatalf("thread root rendered %q", root)
		}
		header := ansi.Strip(render(t, 120, 0, items[8], false, false))
		if !strings.Contains(header, "Close thread (3)") {
			t.Fatalf("thread header rendered %q", header)
		}
		child := ansi.Strip(render(t, 120, 0, items[9], false, false))
		if !strings.Contains(child, "↳") || !strings.Contains(child, "DeepChild") {
			t.Fatalf("indented child rendered %q", child)
		}
	})

	t.Run("empty and embedded whitespace leave the item unchanged", func(t *testing.T) {
		empty := item{title: "", desc: "Ada <ada@example.com>", uid: 4, isRead: true}
		beforeTitle, beforeFilter := empty.title, empty.FilterValue()
		got := render(t, 80, 0, empty, false, false)
		assertBounded(t, 80, got)
		if empty.title != beforeTitle || empty.FilterValue() != beforeFilter {
			t.Fatalf("empty render mutated item title %q filter %q", empty.title, empty.FilterValue())
		}

		spaced := item{
			title:  "Hello\r\nWorld\t!",
			desc:   "Ann\nLee\t<ann@example.com>",
			uid:    5,
			date:   when,
			isRead: false,
		}
		title, filter := spaced.title, spaced.FilterValue()
		rendered := render(t, 120, 0, spaced, false, false)
		assertBounded(t, 120, rendered)
		plain := ansi.Strip(rendered)
		if !strings.Contains(plain, "Ann Lee · Hello World !") {
			t.Fatalf("whitespace flattened to %q", plain)
		}
		if spaced.title != title || spaced.FilterValue() != filter || !strings.Contains(filter, "\n") {
			t.Fatalf("whitespace render mutated title %q filter %q", spaced.title, spaced.FilterValue())
		}

		read := render(t, 80, 0, base("Colors", "Ada <ada@example.com>"), false, false)
		unreadItem := base("Colors", "Ada <ada@example.com>")
		unreadItem.isRead = false
		unread := render(t, 80, 0, unreadItem, false, false)
		assertBounded(t, 80, read)
		assertBounded(t, 80, unread)
		if read == unread || !strings.Contains(read, "\x1b") || !strings.Contains(unread, "\x1b") {
			t.Fatalf("read/unread styles were not applied:\nread %q\nunread %q", read, unread)
		}
	})
}
