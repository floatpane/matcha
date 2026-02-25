package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/fetcher"
)

func TestEmailViewUpdate(t *testing.T) {
	emailWithAttachments := fetcher.Email{
		From:    "test@example.com",
		Subject: "Test Email with Attachments",
		Body:    "This is the body.",
		Date:    time.Now(),
		Attachments: []fetcher.Attachment{
			{Filename: "attachment1.txt", Data: []byte("attachment1")},
			{Filename: "attachment2.txt", Data: []byte("attachment2")},
		},
	}

	emailWithoutAttachments := fetcher.Email{
		From:    "test@example.com",
		Subject: "Test Email without Attachments",
		Body:    "This is the body.",
		Date:    time.Now(),
	}

	t.Run("Focus on attachments", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)
		if emailView.focusOnAttachments {
			t.Error("focusOnAttachments should be initially false")
		}

		// Tab to focus on attachments
		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)

		if !emailView.focusOnAttachments {
			t.Error("focusOnAttachments should be true after tabbing")
		}

		// Tab back to body
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)
		if emailView.focusOnAttachments {
			t.Error("focusOnAttachments should be false after tabbing again")
		}
	})

	t.Run("No focus on attachments when there are none", func(t *testing.T) {
		emailView := NewEmailView(emailWithoutAttachments, 0, 80, 24, MailboxInbox, false)
		if emailView.focusOnAttachments {
			t.Error("focusOnAttachments should be initially false")
		}
		// Tab
		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)
		if emailView.focusOnAttachments {
			t.Error("focusOnAttachments should remain false when there are no attachments")
		}
	})

	t.Run("Navigate attachments", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)
		// Focus on attachments
		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)

		if emailView.attachmentCursor != 0 {
			t.Errorf("Initial attachmentCursor should be 0, got %d", emailView.attachmentCursor)
		}

		// Move down
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		emailView = model.(*EmailView)
		if emailView.attachmentCursor != 1 {
			t.Errorf("After one down arrow, attachmentCursor should be 1, got %d", emailView.attachmentCursor)
		}

		// Move down again (should not go past the end)
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		emailView = model.(*EmailView)
		if emailView.attachmentCursor != 1 {
			t.Errorf("attachmentCursor should not go past the end of the list, got %d", emailView.attachmentCursor)
		}

		// Move up
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		emailView = model.(*EmailView)
		if emailView.attachmentCursor != 0 {
			t.Errorf("After one up arrow, attachmentCursor should be 0, got %d", emailView.attachmentCursor)
		}
	})

	t.Run("Download attachment", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)
		// Focus on attachments
		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)

		// Move to the second attachment
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		emailView = model.(*EmailView)

		// Press enter
		_, cmd := emailView.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Expected a command, but got nil")
		}

		msg := cmd()
		downloadMsg, ok := msg.(DownloadAttachmentMsg)
		if !ok {
			t.Fatalf("Expected a DownloadAttachmentMsg, but got %T", msg)
		}
		if downloadMsg.Filename != "attachment2.txt" {
			t.Errorf("Expected to download 'attachment2.txt', but got '%s'", downloadMsg.Filename)
		}
		if downloadMsg.Mailbox != MailboxInbox {
			t.Errorf("Expected mailbox to be MailboxInbox, got %s", downloadMsg.Mailbox)
		}
	})

	t.Run("Reply to email", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)

		_, cmd := emailView.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		if cmd == nil {
			t.Fatal("Expected a command, but got nil")
		}

		msg := cmd()
		replyMsg, ok := msg.(ReplyToEmailMsg)
		if !ok {
			t.Fatalf("Expected a ReplyToEmailMsg, but got %T", msg)
		}
		if replyMsg.Email.Subject != emailWithAttachments.Subject {
			t.Errorf("Expected reply to have subject '%s', but got '%s'", emailWithAttachments.Subject, replyMsg.Email.Subject)
		}
	})
}
