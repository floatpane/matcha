package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch batch := msg.(type) {
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, m := range batch {
			if m != nil {
				msgs = append(msgs, m)
			}
		}
		return msgs
	default:
		if msg != nil {
			return []tea.Msg{msg}
		}
	}
	return nil
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
		inbox.list, _ = inbox.list.Update(tea.KeyMsg{Type: tea.KeyDown})

		// Simulate pressing Enter to view the selected email.
		_, cmd := inbox.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
		{ID: "account-1", Email: "test1@example.com", Name: "User 1"},
		{ID: "account-2", Email: "test2@example.com", Name: "User 2"},
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

	// Should have 2 tabs: ALL + 1 account
	if len(inbox.tabs) != 2 {
		t.Errorf("Expected 2 tabs, got %d", len(inbox.tabs))
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
	_, cmd := inbox.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
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
	_, cmd := inbox.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
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

	_, cmd := inbox.Update(tea.KeyMsg{Type: tea.KeyDown})
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
}
