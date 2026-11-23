package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
)

// mockListService is a mock implementation of the daemonclient.Service for testing.
type mockListService struct {
	daemonclient.Service // embed to satisfy interface
	FetchEmailsCalled    bool
	LastAccountID        string
	LastFolder           string
	EmailsToReturn       []backend.Email
	ErrorToReturn        error
}

func (m *mockListService) FetchEmails(accountID, folderName string, limit, offset uint32) ([]backend.Email, error) {
	m.FetchEmailsCalled = true
	m.LastAccountID = accountID
	m.LastFolder = folderName
	return m.EmailsToReturn, m.ErrorToReturn
}

func (m *mockListService) Close() error { return nil }
func (m *mockListService) Reset() {
	m.FetchEmailsCalled = false
	m.LastAccountID = ""
	m.LastFolder = ""
}

var mockEmails = []backend.Email{
	{UID: 101, From: "sender1@example.com", Subject: "Test Subject 1", Date: time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC), IsRead: false},
	{UID: 102, From: "sender2@example.com", Subject: "Test Subject 2", Date: time.Date(2026, 7, 6, 11, 0, 0, 0, time.UTC), IsRead: true},
}

func TestRunList(t *testing.T) {
	cfg := &config.Config{
		Accounts: []config.Account{
			{Email: "test@example.com", FetchEmail: "test@example.com", ID: "acc1"},
			{Email: "another@example.com", FetchEmail: "another@example.com", ID: "acc2"},
		},
	}

	mockSvc := &mockListService{
		EmailsToReturn: mockEmails,
	}

	originalLoadConfig := configLoadConfig
	originalNewServiceFunc := NewServiceFunc
	originalOut := Out
	defer func() {
		configLoadConfig = originalLoadConfig
		NewServiceFunc = originalNewServiceFunc
		Out = originalOut
	}()

	configLoadConfig = func() (*config.Config, error) {
		return cfg, nil
	}
	NewServiceFunc = func(cfg *config.Config, autoStart bool) daemonclient.Service {
		if autoStart {
			t.Error("autoStart should be false")
		}
		return mockSvc
	}

	t.Run("list inbox default", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf

		if err := RunList([]string{}); err != nil {
			t.Fatalf("RunList: %v", err)
		}

		if !mockSvc.FetchEmailsCalled {
			t.Error("FetchEmails should have been called")
		}
		if mockSvc.LastAccountID != "acc1" {
			t.Errorf("account = %q, want acc1 (first account by default)", mockSvc.LastAccountID)
		}
		if mockSvc.LastFolder != "INBOX" {
			t.Errorf("folder = %q, want INBOX (default)", mockSvc.LastFolder)
		}
		if out := buf.String(); !strings.Contains(out, "Test Subject 1") || !strings.Contains(out, "sender1@example.com") {
			t.Errorf("output missing expected email fields: %q", out)
		}
	})

	t.Run("list specific folder and account", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf

		if err := RunList([]string{"--from", "another@example.com", "Sent"}); err != nil {
			t.Fatalf("RunList: %v", err)
		}

		if !mockSvc.FetchEmailsCalled {
			t.Error("FetchEmails should have been called")
		}
		if mockSvc.LastAccountID != "acc2" {
			t.Errorf("account = %q, want acc2 (specified)", mockSvc.LastAccountID)
		}
		if mockSvc.LastFolder != "Sent" {
			t.Errorf("folder = %q, want Sent (specified)", mockSvc.LastFolder)
		}
	})

	t.Run("list with --json output", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf

		if err := RunList([]string{"--json"}); err != nil {
			t.Fatalf("RunList: %v", err)
		}

		if !mockSvc.FetchEmailsCalled {
			t.Error("FetchEmails should have been called")
		}

		var emails []map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &emails); err != nil {
			t.Fatalf("output should be valid JSON: %v", err)
		}
		if len(emails) != 2 {
			t.Fatalf("got %d emails, want 2", len(emails))
		}
		if emails[0]["uid"] != float64(101) {
			t.Errorf("uid = %v, want 101", emails[0]["uid"])
		}
		if emails[0]["from"] != "sender1@example.com" {
			t.Errorf("from = %v, want sender1@example.com", emails[0]["from"])
		}
		if emails[0]["subject"] != "Test Subject 1" {
			t.Errorf("subject = %v, want Test Subject 1", emails[0]["subject"])
		}
		if emails[0]["date"] != "2026-07-06T10:00:00Z" {
			t.Errorf("date = %v, want 2026-07-06T10:00:00Z", emails[0]["date"])
		}
		if emails[0]["is_read"] != false {
			t.Errorf("is_read = %v, want false", emails[0]["is_read"])
		}
	})

	t.Run("account not found", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf

		err := RunList([]string{"--from", "notfound@example.com"})
		if err == nil {
			t.Fatal("expected an error for unknown account")
		}
		if !strings.Contains(err.Error(), `no account found matching "notfound@example.com"`) {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
