package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// TestSaveAndLoadConfig verifies that the config can be saved to and loaded from a file correctly.
func TestSaveAndLoadConfig(t *testing.T) {
	// Use an in-memory mock keyring so tests do not interact with the host OS keyring
	keyring.MockInit()

	// Create a temporary directory for the test to avoid interfering with actual user config.
	tempDir := t.TempDir()

	// Temporarily override the user home directory to our temp directory.
	// This ensures that our config file is written to a predictable, temporary location.
	t.Setenv("HOME", tempDir)

	// Define a sample configuration to save with multiple accounts.
	expectedConfig := &Config{
		Accounts: []Account{
			{
				ID:              "test-id-1",
				Name:            "Test User",
				Email:           "test@example.com",
				Password:        "supersecret",
				ServiceProvider: "gmail",
				SendAsEmail:     "alias@example.com",
			},
			{
				ID:              "test-id-2",
				Name:            "Custom User",
				Email:           "custom@example.com",
				Password:        "customsecret",
				ServiceProvider: "custom",
				IMAPServer:      "imap.custom.com",
				IMAPPort:        993,
				SMTPServer:      "smtp.custom.com",
				SMTPPort:        587,
				CatchAll:        true,
			},
		},
	}

	// Attempt to save the configuration.
	err := SaveConfig(expectedConfig)
	if err != nil {
		t.Fatalf("SaveConfig() failed: %v", err)
	}

	// Attempt to load the configuration back.
	loadedConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Compare the loaded configuration with the original one.
	// reflect.DeepEqual is used for a deep comparison of the structs.
	if !reflect.DeepEqual(loadedConfig, expectedConfig) {
		t.Errorf("Loaded config does not match expected config.\nGot:  %+v\nWant: %+v", loadedConfig, expectedConfig)
	}
}

// TestAccountGetIMAPServer tests the logic that determines the IMAP server address.
func TestAccountGetIMAPServer(t *testing.T) {
	testCases := []struct {
		name    string
		account Account
		want    string
	}{
		{"Gmail", Account{ServiceProvider: "gmail"}, "imap.gmail.com"},
		{"iCloud", Account{ServiceProvider: "icloud"}, "imap.mail.me.com"},
		{"Custom", Account{ServiceProvider: "custom", IMAPServer: "imap.custom.com"}, "imap.custom.com"},
		{"Unsupported", Account{ServiceProvider: "yahoo"}, ""},
		{"Empty", Account{ServiceProvider: ""}, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.account.GetIMAPServer()
			if got != tc.want {
				t.Errorf("GetIMAPServer() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAccountGetSMTPServer tests the logic that determines the SMTP server address.
func TestAccountGetSMTPServer(t *testing.T) {
	testCases := []struct {
		name    string
		account Account
		want    string
	}{
		{"Gmail", Account{ServiceProvider: "gmail"}, "smtp.gmail.com"},
		{"iCloud", Account{ServiceProvider: "icloud"}, "smtp.mail.me.com"},
		{"Custom", Account{ServiceProvider: "custom", SMTPServer: "smtp.custom.com"}, "smtp.custom.com"},
		{"Unsupported", Account{ServiceProvider: "yahoo"}, ""},
		{"Empty", Account{ServiceProvider: ""}, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.account.GetSMTPServer()
			if got != tc.want {
				t.Errorf("GetSMTPServer() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestConfigAddRemoveAccount tests adding and removing accounts from config.
func TestConfigAddRemoveAccount(t *testing.T) {
	// Use an in-memory mock keyring to test the deletion step cleanly
	keyring.MockInit()

	cfg := &Config{}

	// Add an account
	account := Account{
		Name:            "Test",
		Email:           "test@example.com",
		ServiceProvider: "gmail",
	}
	cfg.AddAccount(account)

	if len(cfg.Accounts) != 1 {
		t.Fatalf("Expected 1 account, got %d", len(cfg.Accounts))
	}

	// Check that ID was auto-generated
	if cfg.Accounts[0].ID == "" {
		t.Error("Expected account ID to be auto-generated")
	}

	// Remove the account
	accountID := cfg.Accounts[0].ID
	removed := cfg.RemoveAccount(accountID)
	if !removed {
		t.Error("RemoveAccount should return true when account exists")
	}

	if len(cfg.Accounts) != 0 {
		t.Fatalf("Expected 0 accounts after removal, got %d", len(cfg.Accounts))
	}

	// Try to remove non-existent account
	removed = cfg.RemoveAccount("non-existent")
	if removed {
		t.Error("RemoveAccount should return false for non-existent account")
	}
}

// TestConfigGetAccountByID tests retrieving accounts by ID.
func TestConfigGetAccountByID(t *testing.T) {
	cfg := &Config{
		Accounts: []Account{
			{ID: "id-1", Email: "test1@example.com"},
			{ID: "id-2", Email: "test2@example.com"},
		},
	}

	account := cfg.GetAccountByID("id-1")
	if account == nil {
		t.Fatal("Expected to find account with id-1")
	}
	if account.Email != "test1@example.com" {
		t.Errorf("Expected email test1@example.com, got %s", account.Email)
	}

	// Non-existent ID
	account = cfg.GetAccountByID("non-existent")
	if account != nil {
		t.Error("Expected nil for non-existent account ID")
	}
}

// TestConfigGetAccountByEmail tests retrieving accounts by email.
func TestConfigGetAccountByEmail(t *testing.T) {
	cfg := &Config{
		Accounts: []Account{
			{ID: "id-1", Email: "test1@example.com"},
			{ID: "id-2", Email: "test2@example.com"},
		},
	}

	account := cfg.GetAccountByEmail("test2@example.com")
	if account == nil {
		t.Fatal("Expected to find account with test2@example.com")
	}
	if account.ID != "id-2" {
		t.Errorf("Expected ID id-2, got %s", account.ID)
	}

	// Non-existent email
	account = cfg.GetAccountByEmail("nonexistent@example.com")
	if account != nil {
		t.Error("Expected nil for non-existent account email")
	}
}

func TestAddContactNormalizesEmailAndDeduplicates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := AddContact("Alice", "Alice@Example.com"); err != nil {
		t.Fatalf("AddContact() failed: %v", err)
	}
	if err := AddContact("", "alice@example.com"); err != nil {
		t.Fatalf("AddContact() failed: %v", err)
	}

	cache, err := LoadContactsCache()
	if err != nil {
		t.Fatalf("LoadContactsCache() failed: %v", err)
	}

	if len(cache.Contacts) != 1 {
		t.Fatalf("Expected 1 contact after deduplication, got %d", len(cache.Contacts))
	}

	contact := cache.Contacts[0]
	if contact.Email != "alice@example.com" {
		t.Errorf("Expected normalized email alice@example.com, got %s", contact.Email)
	}
	if contact.UseCount != 2 {
		t.Errorf("Expected UseCount 2 after duplicate add, got %d", contact.UseCount)
	}
}

// TestConfigHasAccounts tests the HasAccounts method.
func TestConfigHasAccounts(t *testing.T) {
	cfg := &Config{}
	if cfg.HasAccounts() {
		t.Error("Expected HasAccounts to return false for empty config")
	}

	cfg.AddAccount(Account{Email: "test@example.com"})
	if !cfg.HasAccounts() {
		t.Error("Expected HasAccounts to return true after adding account")
	}
}

// TestAccountGetPorts tests the port retrieval methods.
func TestAccountGetPorts(t *testing.T) {
	// Gmail account should use default ports
	gmailAccount := Account{ServiceProvider: "gmail"}
	if gmailAccount.GetIMAPPort() != 993 {
		t.Errorf("Expected Gmail IMAP port 993, got %d", gmailAccount.GetIMAPPort())
	}
	if gmailAccount.GetSMTPPort() != 587 {
		t.Errorf("Expected Gmail SMTP port 587, got %d", gmailAccount.GetSMTPPort())
	}

	// Custom account with custom ports
	customAccount := Account{
		ServiceProvider: "custom",
		IMAPPort:        1993,
		SMTPPort:        1587,
	}
	if customAccount.GetIMAPPort() != 1993 {
		t.Errorf("Expected custom IMAP port 1993, got %d", customAccount.GetIMAPPort())
	}
	if customAccount.GetSMTPPort() != 1587 {
		t.Errorf("Expected custom SMTP port 1587, got %d", customAccount.GetSMTPPort())
	}

	// Custom account with default ports (0 means use default)
	customDefaultAccount := Account{ServiceProvider: "custom"}
	if customDefaultAccount.GetIMAPPort() != 993 {
		t.Errorf("Expected default IMAP port 993 for custom with no port, got %d", customDefaultAccount.GetIMAPPort())
	}
	if customDefaultAccount.GetSMTPPort() != 587 {
		t.Errorf("Expected default SMTP port 587 for custom with no port, got %d", customDefaultAccount.GetSMTPPort())
	}
}

func TestAccountSendIdentityHelpers(t *testing.T) {
	t.Run("send as takes precedence", func(t *testing.T) {
		account := Account{
			Name:        "Alias User",
			Email:       "login@gmail.com",
			FetchEmail:  "inbox@gmail.com",
			SendAsEmail: "alias@example.com",
		}

		if got := account.GetFetchEmail(); got != "inbox@gmail.com" {
			t.Fatalf("GetFetchEmail() = %q, want %q", got, "inbox@gmail.com")
		}
		if got := account.GetSendAsEmail(); got != "alias@example.com" {
			t.Fatalf("GetSendAsEmail() = %q, want %q", got, "alias@example.com")
		}
		if got := account.FormatFromHeader(); got != "Alias User <alias@example.com>" {
			t.Fatalf("FormatFromHeader() = %q, want %q", got, "Alias User <alias@example.com>")
		}
	})

	t.Run("send as falls back to fetch then login", func(t *testing.T) {
		account := Account{
			Name:       "Fallback User",
			Email:      "login@gmail.com",
			FetchEmail: "inbox@gmail.com",
		}
		if got := account.GetSendAsEmail(); got != "inbox@gmail.com" {
			t.Fatalf("GetSendAsEmail() = %q, want %q", got, "inbox@gmail.com")
		}

		account.FetchEmail = ""
		if got := account.GetSendAsEmail(); got != "login@gmail.com" {
			t.Fatalf("GetSendAsEmail() = %q, want %q", got, "login@gmail.com")
		}
	})

	t.Run("format from header avoids double wrapping", func(t *testing.T) {
		account := Account{
			Name:        "Account Name",
			SendAsEmail: "Custom Name <custom@example.com>",
		}
		if got := account.FormatFromHeader(); got != "Custom Name <custom@example.com>" {
			t.Fatalf("FormatFromHeader() = %q, want %q", got, "Custom Name <custom@example.com>")
		}
	})
}

func TestTranslateDateFormat(t *testing.T) {
	testCases := []struct {
		name   string
		input  string
		want   string
		sample string // expected output of time.Format for a fixed sample instant
	}{
		{"EU preset", DateFormatEU, "02/01/2006 15:04", "17/04/2026 09:05"},
		{"US preset", DateFormatUS, "01/02/2006 03:04 PM", "04/17/2026 09:05 AM"},
		{"ISO preset", DateFormatISO, "2006-01-02 15:04", "2026-04-17 09:05"},
		{"seconds", "HH:MM:SS", "15:04:05", "09:05:00"},
		{"explicit minutes", "YYYY-MM-DD HH:mm", "2006-01-02 15:04", "2026-04-17 09:05"},
		{"2-digit year", "DD/MM/YY", "02/01/06", "17/04/26"},
		{"literal passthrough", "some text", "some text", "some text"},
	}

	sample := time.Date(2026, 4, 17, 9, 5, 0, 0, time.UTC)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := translateDateFormat(tc.input)
			if got != tc.want {
				t.Fatalf("translateDateFormat(%q) = %q, want %q", tc.input, got, tc.want)
			}
			if rendered := sample.Format(got); rendered != tc.sample {
				t.Fatalf("sample.Format(%q) = %q, want %q", got, rendered, tc.sample)
			}
		})
	}
}

func TestConfigGetDateFormatDefault(t *testing.T) {
	c := &Config{}
	got := c.GetDateFormat()
	want := translateDateFormat(DateFormatEU)
	if got != want {
		t.Fatalf("GetDateFormat() with empty DateFormat = %q, want %q", got, want)
	}
}

func TestConfigGetDateFormatCustom(t *testing.T) {
	c := &Config{DateFormat: "DD/MM/YYYY HH:MM"}
	if got, want := c.GetDateFormat(), "02/01/2006 15:04"; got != want {
		t.Fatalf("GetDateFormat() = %q, want %q", got, want)
	}
}
