package fetcher

import (
	"bytes"
	"strings"
	"testing"

	"github.com/floatpane/matcha/config"
)

type testPartHeader map[string]string

func (h testPartHeader) Add(key, value string) {
	h[key] = value
}

func (h testPartHeader) Del(key string) {
	delete(h, key)
}

func (h testPartHeader) Get(key string) string {
	return h[key]
}

func (h testPartHeader) Set(key, value string) {
	h[key] = value
}

func TestDecodePartUsesCharsetWhenContentTypeIsMalformed(t *testing.T) {
	header := testPartHeader{}
	header.Set("Content-Type", "text/plain; charset=iso-8859-1; broken")

	decoded, err := decodePart(bytes.NewReader([]byte{0x63, 0x61, 0x66, 0xe9}), header)
	if err != nil {
		t.Fatalf("decodePart() returned error: %v", err)
	}

	if decoded != "café" {
		t.Fatalf("decodePart() = %q, want %q", decoded, "café")
	}
}

func TestDecodePartFallsBackToUTF8WhenMalformedContentTypeHasNoCharset(t *testing.T) {
	header := testPartHeader{}
	header.Set("Content-Type", "text/plain; broken")

	decoded, err := decodePart(strings.NewReader("hello"), header)
	if err != nil {
		t.Fatalf("decodePart() returned error: %v", err)
	}

	if decoded != "hello" {
		t.Fatalf("decodePart() = %q, want %q", decoded, "hello")
	}
}

// TestFetchEmails is an integration test that requires a live IMAP server and valid credentials.
// NOTE: This test will be skipped if it cannot load a configuration file,
// making it safe to run in a CI environment without credentials.
// To run this test locally, ensure you have a valid `config.json` file.
func TestFetchEmails(t *testing.T) {
	// Attempt to load the configuration.
	cfg, err := config.LoadConfig()
	if err != nil {
		// If config doesn't exist, skip the test. This is useful for CI environments.
		t.Skipf("Skipping TestFetchEmails: could not load config: %v", err)
	}

	// Check if there are any accounts configured
	if !cfg.HasAccounts() {
		t.Skip("Skipping TestFetchEmails: no accounts configured.")
	}

	// Get the first account
	account := cfg.GetFirstAccount()
	if account == nil {
		t.Skip("Skipping TestFetchEmails: no accounts available.")
	}

	// If the password is a placeholder, skip the test to avoid failed auth attempts.
	if account.Password == "" || account.Password == "supersecret" {
		t.Skip("Skipping TestFetchEmails: placeholder or empty password found in config.")
	}

	emails, err := FetchEmails(account, 10, 10)
	if err != nil {
		t.Fatalf("FetchEmails() failed with error: %v", err)
	}

	if len(emails) == 0 {
		// This is not necessarily a failure, but we can log it.
		t.Log("FetchEmails() returned 0 emails. This might be expected.")
	}

	// Check that the emails are sorted from newest to oldest.
	// Skip emails with zero/invalid dates when checking sort order.
	if len(emails) > 1 {
		var validEmails []Email
		for _, e := range emails {
			if !e.Date.IsZero() {
				validEmails = append(validEmails, e)
			}
		}
		if len(validEmails) > 1 {
			if validEmails[0].Date.Before(validEmails[len(validEmails)-1].Date) {
				t.Error("Emails do not appear to be sorted from newest to oldest.")
			}
		}
	}

	// Check a sample email for expected content.
	for _, email := range emails {
		if email.Subject == "" && email.From == "" {
			t.Errorf("Fetched email has empty subject and from fields: %+v", email)
		}
	}

	// Verify that AccountID is set on fetched emails
	for _, email := range emails {
		if email.AccountID != account.ID {
			t.Errorf("Expected AccountID %s, got %s", account.ID, email.AccountID)
		}
	}
}

// TestFetchEmailsWithCustomServer tests fetching with a custom server configuration.
// This test is skipped unless a custom account is configured.
func TestFetchEmailsWithCustomServer(t *testing.T) {
	// Attempt to load the configuration.
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Skipf("Skipping TestFetchEmailsWithCustomServer: could not load config: %v", err)
	}

	// Look for a custom account
	var customAccount *config.Account
	for i := range cfg.Accounts {
		if cfg.Accounts[i].ServiceProvider == "custom" {
			customAccount = &cfg.Accounts[i]
			break
		}
	}

	if customAccount == nil {
		t.Skip("Skipping TestFetchEmailsWithCustomServer: no custom account configured.")
	}

	if customAccount.Password == "" || customAccount.Password == "supersecret" {
		t.Skip("Skipping TestFetchEmailsWithCustomServer: placeholder or empty password found.")
	}

	if customAccount.IMAPServer == "" {
		t.Skip("Skipping TestFetchEmailsWithCustomServer: no IMAP server configured.")
	}

	emails, err := FetchEmails(customAccount, 5, 0)
	if err != nil {
		t.Fatalf("FetchEmails() with custom server failed: %v", err)
	}

	t.Logf("Fetched %d emails from custom server %s", len(emails), customAccount.IMAPServer)
}
