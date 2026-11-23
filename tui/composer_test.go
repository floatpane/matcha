package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/spellcheck"
)

func TestMailingListSuggestionTruncates(t *testing.T) {
	composer := NewComposer("", "", "", "", false)
	composer.width = 60

	addresses := make([]string, 20)
	for i := range addresses {
		addresses[i] = fmt.Sprintf("very.long.recipient.%02d@example.com", i)
	}

	display := suggestionDisplay(config.Contact{
		Name:      "Team",
		Addresses: addresses,
	}, suggestionDisplayWidth(composer.width))

	if got, want := len([]rune(display)), suggestionDisplayWidth(composer.width); got > want {
		t.Fatalf("Expected mailing-list suggestion to be at most %d runes, got %d: %q", want, got, display)
	}

	singleAddress := config.Contact{
		Name:  "Very Long Contact Name That Should Stay Fully Visible",
		Email: "very.long.single.address.that.exceeds.width@example.com",
	}
	singleDisplay := suggestionDisplay(singleAddress, suggestionDisplayWidth(composer.width))
	expected := fmt.Sprintf("%s <%s>", singleAddress.Name, singleAddress.Email)
	if singleDisplay != expected {
		t.Fatalf("Expected single-address suggestion to stay untruncated, got %q", singleDisplay)
	}
}

func TestNormalizeEmailList(t *testing.T) {
	got, ok := normalizeEmailList("Alice Example <alice@example.com>, bob@example.com")
	if !ok {
		t.Fatal("Expected valid email list")
	}
	if want := "alice@example.com, bob@example.com"; got != want {
		t.Fatalf("normalizeEmailList() = %q, want %q", got, want)
	}

	if _, ok := normalizeEmailList("not-an-email"); ok {
		t.Fatal("Expected invalid email list")
	}
}

func TestComposerEmailValidationOnFieldBlur(t *testing.T) {
	composer := NewComposer("", "", "", "", false)
	composer.toInput.SetValue("not-an-email")

	model, _ := composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	composer = model.(*Composer)

	if composer.toError == "" {
		t.Fatal("Expected To validation error after leaving invalid field")
	}
	if !strings.Contains(fmt.Sprint(composer.View()), composer.toError) {
		t.Fatal("Expected validation error to be rendered below To field")
	}
}

func TestComposerFromValidationOnFieldBlur(t *testing.T) {
	tests := []struct {
		name      string
		from      string
		wantError bool
	}{
		{
			name:      "invalid from",
			from:      "not-an-email",
			wantError: true,
		},
		{
			name: "bare address",
			from: "user@example.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := []config.Account{
				{ID: "account-1", Email: "user@example.org", CatchAll: true},
			}
			composer := NewComposerWithAccounts(accounts, "account-1", "", "", "", false)
			composer.focusIndex = focusFrom
			composer.fromInput.Focus()
			composer.fromInput.SetValue(tt.from)

			model, _ := composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			composer = model.(*Composer)

			if tt.wantError {
				if composer.fromError == "" {
					t.Fatal("Expected From validation error after leaving invalid catch-all From field")
				}
				if !strings.Contains(fmt.Sprint(composer.View()), composer.fromError) {
					t.Fatal("Expected From validation error to be rendered below From field")
				}
				return
			}
			if composer.fromError != "" {
				t.Fatalf("Expected From address to be valid, got %q", composer.fromError)
			}
		})
	}
}

func TestComposerEmailValidationClearsWhenTyping(t *testing.T) {
	composer := NewComposer("", "", "", "", false)
	composer.toInput.SetValue("not-an-email")

	model, _ := composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	composer = model.(*Composer)
	if composer.toError == "" {
		t.Fatal("Expected To validation error after leaving invalid field")
	}

	composer.focusIndex = focusTo
	composer.toInput.Focus()
	model, _ = composer.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	composer = model.(*Composer)

	if composer.toError != "" {
		t.Fatalf("Expected To validation error to clear when typing, got %q", composer.toError)
	}
}

func TestComposerSendValidatesEmailFields(t *testing.T) {
	tests := []struct {
		name          string
		to            string
		cc            string
		catchAllFrom  string
		wantCcError   bool
		wantFromError bool
	}{
		{
			name:        "invalid cc",
			to:          "recipient@example.com",
			cc:          "not-an-email",
			wantCcError: true,
		},
		{
			name:          "invalid catch-all from",
			to:            "recipient@example.com",
			catchAllFrom:  "not-an-email",
			wantFromError: true,
		},
		{
			name: "no recipients",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var composer *Composer
			if tt.catchAllFrom != "" {
				accounts := []config.Account{
					{ID: "account-1", Email: "user@example.org", CatchAll: true},
				}
				composer = NewComposerWithAccounts(accounts, "account-1", "", "", "", false)
				composer.fromInput.SetValue(tt.catchAllFrom)
			} else {
				composer = NewComposer("", "", "", "", false)
			}
			composer.toInput.SetValue(tt.to)
			composer.ccInput.SetValue(tt.cc)
			composer.subjectInput.SetValue("Test Subject")
			composer.bodyInput.SetValue("This is the body.")
			composer.focusIndex = focusSend

			model, cmd := composer.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			composer = model.(*Composer)

			if cmd == nil {
				t.Fatal("Expected a NotifyMsg command after failed send attempt")
			}
			msg := cmd()
			if _, ok := msg.(NotifyMsg); !ok {
				t.Fatalf("Expected NotifyMsg from failed send, got %T", msg)
			}
			if tt.wantCcError && composer.ccError == "" {
				t.Fatal("Expected Cc validation error after send attempt")
			}
			if tt.wantFromError && composer.fromError == "" {
				t.Fatal("Expected From validation error after send attempt")
			}
			if tt.wantCcError && !strings.Contains(fmt.Sprint(composer.View()), composer.ccError) {
				t.Fatal("Expected Cc validation error to be rendered in composer view")
			}
			if tt.wantFromError && !strings.Contains(fmt.Sprint(composer.View()), composer.fromError) {
				t.Fatal("Expected From validation error to be rendered in composer view")
			}
		})
	}
}

func TestComposerContactSuggestionUsesDisplayName(t *testing.T) {
	composer := NewComposer("", "", "", "", false)
	composer.showSuggestions = true
	composer.suggestions = []config.Contact{{
		Name:  "Alice Example",
		Email: "alice@example.com",
	}}

	model, _ := composer.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	composer = model.(*Composer)

	if got, want := composer.toInput.Value(), "Alice Example <alice@example.com>, "; got != want {
		t.Fatalf("Expected suggestion to insert display-name address, got %q, want %q", got, want)
	}
}

// TestComposerUpdate verifies the state transitions in the email composer.
func TestComposerUpdate(t *testing.T) {
	// Initialize a new composer with accounts.
	accounts := []config.Account{
		{ID: "account-1", Email: "test@example.com", Name: "Test User"},
	}
	composer := NewComposerWithAccounts(accounts, "account-1", "", "", "", false)

	t.Run("Focus cycling", func(t *testing.T) {
		// Initial focus is on the 'To' input (index 1, since From is 0).
		// But NewComposer starts focus at focusTo which is 1.
		if composer.focusIndex != focusTo {
			t.Errorf("Initial focusIndex should be %d (focusTo), got %d", focusTo, composer.focusIndex)
		}

		// Simulate pressing Tab to move to the 'Cc' field.
		model, _ := composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusCc {
			t.Errorf("After one Tab, focusIndex should be %d (focusCc), got %d", focusCc, composer.focusIndex)
		}

		// Simulate pressing Tab to move to the 'Bcc' field.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusBcc {
			t.Errorf("After two Tabs, focusIndex should be %d (focusBcc), got %d", focusBcc, composer.focusIndex)
		}

		// Simulate pressing Tab to move to the 'Subject' field.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusSubject {
			t.Errorf("After three Tabs, focusIndex should be %d (focusSubject), got %d", focusSubject, composer.focusIndex)
		}

		// Simulate pressing Tab again to move to the 'Body' field.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusBody {
			t.Errorf("After four Tabs, focusIndex should be %d (focusBody), got %d", focusBody, composer.focusIndex)
		}

		// Simulate pressing Tab again to move to the 'Signature' field.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusSignature {
			t.Errorf("After five Tabs, focusIndex should be %d (focusSignature), got %d", focusSignature, composer.focusIndex)
		}

		// Simulate pressing Tab again to move to the 'Attachment' field.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusAttachment {
			t.Errorf("After six Tabs, focusIndex should be %d (focusAttachment), got %d", focusAttachment, composer.focusIndex)
		}

		// Simulate pressing Tab again to move to the 'Send' button.
		// Crypto toggles are skipped when S/MIME and PGP are not configured.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusSend {
			t.Errorf("After seven Tabs, focusIndex should be %d (focusSend), got %d", focusSend, composer.focusIndex)
		}

		// Simulate one more Tab to wrap around.
		// With single account, From field is skipped, so it wraps to focusTo.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusTo {
			t.Errorf("After eight Tabs, focusIndex should wrap to %d (focusTo) since single account skips From, got %d", focusTo, composer.focusIndex)
		}
	})

	t.Run("Send email message", func(t *testing.T) {
		// Re-initialize composer for this test
		composer = NewComposerWithAccounts(accounts, "account-1", "", "", "", false)

		// Set values for the email fields.
		composer.toInput.SetValue("recipient@example.com")
		composer.subjectInput.SetValue("Test Subject")
		composer.bodyInput.SetValue("This is the body.")
		// Set focus to the Send button.
		composer.focusIndex = focusSend

		// Simulate pressing Enter to send the email.
		_, cmd := composer.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Expected a command to be returned, but got nil.")
		}

		// Execute the command and check the resulting message.
		msg := cmd()
		sendMsg, ok := msg.(SendEmailMsg)
		if !ok {
			t.Fatalf("Expected a SendEmailMsg, but got %T", msg)
		}

		// Verify the content of the message.
		if sendMsg.To != "recipient@example.com" {
			t.Errorf("Expected To 'recipient@example.com', got %q", sendMsg.To)
		}
		if sendMsg.Subject != "Test Subject" {
			t.Errorf("Expected Subject 'Test Subject', got %q", sendMsg.Subject)
		}
		if sendMsg.Body != "This is the body." {
			t.Errorf("Expected Body 'This is the body.', got %q", sendMsg.Body)
		}
		if sendMsg.AccountID != "account-1" {
			t.Errorf("Expected AccountID 'account-1', got %q", sendMsg.AccountID)
		}
	})

	t.Run("Account picker with multiple accounts", func(t *testing.T) {
		multiAccounts := []config.Account{
			{ID: "account-1", Email: "test1@example.com", Name: "User 1"},
			{ID: "account-2", Email: "test2@example.com", Name: "User 2"},
		}
		multiComposer := NewComposerWithAccounts(multiAccounts, "account-1", "", "", "", false)

		// Move focus to From field
		multiComposer.focusIndex = focusFrom

		// Press Enter to open account picker
		model, _ := multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		multiComposer = model.(*Composer)

		if !multiComposer.showAccountPicker {
			t.Error("Expected account picker to be shown")
		}

		// Navigate down to highlight the second account; the selection is
		// only applied on enter.
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		multiComposer = model.(*Composer)

		if multiComposer.accountPickerCursor != 1 {
			t.Errorf("Expected accountPickerCursor to be 1, got %d", multiComposer.accountPickerCursor)
		}
		if multiComposer.selectedAccountIdx != 0 {
			t.Errorf("Expected selectedAccountIdx to stay 0 until enter, got %d", multiComposer.selectedAccountIdx)
		}

		// Press Enter to confirm selection
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		multiComposer = model.(*Composer)

		if multiComposer.showAccountPicker {
			t.Error("Expected account picker to be closed")
		}

		// Verify the selected account
		if multiComposer.GetSelectedAccountID() != "account-2" {
			t.Errorf("Expected selected account ID 'account-2', got %q", multiComposer.GetSelectedAccountID())
		}
	})

	t.Run("Single account no picker", func(t *testing.T) {
		singleAccounts := []config.Account{
			{ID: "account-1", Email: "test@example.com"},
		}
		singleComposer := NewComposerWithAccounts(singleAccounts, "account-1", "", "", "", false)

		// Move focus to From field
		singleComposer.focusIndex = focusFrom

		// Press Enter - should not open picker with single account
		model, _ := singleComposer.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		singleComposer = model.(*Composer)

		if singleComposer.showAccountPicker {
			t.Error("Account picker should not open with single account")
		}
	})

	t.Run("Multi-account focus cycling includes From", func(t *testing.T) {
		multiAccounts := []config.Account{
			{ID: "account-1", Email: "test1@example.com"},
			{ID: "account-2", Email: "test2@example.com"},
		}
		multiComposer := NewComposerWithAccounts(multiAccounts, "account-1", "", "", "", false)

		// Initial focus is on 'To' field
		if multiComposer.focusIndex != focusTo {
			t.Errorf("Initial focusIndex should be %d (focusTo), got %d", focusTo, multiComposer.focusIndex)
		}

		// Tab through all fields: To -> Cc -> Bcc -> Subject -> Body -> Signature -> Attachment -> Send -> From (wrap)
		// Crypto toggles are skipped when S/MIME and PGP are not configured.
		model, _ := multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // To -> Cc
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Cc -> Bcc
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Bcc -> Subject
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Subject -> Body
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Body -> Signature
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Signature -> Attachment
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Attachment -> Send
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Send -> From (wrap)
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // From -> To (wrap)
		multiComposer = model.(*Composer)

		// With multiple accounts, From field should be included in tab order
		if multiComposer.focusIndex != focusTo {
			t.Errorf("After nine Tabs with multi-account, focusIndex should wrap to %d (focusTo), got %d", focusTo, multiComposer.focusIndex)
		}
	})
}

func TestFormatAttachmentNameIncludesSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.jpg")
	if err := os.WriteFile(path, make([]byte, 1258291), 0600); err != nil {
		t.Fatal(err)
	}

	got := formatAttachmentName(path)
	want := "image.jpg (1.2 MB)"
	if got != want {
		t.Fatalf("formatAttachmentName() = %q, want %q", got, want)
	}
}

func TestFormatAttachmentNameMissingFile(t *testing.T) {
	got := formatAttachmentName("/missing/image.jpg")
	want := "image.jpg"
	if got != want {
		t.Fatalf("formatAttachmentName() = %q, want %q", got, want)
	}
}

func TestComposerAttachmentSelectionAndRemoval(t *testing.T) {
	composer := NewComposer("", "", "", "", false)
	composer.focusIndex = focusAttachment
	composer.attachmentPaths = []string{"/tmp/a.txt", "/tmp/b.txt", "/tmp/c.txt"}
	composer.attachmentNames = map[string]string{
		"/tmp/a.txt": "a.txt",
		"/tmp/b.txt": "b.txt",
		"/tmp/c.txt": "c.txt",
	}

	model, _ := composer.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	composer = model.(*Composer)
	if composer.attachmentCursor != 1 {
		t.Fatalf("Expected attachmentCursor 1 after Down, got %d", composer.attachmentCursor)
	}

	model, _ = composer.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	composer = model.(*Composer)

	want := []string{"/tmp/a.txt", "/tmp/c.txt"}
	if len(composer.attachmentPaths) != len(want) {
		t.Fatalf("Expected %d attachments after removal, got %d", len(want), len(composer.attachmentPaths))
	}
	for i, path := range want {
		if composer.attachmentPaths[i] != path {
			t.Fatalf("attachmentPaths[%d] = %q, want %q", i, composer.attachmentPaths[i], path)
		}
	}
	if _, ok := composer.attachmentNames["/tmp/b.txt"]; ok {
		t.Fatal("Expected removed attachment display name to be deleted")
	}
	if composer.attachmentCursor != 1 {
		t.Fatalf("Expected cursor to stay on the next attachment, got %d", composer.attachmentCursor)
	}

	model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	composer = model.(*Composer)
	if composer.attachmentCursor != 0 {
		t.Fatalf("Expected attachmentCursor to wrap to 0 after Down, got %d", composer.attachmentCursor)
	}
}

// TestComposerGetFromAddress verifies the from address formatting.
func TestComposerGetFromAddress(t *testing.T) {
	t.Run("With name", func(t *testing.T) {
		accounts := []config.Account{
			{ID: "account-1", FetchEmail: "test@example.com", Name: "Test User"},
		}
		composer := NewComposerWithAccounts(accounts, "account-1", "", "", "", false)

		fromAddr := composer.getFromAddress()
		expected := "Test User <test@example.com>"
		if fromAddr != expected {
			t.Errorf("Expected from address %q, got %q", expected, fromAddr)
		}
	})

	t.Run("Without name", func(t *testing.T) {
		accounts := []config.Account{
			{ID: "account-1", FetchEmail: "test@example.com"},
		}
		composer := NewComposerWithAccounts(accounts, "account-1", "", "", "", false)

		fromAddr := composer.getFromAddress()
		expected := "test@example.com"
		if fromAddr != expected {
			t.Errorf("Expected from address %q, got %q", expected, fromAddr)
		}
	})

	t.Run("Send as overrides fetch email", func(t *testing.T) {
		accounts := []config.Account{
			{ID: "account-1", FetchEmail: "gmail@gmail.com", SendAsEmail: "alias@example.com", Name: "Test User"},
		}
		composer := NewComposerWithAccounts(accounts, "account-1", "", "", "", false)

		fromAddr := composer.getFromAddress()
		expected := "Test User <alias@example.com>"
		if fromAddr != expected {
			t.Errorf("Expected from address %q, got %q", expected, fromAddr)
		}
	})

	t.Run("No accounts", func(t *testing.T) {
		composer := NewComposer("", "", "", "", false)

		fromAddr := composer.getFromAddress()
		if fromAddr != "" {
			t.Errorf("Expected empty from address, got %q", fromAddr)
		}
	})
}

// TestComposerSetSelectedAccount verifies account selection.
func TestComposerSetSelectedAccount(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", FetchEmail: "test1@example.com"},
		{ID: "account-2", FetchEmail: "test2@example.com"},
		{ID: "account-3", FetchEmail: "test3@example.com"},
	}
	composer := NewComposerWithAccounts(accounts, "account-1", "", "", "", false)

	composer.SetSelectedAccount("account-3")
	if composer.selectedAccountIdx != 2 {
		t.Errorf("Expected selectedAccountIdx 2, got %d", composer.selectedAccountIdx)
	}
	if composer.GetSelectedAccountID() != "account-3" {
		t.Errorf("Expected selected account ID 'account-3', got %q", composer.GetSelectedAccountID())
	}

	// Test non-existent account (should not change)
	composer.SetSelectedAccount("non-existent")
	if composer.selectedAccountIdx != 2 {
		t.Errorf("Expected selectedAccountIdx to remain 2, got %d", composer.selectedAccountIdx)
	}
}

// TestComposerDynamicHeight verifies that window resize updates textarea heights.
func TestComposerDynamicHeight(t *testing.T) {
	composer := NewComposer("", "", "", "", false)

	model, _ := composer.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	composer = model.(*Composer)

	if composer.height != 40 {
		t.Errorf("Expected height 40, got %d", composer.height)
	}

	bodyH := composer.bodyInput.Height()
	sigH := composer.signatureInput.Height()

	if bodyH <= 3 {
		t.Errorf("Expected bodyInput height > 3, got %d", bodyH)
	}
	if sigH <= 1 {
		t.Errorf("Expected signatureInput height > 1, got %d", sigH)
	}
	if bodyH <= sigH {
		t.Errorf("Expected bodyInput height (%d) > signatureInput height (%d)", bodyH, sigH)
	}

	// A regular terminal keeps the usual minimum editor sizes.
	model, _ = composer.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	composer = model.(*Composer)
	if composer.bodyInput.Height() < 3 {
		t.Errorf("bodyInput height should be at least 3, got %d", composer.bodyInput.Height())
	}
	if composer.signatureInput.Height() < 2 {
		t.Errorf("signatureInput height should be at least 2, got %d", composer.signatureInput.Height())
	}

	// A tiny window squeezes the editors instead of overflowing the screen.
	model, _ = composer.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	composer = model.(*Composer)
	if composer.bodyInput.Height() < 1 {
		t.Errorf("bodyInput height should be at least 1, got %d", composer.bodyInput.Height())
	}
	assertViewFits(t, "composer 80x10", composer.View().Content, 80, 10)
}

// assertViewFits fails when a rendered view is taller or wider than w×h.
func assertViewFits(t *testing.T, name, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > h {
		t.Errorf("%s: %d rows, want <= %d", name, len(lines), h)
	}
	for i, l := range lines {
		if got := lipgloss.Width(l); got > w {
			t.Errorf("%s: row %d is %d cells wide, want <= %d: %q", name, i, got, w, l)
		}
	}
}

// TestComposerViewFitsWindow renders the composer in its busiest states at
// several sizes; the view must never exceed the window it was given.
func TestComposerViewFitsWindow(t *testing.T) {
	accounts := []config.Account{
		{ID: "a1", Name: "Alexandra Very-Long-Surname", Email: "alexandra.very.long.surname@some-company-domain.example.com",
			PGPPublicKey: "/k", SMIMECert: "/c", SMIMEKey: "/k"},
		{ID: "a2", Name: "Second", Email: "second@example.com"},
	}
	states := map[string]func(c *Composer){
		"default": func(*Composer) {},
		"errors and attachments": func(c *Composer) {
			c.toError, c.ccError, c.bccError = "✗ Invalid email address", "✗ Invalid email address", "✗ Invalid email address"
			for i := range 6 {
				c.attachmentPaths = append(c.attachmentPaths, fmt.Sprintf("/tmp/a/really/long/path/attachment-%d-with-a-long-file-name.pdf", i))
			}
			c.pgpKeysAvailable["bob@example.com"] = true
			c.toInput.SetValue("bob@example.com")
		},
		"suggestions": func(c *Composer) {
			c.suggestions = []config.Contact{
				{Name: "Very Long Contact Name That Should Not Overflow", Email: "very.long.single.address.that.exceeds.width@example.com"},
				{Name: "Bob", Email: "bob@example.com"},
			}
			c.showSuggestions = true
		},
		"plugin help": func(c *Composer) {
			c.SetPluginKeyBindings([]PluginKeyBinding{{Key: "ctrl+g", Description: "grammar check"}, {Key: "ctrl+t", Description: "translate"}})
			c.SetPluginStatus("words: 120")
		},
		"from focused": func(c *Composer) { c.setFocusTo(focusFrom) },
		"account picker": func(c *Composer) {
			c.showAccountPicker = true
		},
		"plugin prompt": func(c *Composer) {
			c.ShowPluginPrompt("Enter a translation target language code (e.g. de, fr, es, ja)")
		},
		"confirm exit": func(c *Composer) { c.confirmingExit = true },
		"wkd confirm": func(c *Composer) {
			c.wkdConfirmRecipients = []string{"someone.with.a.long.address@example-domain.com", "b@example.com"}
		},
		"emoji popup": func(c *Composer) {
			c.setFocusTo(focusBody)
			for _, r := range "line one\nline two\n:smi" {
				if r == '\n' {
					c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
					continue
				}
				c.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
		},
	}
	sizes := [][2]int{{200, 60}, {120, 40}, {80, 24}, {60, 20}, {40, 12}, {20, 8}, {10, 5}, {1, 1}}
	for name, setup := range states {
		for _, sz := range sizes {
			w, h := sz[0], sz[1]
			c := NewComposerWithAccounts(accounts, "a1", "", "", "", false)
			c.Update(tea.WindowSizeMsg{Width: w, Height: h})
			setup(c)
			assertViewFits(t, fmt.Sprintf("%s @%dx%d", name, w, h), c.View().Content, w, h)
		}
	}
}

func typeComposer(c *Composer, s string) {
	for _, r := range s {
		if r == '\n' {
			c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			continue
		}
		c.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// Regression: accepting a spelling/emoji suggestion after the external editor
// replaced the body used stale byte offsets and panicked with "slice bounds
// out of range".
func TestComposerAcceptPopupAfterSetBody(t *testing.T) {
	t.Run("emoji", func(t *testing.T) {
		c := NewComposer("", "", "", "", false)
		c.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		c.setFocusTo(focusBody)
		typeComposer(c, "hello :smi")
		if !c.emojiShow {
			t.Fatal("expected the emoji popup to be open")
		}
		c.SetBody("hi")
		c.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if got := c.GetBody(); got != "hi" {
			t.Fatalf("body = %q, want %q", got, "hi")
		}
	})

	t.Run("spelling", func(t *testing.T) {
		dict := filepath.Join(t.TempDir(), "en.dic")
		if err := os.WriteFile(dict, []byte("2\nhello\nworld\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		checker := spellcheck.NewChecker()
		if err := checker.Load(dict, "en"); err != nil {
			t.Fatal(err)
		}
		c := NewComposer("", "", "", "", false)
		c.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		c.spellChecker = checker
		c.setFocusTo(focusBody)
		typeComposer(c, "hello wrold")
		if !c.spellShow {
			t.Fatal("expected the spelling popup to be open")
		}
		c.SetBody("hi")
		c.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if got := c.GetBody(); got != "hi" {
			t.Fatalf("body = %q, want %q", got, "hi")
		}
	})
}

// Regression: a terminal narrower than 6 columns gave the inputs a negative
// width and bubbles' textinput panicked rendering its placeholder.
func TestComposerTinyWindowNoPanic(t *testing.T) {
	for w := 0; w <= 7; w++ {
		c := NewComposer("", "", "", "", false)
		c.Update(tea.WindowSizeMsg{Width: w, Height: 3})
		_ = c.View()
	}
}

// Regression: Bubble Tea v2 reports the space bar as "space", so the crypto
// toggles ignored it although the tips advertise "Space or Enter".
func TestComposerSpaceTogglesCrypto(t *testing.T) {
	accounts := []config.Account{{ID: "a1", Email: "x@example.com", PGPPublicKey: "/k"}}
	c := NewComposerWithAccounts(accounts, "a1", "", "", "", false)
	c.focusIndex = focusSignPGP
	before := c.signPGP
	c.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if c.signPGP == before {
		t.Fatal("space did not toggle PGP signing")
	}
}

// Mouse clicks are hit-tested against the rows actually rendered, so extra
// rows (validation errors, the suggestion dropdown) don't shift the targets.
func TestComposerMouseClicksFollowRenderedRows(t *testing.T) {
	rowOf := func(c *Composer, needle string) int {
		for i, l := range strings.Split(c.View().Content, "\n") {
			if strings.Contains(ansi.Strip(l), needle) {
				return i
			}
		}
		t.Fatalf("%q not rendered", needle)
		return -1
	}

	c := NewComposer("", "", "", "", false)
	c.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	c.toInput.SetValue("not-an-email")
	c.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // leaves To, adds an error row
	c.Update(tea.MouseClickMsg{X: 4, Y: rowOf(c, "Subject"), Button: tea.MouseLeft})
	if c.focusIndex != focusSubject {
		t.Fatalf("click on the Subject row focused %d, want %d", c.focusIndex, focusSubject)
	}

	c.setFocusTo(focusTo)
	c.suggestions = []config.Contact{{Name: "Bob", Email: "bob@example.com"}, {Name: "Bobby", Email: "bobby@example.com"}}
	c.showSuggestions = true
	c.Update(tea.MouseClickMsg{X: 4, Y: rowOf(c, "bobby@example.com"), Button: tea.MouseLeft})
	if got, want := c.GetTo(), "Bobby <bobby@example.com>, "; got != want {
		t.Fatalf("clicking a suggestion set To to %q, want %q", got, want)
	}

	// From can't take focus with a single account: clicking it is a no-op.
	single := NewComposerWithAccounts([]config.Account{{ID: "a1", Email: "x@example.com"}}, "a1", "", "", "", false)
	single.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	single.Update(tea.MouseClickMsg{X: 4, Y: rowOf(single, "From"), Button: tea.MouseLeft})
	if single.focusIndex != focusTo {
		t.Fatalf("click on From moved focus to %d", single.focusIndex)
	}

	// Dialogs block clicks on the form behind them.
	single.wkdConfirmRecipients = []string{"a@b.c"}
	single.Update(tea.MouseClickMsg{X: 4, Y: 5, Button: tea.MouseLeft})
	if single.focusIndex != focusTo {
		t.Fatalf("click behind the WKD dialog moved focus to %d", single.focusIndex)
	}
}

// Contact suggestions belong to the To field: once focus moves elsewhere,
// Enter must type a newline instead of rewriting To.
func TestComposerSuggestionsOnlyActiveInTo(t *testing.T) {
	c := NewComposer("", "", "", "", false)
	c.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	c.toInput.SetValue("bo")
	c.suggestions = []config.Contact{{Name: "Bob", Email: "bob@example.com"}}
	c.showSuggestions = true
	c.setFocusTo(focusBody)
	typeComposer(c, "Hi\nthere")
	if got := c.GetTo(); got != "bo" {
		t.Fatalf("To = %q, want it untouched", got)
	}
	if got := c.GetBody(); got != "Hi\nthere" {
		t.Fatalf("body = %q, want %q", got, "Hi\nthere")
	}
}

func TestComposerAccountPickerEscCancels(t *testing.T) {
	accounts := []config.Account{{ID: "a1", Email: "a@example.com"}, {ID: "a2", Email: "b@example.com"}}
	c := NewComposerWithAccounts(accounts, "a1", "", "", "", false)
	c.focusIndex = focusFrom
	c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	c.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	c.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if c.showAccountPicker || c.GetSelectedAccountID() != "a1" {
		t.Fatalf("esc should cancel the picker: open=%v account=%q", c.showAccountPicker, c.GetSelectedAccountID())
	}
}

// Hiding Cc/Bcc while one of them is focused must move focus to a visible
// field; rows holding recipients always stay visible.
func TestComposerHiddenCcBcc(t *testing.T) {
	c := NewComposer("", "", "", "", false)
	c.setFocusTo(focusCc)
	c.Update(ToggleCcBccMsg{})
	if c.focusIndex == focusCc || c.focusIndex == focusBcc {
		t.Fatalf("focus stayed on hidden field %d", c.focusIndex)
	}

	c.SetShowCcBcc(false)
	c.SetCc("carol@example.com")
	if !strings.Contains(ansi.Strip(c.View().Content), "carol@example.com") {
		t.Fatal("a Cc recipient must stay visible even when Cc/Bcc are hidden")
	}
}

// An enabled encryption toggle stays visible when its precondition goes
// away, so it can't silently apply (or fail) while hidden.
func TestComposerEnabledEncryptionStaysVisible(t *testing.T) {
	accounts := []config.Account{{ID: "a1", Email: "x@example.com", PGPPublicKey: "/k"}}
	c := NewComposerWithAccounts(accounts, "a1", "bob@example.com", "", "", false)
	c.pgpKeysAvailable["bob@example.com"] = true
	c.focusIndex = focusEncryptPGP
	c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !c.encryptPGP {
		t.Fatal("expected PGP encryption to be on")
	}
	c.toInput.SetValue("carol@example.com")
	if !slices.Contains(c.visibleCryptoToggles(), focusEncryptPGP) {
		t.Fatal("enabled PGP encryption toggle disappeared")
	}
}

func TestComposerMouseWheelScrollsBody(t *testing.T) {
	c := NewComposer("", "", "", "", false)
	c.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	c.setFocusTo(focusBody)
	c.SetBody(strings.Repeat("line\n", 40) + "end")
	_ = c.View()
	before := c.bodyInput.Line()
	c.Update(tea.MouseWheelMsg{X: 10, Y: c.bodyRow, Button: tea.MouseWheelUp})
	if c.bodyInput.Line() >= before {
		t.Fatalf("wheel up did not scroll the body (line %d -> %d)", before, c.bodyInput.Line())
	}
}
