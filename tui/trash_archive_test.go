package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

// TestNewTrashArchive verifies that a new TrashArchive is created correctly.
func TestNewTrashArchive(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com", FetchEmail: "fetch@example.com"},
	}

	trashEmails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Trash Email 1", AccountID: "account-1"},
	}

	archiveEmails := []fetcher.Email{
		{UID: 2, From: "b@example.com", Subject: "Archive Email 1", AccountID: "account-1"},
	}

	ta := NewTrashArchive(trashEmails, archiveEmails, accounts)

	// Default view should be Trash
	if ta.activeView != MailboxTrash {
		t.Errorf("Expected default view to be MailboxTrash, got %v", ta.activeView)
	}

	// GetActiveMailbox should return Trash
	if ta.GetActiveMailbox() != MailboxTrash {
		t.Errorf("Expected GetActiveMailbox to return MailboxTrash, got %v", ta.GetActiveMailbox())
	}

	// GetActiveInbox should return trashInbox
	if ta.GetActiveInbox() != ta.trashInbox {
		t.Error("Expected GetActiveInbox to return trashInbox")
	}
}

// TestTrashArchiveToggle verifies toggling between trash and archive views.
func TestTrashArchiveToggle(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	trashEmails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Trash Email", AccountID: "account-1"},
	}

	archiveEmails := []fetcher.Email{
		{UID: 2, From: "b@example.com", Subject: "Archive Email", AccountID: "account-1"},
	}

	ta := NewTrashArchive(trashEmails, archiveEmails, accounts)

	// Initially should be Trash
	if ta.activeView != MailboxTrash {
		t.Fatalf("Expected initial view to be MailboxTrash, got %v", ta.activeView)
	}

	// Press tab to switch to Archive
	ta.Update(tea.KeyMsg{Type: tea.KeyTab})

	if ta.activeView != MailboxArchive {
		t.Errorf("Expected view to be MailboxArchive after tab, got %v", ta.activeView)
	}

	if ta.GetActiveInbox() != ta.archiveInbox {
		t.Error("Expected GetActiveInbox to return archiveInbox after toggle")
	}

	// Press tab again to switch back to Trash
	ta.Update(tea.KeyMsg{Type: tea.KeyTab})

	if ta.activeView != MailboxTrash {
		t.Errorf("Expected view to be MailboxTrash after second tab, got %v", ta.activeView)
	}
}

// TestTrashArchiveRemoveEmail verifies removing emails from the correct inbox.
func TestTrashArchiveRemoveEmail(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	trashEmails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Trash Email 1", AccountID: "account-1"},
		{UID: 2, From: "b@example.com", Subject: "Trash Email 2", AccountID: "account-1"},
	}

	archiveEmails := []fetcher.Email{
		{UID: 3, From: "c@example.com", Subject: "Archive Email 1", AccountID: "account-1"},
		{UID: 4, From: "d@example.com", Subject: "Archive Email 2", AccountID: "account-1"},
	}

	ta := NewTrashArchive(trashEmails, archiveEmails, accounts)

	// Remove a trash email
	ta.RemoveEmail(1, "account-1", MailboxTrash)

	if len(ta.trashInbox.allEmails) != 1 {
		t.Errorf("Expected 1 trash email after removal, got %d", len(ta.trashInbox.allEmails))
	}

	// Archive emails should be unchanged
	if len(ta.archiveInbox.allEmails) != 2 {
		t.Errorf("Expected 2 archive emails unchanged, got %d", len(ta.archiveInbox.allEmails))
	}

	// Remove an archive email
	ta.RemoveEmail(3, "account-1", MailboxArchive)

	if len(ta.archiveInbox.allEmails) != 1 {
		t.Errorf("Expected 1 archive email after removal, got %d", len(ta.archiveInbox.allEmails))
	}
}

// TestTrashArchiveSetEmails verifies updating emails in trash and archive.
func TestTrashArchiveSetEmails(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	ta := NewTrashArchive(nil, nil, accounts)

	// Set trash emails
	newTrashEmails := []fetcher.Email{
		{UID: 10, From: "new@example.com", Subject: "New Trash", AccountID: "account-1"},
	}
	ta.SetTrashEmails(newTrashEmails, accounts)

	if len(ta.trashInbox.allEmails) != 1 {
		t.Errorf("Expected 1 trash email after SetTrashEmails, got %d", len(ta.trashInbox.allEmails))
	}

	// Set archive emails
	newArchiveEmails := []fetcher.Email{
		{UID: 20, From: "archive@example.com", Subject: "New Archive", AccountID: "account-1"},
		{UID: 21, From: "archive2@example.com", Subject: "New Archive 2", AccountID: "account-1"},
	}
	ta.SetArchiveEmails(newArchiveEmails, accounts)

	if len(ta.archiveInbox.allEmails) != 2 {
		t.Errorf("Expected 2 archive emails after SetArchiveEmails, got %d", len(ta.archiveInbox.allEmails))
	}
}

// TestTrashArchiveWindowSizeMsg verifies window size is passed to both inboxes.
func TestTrashArchiveWindowSizeMsg(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	ta := NewTrashArchive(nil, nil, accounts)

	ta.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	if ta.width != 100 {
		t.Errorf("Expected width 100, got %d", ta.width)
	}
	if ta.height != 50 {
		t.Errorf("Expected height 50, got %d", ta.height)
	}
}

// TestTrashArchiveViewEmailMsg verifies that selecting an email generates correct message.
func TestTrashArchiveViewEmailMsg(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	trashEmails := []fetcher.Email{
		{UID: 100, From: "sender@example.com", Subject: "Test Trash", AccountID: "account-1", Date: time.Now()},
	}

	ta := NewTrashArchive(trashEmails, nil, accounts)

	// Simulate pressing Enter on trash view
	_, cmd := ta.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Expected a command, but got nil")
	}

	msg := cmd()
	viewMsg, ok := msg.(ViewEmailMsg)
	if !ok {
		t.Fatalf("Expected ViewEmailMsg, got %T", msg)
	}

	if viewMsg.UID != 100 {
		t.Errorf("Expected UID 100, got %d", viewMsg.UID)
	}

	if viewMsg.Mailbox != MailboxTrash {
		t.Errorf("Expected Mailbox MailboxTrash, got %v", viewMsg.Mailbox)
	}
}

// TestTrashArchiveDeleteEmailMsg verifies delete messages from trash/archive views.
func TestTrashArchiveDeleteEmailMsg(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	archiveEmails := []fetcher.Email{
		{UID: 200, From: "sender@example.com", Subject: "Test Archive", AccountID: "account-1", Date: time.Now()},
	}

	ta := NewTrashArchive(nil, archiveEmails, accounts)

	// Switch to archive view
	ta.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Simulate pressing 'd' to delete
	_, cmd := ta.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("Expected a command, but got nil")
	}

	msg := cmd()
	deleteMsg, ok := msg.(DeleteEmailMsg)
	if !ok {
		t.Fatalf("Expected DeleteEmailMsg, got %T", msg)
	}

	if deleteMsg.UID != 200 {
		t.Errorf("Expected UID 200, got %d", deleteMsg.UID)
	}

	if deleteMsg.Mailbox != MailboxArchive {
		t.Errorf("Expected Mailbox MailboxArchive, got %v", deleteMsg.Mailbox)
	}
}

// TestTrashArchiveMultipleAccounts verifies tabs are created for multiple accounts.
func TestTrashArchiveMultipleAccounts(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test1@example.com", FetchEmail: "fetch1@example.com"},
		{ID: "account-2", Email: "test2@example.com", FetchEmail: "fetch2@example.com"},
	}

	trashEmails := []fetcher.Email{
		{UID: 1, From: "a@example.com", Subject: "Trash 1", AccountID: "account-1"},
		{UID: 2, From: "b@example.com", Subject: "Trash 2", AccountID: "account-2"},
	}

	ta := NewTrashArchive(trashEmails, nil, accounts)

	// Both trash and archive inboxes should have 3 tabs (ALL + 2 accounts)
	if len(ta.trashInbox.tabs) != 3 {
		t.Errorf("Expected 3 tabs in trashInbox, got %d", len(ta.trashInbox.tabs))
	}

	if len(ta.archiveInbox.tabs) != 3 {
		t.Errorf("Expected 3 tabs in archiveInbox, got %d", len(ta.archiveInbox.tabs))
	}

	// Verify FetchEmail is used for tab labels
	if ta.trashInbox.tabs[1].Label != "fetch1@example.com" {
		t.Errorf("Expected tab label 'fetch1@example.com', got %q", ta.trashInbox.tabs[1].Label)
	}
}

// TestTrashInboxMailboxKind verifies that trash inbox has correct mailbox kind.
func TestTrashInboxMailboxKind(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	inbox := NewTrashInbox(nil, accounts)

	if inbox.GetMailbox() != MailboxTrash {
		t.Errorf("Expected MailboxTrash, got %v", inbox.GetMailbox())
	}
}

// TestArchiveInboxMailboxKind verifies that archive inbox has correct mailbox kind.
func TestArchiveInboxMailboxKind(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	inbox := NewArchiveInbox(nil, accounts)

	if inbox.GetMailbox() != MailboxArchive {
		t.Errorf("Expected MailboxArchive, got %v", inbox.GetMailbox())
	}
}

// TestInboxGetBaseTitle verifies correct titles for different mailbox kinds.
func TestInboxGetBaseTitle(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com"},
	}

	tests := []struct {
		name     string
		mailbox  MailboxKind
		expected string
	}{
		{"Inbox", MailboxInbox, "Inbox"},
		{"Sent", MailboxSent, "Sent"},
		{"Trash", MailboxTrash, "Trash"},
		{"Archive", MailboxArchive, "Archive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inbox := NewInboxWithMailbox(nil, accounts, tt.mailbox)
			title := inbox.getBaseTitle()
			if title != tt.expected {
				t.Errorf("Expected title %q, got %q", tt.expected, title)
			}
		})
	}
}

// TestInboxFetchEmailUsedForTabs verifies that FetchEmail is used for tab labels.
func TestInboxFetchEmailUsedForTabs(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "login@example.com", FetchEmail: "display@example.com"},
		{ID: "account-2", Email: "login2@example.com"}, // No FetchEmail, should fallback to Email
	}

	inbox := NewInbox(nil, accounts)

	// First account should use FetchEmail
	if inbox.tabs[1].Label != "display@example.com" {
		t.Errorf("Expected tab label 'display@example.com', got %q", inbox.tabs[1].Label)
	}

	// Second account should fallback to Email
	if inbox.tabs[2].Label != "login2@example.com" {
		t.Errorf("Expected tab label 'login2@example.com', got %q", inbox.tabs[2].Label)
	}
}
