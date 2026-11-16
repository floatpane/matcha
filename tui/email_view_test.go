package tui

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/view"
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
	folder.Update(remoteImagesLoadedMsg{requestID: 42, body: &view.RenderedBody{Text: "loaded"}})
	if !preview.showRemoteImages || preview.remoteImagesLoading {
		t.Fatal("folder preview did not receive the remote image result")
	}

	replyEmail := NewEmailView(email, 0, 80, 24, MailboxInbox, false)
	replyEmail.remoteImagesLoading = true
	replyEmail.remoteLoadID = 43
	reply := &ReplySplitView{emailView: replyEmail, focusComposer: true}
	reply.Update(remoteImagesLoadedMsg{requestID: 43, body: &view.RenderedBody{Text: "loaded"}})
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

func TestRemoteImageFailuresAreReportedAndRetryable(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	// A private address is refused without touching the network.
	email := fetcher.Email{Body: `<p>hi</p><img src="http://127.0.0.1:1/x.png" alt="pic">`, BodyMIMEType: "text/html"}
	ev := NewEmailView(email, 0, 80, 24, MailboxInbox, false)
	_, cmd := ev.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	if cmd == nil {
		t.Fatal("I did not start loading remote images")
	}
	_, notice := ev.Update(cmd())
	if notice == nil {
		t.Fatal("a failed remote image load was not reported")
	}
	if msg, ok := notice().(NotifyMsg); !ok || msg.Message != "The remote image could not be loaded" {
		t.Fatalf("failure notice = %#v", notice())
	}
	if help := ev.renderHelp(); !strings.Contains(ansi.Strip(help), "I: retry remote") {
		t.Fatalf("help does not offer a retry: %q", help)
	}

	_, retry := ev.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	if retry == nil || !ev.remoteImagesLoading {
		t.Fatal("I did not retry the failed remote images")
	}
	retried := retry().(remoteImagesLoadedMsg)
	if !retried.userRequested {
		t.Fatal("a retry is a load the user asked for")
	}
	ev.Update(retried)

	// Re-rendering for a new size skips the known failure and says nothing.
	_, resize := ev.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	if resize == nil {
		t.Fatal("resizing did not re-render the remote images")
	}
	if skip := ev.remoteImagesToSkip(false); !skip["http://127.0.0.1:1/x.png"] || len(skip) != 1 {
		t.Fatalf("a re-render for a new size would download %v again", skip)
	}
	if skip := ev.remoteImagesToSkip(true); skip != nil {
		t.Fatalf("a retry the user asked for skips %v", skip)
	}
	reloaded := resize().(remoteImagesLoadedMsg)
	if reloaded.userRequested || len(reloaded.body.FailedRemoteImages) != 1 {
		t.Fatalf("resize reload: userRequested=%v failed=%v", reloaded.userRequested, reloaded.body.FailedRemoteImages)
	}
	if _, cmd := ev.Update(reloaded); cmd != nil {
		t.Fatalf("a resize reload reported something: %#v", cmd())
	}

	// A resize during a load re-renders once the load is done.
	_, cmd = ev.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	ev.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	_, after := ev.Update(cmd())
	if after == nil || !ev.remoteImagesLoading {
		t.Fatal("images loaded for the old size were not re-rendered after a resize")
	}
}

func TestLoadingRemoteImagesOfEmailWithoutAnySaysSo(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	ev := NewEmailView(fetcher.Email{Body: "plain", BodyMIMEType: "text/plain"}, 0, 80, 24, MailboxInbox, false)
	_, cmd := ev.Update(LoadRemoteImagesMsg{})
	if cmd == nil {
		t.Fatal("LoadRemoteImagesMsg did not start a load")
	}
	_, notice := ev.Update(cmd())
	if notice == nil {
		t.Fatal("no notice for an email without remote images")
	}
	if msg, ok := notice().(InfoNotifyMsg); !ok || !strings.Contains(msg.Message, "no remote images") {
		t.Fatalf("notice = %#v", notice())
	}
	if _, cmd := ev.Update(LoadRemoteImagesMsg{}); cmd != nil {
		t.Fatal("loading again after a complete load should do nothing")
	}
}

func TestLoadRemoteImagesMsgReachesSplitPreview(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	preview := NewEmailView(fetcher.Email{Body: "x", BodyMIMEType: "text/plain"}, 0, 80, 24, MailboxInbox, false)
	folder := &FolderInbox{previewPane: preview}
	if _, cmd := folder.Update(LoadRemoteImagesMsg{}); cmd == nil || !preview.remoteImagesLoading {
		t.Fatal("the split preview did not start loading remote images")
	}
}

func TestFirstLayoutMakesRoomForTheRemoteImagesHint(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	email := fetcher.Email{Body: `<p>hi</p><img src="https://example.com/a.png">`, BodyMIMEType: "text/html"}
	for width := 40; width <= 140; width++ {
		ev := NewEmailViewPreview(email, width, 30, 0, 0, false)
		if body, chrome := ev.viewport.Height(), ev.chromeHeight(); body+chrome != 30 {
			t.Fatalf("width %d: body %d + chrome %d rows, want 30 (the help bar is cut off)", width, body, chrome)
		}
		if !strings.Contains(strings.Join(strings.Fields(ansi.Strip(ev.renderHelp())), " "), "load remote images") {
			t.Fatalf("width %d: the help has no remote images hint", width)
		}
	}
}
