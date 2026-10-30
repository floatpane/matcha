package tui

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/fetcher"
)

func TestRemoteImagePermissionIsPerMessageView(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	email := fetcher.Email{Body: "hello", BodyMIMEType: "text/plain"}
	first := NewEmailView(email, 0, 80, 24, MailboxInbox, true)
	if first.showImages || first.showRemoteImages {
		t.Fatal("image display and remote loading should start disabled")
	}
	_, cmd := first.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	if !first.showImages || !first.remoteImagesLoading || first.showRemoteImages || cmd == nil {
		t.Fatal("explicit remote-image action should queue loading in this view")
	}
	first.Update(cmd())
	if !first.showRemoteImages || first.remoteImagesLoading {
		t.Fatal("remote images should be enabled when loading finishes")
	}
	first.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !first.showRemoteImages {
		t.Fatal("resizing should retain this message's permission")
	}
	second := NewEmailView(email, 1, 80, 24, MailboxInbox, false)
	if second.showRemoteImages {
		t.Fatal("remote-image permission leaked to another message view")
	}
}

func TestRemoteImageLoadingRunsOnlyInCommandAndIsReused(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="
	png, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	t.Cleanup(server.Close)
	email := fetcher.Email{Body: `<img src="` + server.URL + `/test.png" alt="remote">`, BodyMIMEType: "text/html"}
	first := NewEmailView(email, 0, 80, 24, MailboxInbox, false)
	_, cmd := first.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	if cmd == nil || requests.Load() != 0 {
		t.Fatalf("key press made %d requests or returned no command", requests.Load())
	}
	first.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if requests.Load() != 0 {
		t.Fatalf("resize while loading made %d requests", requests.Load())
	}
	result := cmd()
	if requests.Load() != 0 {
		t.Fatalf("background command fetched a local image: %d requests", requests.Load())
	}
	first.Update(result)
	first.Update(tea.WindowSizeMsg{Width: 81, Height: 24})
	if requests.Load() != 0 {
		t.Fatalf("resize after loading fetched a local image: %d requests", requests.Load())
	}
	first.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	first.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if requests.Load() != 0 {
		t.Fatalf("image toggle fetched a local image: %d requests", requests.Load())
	}
	second := NewEmailView(email, 1, 80, 24, MailboxInbox, false)
	second.Update(result)
	if second.showRemoteImages {
		t.Fatal("stale remote image result affected a different view")
	}
}

func TestRemoteImageResultReachesNestedViews(t *testing.T) {
	email := fetcher.Email{Body: "original", BodyMIMEType: "text/plain"}
	preview := NewEmailView(email, 0, 80, 24, MailboxInbox, false)
	preview.remoteImagesLoading = true
	preview.remoteLoadID = 42
	folder := &FolderInbox{previewPane: preview, movingEmail: true}
	folder.Update(remoteImagesLoadedMsg{requestID: 42, body: "loaded"})
	if !preview.showRemoteImages || preview.remoteImagesLoading {
		t.Fatal("folder preview did not receive the remote image result")
	}

	replyEmail := NewEmailView(email, 0, 80, 24, MailboxInbox, false)
	replyEmail.remoteImagesLoading = true
	replyEmail.remoteLoadID = 43
	reply := &ReplySplitView{emailView: replyEmail, focusComposer: true}
	reply.Update(remoteImagesLoadedMsg{requestID: 43, body: "loaded"})
	if !replyEmail.showRemoteImages || replyEmail.remoteImagesLoading {
		t.Fatal("reply email pane did not receive the remote image result")
	}
}

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

		// Move down again (should wrap to the first attachment)
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		emailView = model.(*EmailView)
		if emailView.attachmentCursor != 0 {
			t.Errorf("attachmentCursor should wrap to the start of the list, got %d", emailView.attachmentCursor)
		}

		// Move up (should wrap to the last attachment)
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		emailView = model.(*EmailView)
		if emailView.attachmentCursor != 1 {
			t.Errorf("After one up arrow from the first item, attachmentCursor should be 1, got %d", emailView.attachmentCursor)
		}
	})

	t.Run("Attachment navigation does not scroll body", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)
		emailView.viewport.SetHeight(2)
		emailView.viewport.SetContent("line 1\nline 2\nline 3\nline 4\nline 5\n")
		emailView.viewport.SetYOffset(1)

		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)
		if !emailView.focusOnAttachments {
			t.Fatal("focusOnAttachments should be true after tabbing")
		}

		before := emailView.viewport.YOffset()
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		emailView = model.(*EmailView)
		if got := emailView.viewport.YOffset(); got != before {
			t.Fatalf("attachment navigation should not scroll the email body, got offset %d want %d", got, before)
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
