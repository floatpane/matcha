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

type mockReadService struct {
	daemonclient.Service // embed
	FetchEmailBodyCalled bool
	LastAccountID        string
	LastFolder           string
	LastUID              uint32
	BodyToReturn         string
	MIMEToReturn         string
	AttachmentsToReturn  []backend.Attachment
	ErrorToReturn        error
}

func (m *mockReadService) FetchEmailBody(accountID, folderName string, uid uint32) (string, string, []backend.Attachment, error) {
	m.FetchEmailBodyCalled = true
	m.LastAccountID = accountID
	m.LastFolder = folderName
	m.LastUID = uid
	return m.BodyToReturn, m.MIMEToReturn, m.AttachmentsToReturn, m.ErrorToReturn
}

func (m *mockReadService) Close() error { return nil }
func (m *mockReadService) Reset() {
	m.FetchEmailBodyCalled = false
	m.LastFolder = ""
	m.LastUID = 0
}

func TestRunRead(t *testing.T) {
	cfg := &config.Config{
		Accounts: []config.Account{
			{Email: "test@example.com", FetchEmail: "test@example.com", ID: "acc1"},
		},
	}
	mockSvc := &mockReadService{
		BodyToReturn: "This is the plain text body.",
		MIMEToReturn: "text/plain",
		AttachmentsToReturn: []backend.Attachment{
			{Filename: "att1.txt"},
		},
	}

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

	t.Run("read email plain text", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf
		if err := RunRead([]string{"INBOX", "12345"}); err != nil {
			t.Fatalf("RunRead: %v", err)
		}

		if !mockSvc.FetchEmailBodyCalled {
			t.Error("FetchEmailBody should have been called")
		}
		if mockSvc.LastAccountID != "acc1" {
			t.Errorf("account = %q, want acc1", mockSvc.LastAccountID)
		}
		if mockSvc.LastFolder != "INBOX" {
			t.Errorf("folder = %q, want INBOX", mockSvc.LastFolder)
		}
		if mockSvc.LastUID != 12345 {
			t.Errorf("uid = %d, want 12345", mockSvc.LastUID)
		}
		if buf.String() != "This is the plain text body." {
			t.Errorf("body = %q", buf.String())
		}
	})

	t.Run("read email with json output", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf
		if err := RunRead([]string{"--json", "INBOX", "12345"}); err != nil {
			t.Fatalf("RunRead: %v", err)
		}

		if !mockSvc.FetchEmailBodyCalled {
			t.Error("FetchEmailBody should have been called")
		}

		var bodyData map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &bodyData); err != nil {
			t.Fatalf("output should be valid JSON: %v", err)
		}
		if bodyData["body"] != "This is the plain text body." {
			t.Errorf("body = %v", bodyData["body"])
		}
		if bodyData["mime_type"] != "text/plain" {
			t.Errorf("mime_type = %v", bodyData["mime_type"])
		}
		attachments, ok := bodyData["attachments"].([]interface{})
		if !ok {
			t.Fatalf("attachments not an array: %v", bodyData["attachments"])
		}
		if len(attachments) != 1 {
			t.Fatalf("got %d attachments, want 1", len(attachments))
		}
		att, ok := attachments[0].(map[string]interface{})
		if !ok {
			t.Fatalf("attachment not an object: %v", attachments[0])
		}
		if att["filename"] != "att1.txt" {
			t.Errorf("filename = %v, want att1.txt", att["filename"])
		}
	})

	t.Run("read email missing uid", func(t *testing.T) {
		mockSvc.Reset()
		err := RunRead([]string{"INBOX"})
		if err == nil {
			t.Fatal("expected an error when uid is missing")
		}
		if !strings.Contains(err.Error(), "folder and uid are required") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("read email with default folder INBOX", func(t *testing.T) {
		mockSvc.Reset()
		var buf bytes.Buffer
		Out = &buf
		if err := RunRead([]string{"12345"}); err != nil {
			t.Fatalf("RunRead: %v", err)
		}

		if !mockSvc.FetchEmailBodyCalled {
			t.Error("FetchEmailBody should have been called")
		}
		if mockSvc.LastAccountID != "acc1" {
			t.Errorf("account = %q, want acc1", mockSvc.LastAccountID)
		}
		if mockSvc.LastFolder != "INBOX" {
			t.Errorf("folder = %q, want INBOX (default)", mockSvc.LastFolder)
		}
		if mockSvc.LastUID != 12345 {
			t.Errorf("uid = %d, want 12345", mockSvc.LastUID)
		}
		if buf.String() != "This is the plain text body." {
			t.Errorf("body = %q", buf.String())
		}
	})

	t.Run("read email with no attachments json output omitempty", func(t *testing.T) {
		mockSvc.Reset()
		mockSvc.AttachmentsToReturn = nil
		defer func() { mockSvc.AttachmentsToReturn = []backend.Attachment{{Filename: "att1.txt"}} }()
		var buf bytes.Buffer
		Out = &buf
		if err := RunRead([]string{"--json", "INBOX", "12345"}); err != nil {
			t.Fatalf("RunRead: %v", err)
		}

		if !mockSvc.FetchEmailBodyCalled {
			t.Error("FetchEmailBody should have been called")
		}

		var bodyData map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &bodyData); err != nil {
			t.Fatalf("output should be valid JSON: %v", err)
		}
		if bodyData["body"] != "This is the plain text body." {
			t.Errorf("body = %v", bodyData["body"])
		}
		if bodyData["mime_type"] != "text/plain" {
			t.Errorf("mime_type = %v", bodyData["mime_type"])
		}
		if _, ok := bodyData["attachments"]; ok {
			t.Error("attachments key should be omitted when empty")
		}
	})
}
