package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
)

type mockSearchService struct {
	daemonclient.Service // embed
	SearchCalled         bool
	LastAccountID        string
	LastFolder           string
	LastQuery            backend.SearchQuery
	ResultsToReturn      []backend.Email
	ErrorToReturn        error
}

func (m *mockSearchService) Search(accountID, folder string, query backend.SearchQuery) ([]backend.Email, error) {
	m.SearchCalled = true
	m.LastAccountID = accountID
	m.LastFolder = folder
	m.LastQuery = query
	return m.ResultsToReturn, m.ErrorToReturn
}
func (m *mockSearchService) Close() error { return nil }
func (m *mockSearchService) Reset() {
	m.SearchCalled = false
	m.LastFolder = ""
	m.LastQuery = backend.SearchQuery{}
}

func TestRunSearch(t *testing.T) {
	cfg := &config.Config{
		Accounts: []config.Account{
			{Email: "test@example.com", FetchEmail: "test@example.com", ID: "acc1"},
		},
	}
	mockSvc := &mockSearchService{ResultsToReturn: mockEmails} // using mockEmails from list_test

	originalLoadConfig := configLoadConfig
	originalNewServiceFunc := NewServiceFunc
	originalOut := Out
	defer func() {
		configLoadConfig = originalLoadConfig
		NewServiceFunc = originalNewServiceFunc
		Out = originalOut
	}()
	configLoadConfig = func() (*config.Config, error) { return cfg, nil }
	NewServiceFunc = func(cfg *config.Config, autoStart bool) daemonclient.Service {
		return mockSvc
	}

	t.Run("search with text output", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf
		if err := RunSearch([]string{"INBOX", "important"}); err != nil {
			t.Fatalf("RunSearch: %v", err)
		}

		if !mockSvc.SearchCalled {
			t.Error("Search should have been called")
		}
		if mockSvc.LastAccountID != "acc1" {
			t.Errorf("account = %q, want acc1", mockSvc.LastAccountID)
		}
		if mockSvc.LastFolder != "INBOX" {
			t.Errorf("folder = %q, want INBOX", mockSvc.LastFolder)
		}
		// A bare term with no operators is parsed into the Body field.
		if mockSvc.LastQuery.Body != "important" {
			t.Errorf("query body = %q, want important", mockSvc.LastQuery.Body)
		}
		if !strings.Contains(buf.String(), "Test Subject 1") {
			t.Errorf("output missing expected result: %q", buf.String())
		}
	})

	t.Run("search with json output", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf
		if err := RunSearch([]string{"--json", "INBOX", "important"}); err != nil {
			t.Fatalf("RunSearch: %v", err)
		}

		if !mockSvc.SearchCalled {
			t.Error("Search should have been called")
		}

		var emails []ListJSON
		if err := json.Unmarshal(buf.Bytes(), &emails); err != nil {
			t.Fatalf("output should be valid JSON: %v", err)
		}
		if len(emails) != 2 {
			t.Fatalf("got %d emails, want 2", len(emails))
		}
		if emails[0].Subject != "Test Subject 1" {
			t.Errorf("subject = %q, want Test Subject 1", emails[0].Subject)
		}
	})

	t.Run("search missing query", func(t *testing.T) {
		mockSvc.Reset()
		err := RunSearch([]string{"INBOX"})
		if err == nil {
			t.Fatal("expected an error when query is missing")
		}
		if !strings.Contains(err.Error(), "folder and query_string are required") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("search with structured query", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf
		if err := RunSearch([]string{"INBOX", "from:alice@example.com subject:report"}); err != nil {
			t.Fatalf("RunSearch: %v", err)
		}

		if !mockSvc.SearchCalled {
			t.Error("Search should have been called")
		}
		if mockSvc.LastQuery.From != "alice@example.com" {
			t.Errorf("query from = %q, want alice@example.com", mockSvc.LastQuery.From)
		}
		if mockSvc.LastQuery.Subject != "report" {
			t.Errorf("query subject = %q, want report", mockSvc.LastQuery.Subject)
		}
	})

	t.Run("search joins unquoted multi-term query", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf
		// Unquoted operators arrive as separate positionals; they must be
		// joined rather than silently truncated to the first token.
		if err := RunSearch([]string{"INBOX", "from:alice@example.com", "subject:report"}); err != nil {
			t.Fatalf("RunSearch: %v", err)
		}
		if mockSvc.LastQuery.From != "alice@example.com" {
			t.Errorf("query from = %q, want alice@example.com", mockSvc.LastQuery.From)
		}
		if mockSvc.LastQuery.Subject != "report" {
			t.Errorf("query subject = %q, want report", mockSvc.LastQuery.Subject)
		}
	})
}
