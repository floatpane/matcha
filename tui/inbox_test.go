package tui

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
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
