package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
)

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

		// Simulate pressing Tab again to move to the 'EncryptSMIME' toggle.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusEncryptSMIME {
			t.Errorf("After seven Tabs, focusIndex should be %d (focusEncryptSMIME), got %d", focusEncryptSMIME, composer.focusIndex)
		}

		// Simulate pressing Tab again to move to the 'EncryptPGP' toggle.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusEncryptPGP {
			t.Errorf("After eight Tabs, focusIndex should be %d (focusEncryptPGP), got %d", focusEncryptPGP, composer.focusIndex)
		}

		// Simulate pressing Tab again to move to the 'Send' button.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusSend {
			t.Errorf("After nine Tabs, focusIndex should be %d (focusSend), got %d", focusSend, composer.focusIndex)
		}

		// Simulate one more Tab to wrap around.
		// With single account, From field is skipped, so it wraps to focusTo.
		model, _ = composer.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		composer = model.(*Composer)
		if composer.focusIndex != focusTo {
			t.Errorf("After ten Tabs, focusIndex should wrap to %d (focusTo) since single account skips From, got %d", focusTo, composer.focusIndex)
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

		// Navigate down to select second account
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		multiComposer = model.(*Composer)

		if multiComposer.selectedAccountIdx != 1 {
			t.Errorf("Expected selectedAccountIdx to be 1, got %d", multiComposer.selectedAccountIdx)
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

		// Tab through all fields: To -> Cc -> Bcc -> Subject -> Body -> Signature -> Attachment -> EncryptSMIME -> Send -> From (wrap)
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
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Attachment -> EncryptSMIME
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // EncryptSMIME -> EncryptPGP
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // EncryptPGP -> Send
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Send -> From (wrap)
		multiComposer = model.(*Composer)
		model, _ = multiComposer.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // From -> To (wrap)
		multiComposer = model.(*Composer)

		// With multiple accounts, From field should be included in tab order
		if multiComposer.focusIndex != focusTo {
			t.Errorf("After ten Tabs with multi-account, focusIndex should wrap to %d (focusTo), got %d", focusTo, multiComposer.focusIndex)
		}
	})
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

	// Small window: heights should not go below minimums
	model, _ = composer.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	composer = model.(*Composer)
	if composer.bodyInput.Height() < 3 {
		t.Errorf("bodyInput height should be at least 3, got %d", composer.bodyInput.Height())
	}
	if composer.signatureInput.Height() < 2 {
		t.Errorf("signatureInput height should be at least 2, got %d", composer.signatureInput.Height())
	}
}
