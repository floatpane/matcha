package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
)

type mockService struct {
	daemonclient.Service // embed to satisfy interface without implementing all methods
	folders              []backend.Folder
	err                  error
}

func (m *mockService) FetchFolders(accountID string) ([]backend.Folder, error) {
	return m.folders, m.err
}

func (m *mockService) Close() error {
	return nil
}

func TestRunFolders_TextOutput(t *testing.T) {
	// Setup mock config loading
	oldLoadConfig := configLoadConfig
	defer func() { configLoadConfig = oldLoadConfig }()

	configLoadConfig = func() (*config.Config, error) {
		return &config.Config{
			Accounts: []config.Account{
				{ID: "acct-1", Email: "test@example.com"},
			},
		}, nil
	}

	// Mock Service function
	oldNewService := NewServiceFunc
	defer func() { NewServiceFunc = oldNewService }()

	mockSvc := &mockService{
		folders: []backend.Folder{
			{Name: "INBOX"},
			{Name: "Sent"},
		},
	}
	NewServiceFunc = func(cfg *config.Config, autoStart bool) daemonclient.Service {
		return mockSvc
	}

	// Mock Output
	var buf bytes.Buffer
	oldOut := Out
	Out = &buf
	defer func() { Out = oldOut }()

	err := RunFolders([]string{})
	if err != nil {
		t.Fatalf("RunFolders failed: %v", err)
	}

	output := buf.String()
	if !containsSubstr(output, "INBOX") || !containsSubstr(output, "Sent") {
		t.Errorf("expected INBOX and Sent in output, got: %q", output)
	}
}

func TestRunFolders_JSONOutput(t *testing.T) {
	// Setup mock config loading
	oldLoadConfig := configLoadConfig
	defer func() { configLoadConfig = oldLoadConfig }()

	configLoadConfig = func() (*config.Config, error) {
		return &config.Config{
			Accounts: []config.Account{
				{ID: "acct-1", Email: "test@example.com"},
			},
		}, nil
	}

	// Mock Service function
	oldNewService := NewServiceFunc
	defer func() { NewServiceFunc = oldNewService }()

	mockSvc := &mockService{
		folders: []backend.Folder{
			{Name: "INBOX"},
			{Name: "Sent"},
		},
	}
	NewServiceFunc = func(cfg *config.Config, autoStart bool) daemonclient.Service {
		return mockSvc
	}

	// Mock Output
	var buf bytes.Buffer
	oldOut := Out
	Out = &buf
	defer func() { Out = oldOut }()

	err := RunFolders([]string{"--json"})
	if err != nil {
		t.Fatalf("RunFolders failed: %v", err)
	}

	var folders []FolderJSON
	if err := json.Unmarshal(buf.Bytes(), &folders); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}

	if len(folders) != 2 || folders[0].Name != "INBOX" || folders[1].Name != "Sent" {
		t.Errorf("unexpected JSON folders output: %+v", folders)
	}
}

func containsSubstr(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

func TestRunFolders_EmptyJSONOutput(t *testing.T) {
	// Setup mock config loading
	oldLoadConfig := configLoadConfig
	defer func() { configLoadConfig = oldLoadConfig }()

	configLoadConfig = func() (*config.Config, error) {
		return &config.Config{
			Accounts: []config.Account{
				{ID: "acct-1", Email: "test@example.com"},
			},
		}, nil
	}

	// Mock Service function
	oldNewService := NewServiceFunc
	defer func() { NewServiceFunc = oldNewService }()

	mockSvc := &mockService{
		folders: []backend.Folder{},
	}
	NewServiceFunc = func(cfg *config.Config, autoStart bool) daemonclient.Service {
		return mockSvc
	}

	// Mock Output
	var buf bytes.Buffer
	oldOut := Out
	Out = &buf
	defer func() { Out = oldOut }()

	err := RunFolders([]string{"--json"})
	if err != nil {
		t.Fatalf("RunFolders failed: %v", err)
	}

	trimmedOutput := bytes.TrimSpace(buf.Bytes())
	if string(trimmedOutput) != "[]" {
		t.Errorf("expected empty JSON array [], got: %q", string(trimmedOutput))
	}
}
