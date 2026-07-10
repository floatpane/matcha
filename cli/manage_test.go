package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
)

type mockManageService struct {
	daemonclient.Service // embed
	LastAccountID        string
	LastSrcFolder        string
	LastDstFolder        string
	LastUIDs             []uint32
	Called               map[string]bool
}

func (m *mockManageService) Reset() {
	m.Called = make(map[string]bool)
	m.LastUIDs = nil
	m.LastSrcFolder = ""
	m.LastDstFolder = ""
}
func (m *mockManageService) ArchiveEmails(accountID, folder string, uids []uint32) error {
	m.Called["ArchiveEmails"] = true
	m.LastAccountID = accountID
	m.LastSrcFolder = folder
	m.LastUIDs = uids
	return nil
}
func (m *mockManageService) DeleteEmails(accountID, folder string, uids []uint32) error {
	m.Called["DeleteEmails"] = true
	m.LastAccountID = accountID
	m.LastSrcFolder = folder
	m.LastUIDs = uids
	return nil
}
func (m *mockManageService) MarkRead(accountID, folder string, uids []uint32) error {
	m.Called["MarkRead"] = true
	m.LastAccountID = accountID
	m.LastSrcFolder = folder
	m.LastUIDs = uids
	return nil
}
func (m *mockManageService) MarkUnread(accountID, folder string, uids []uint32) error {
	m.Called["MarkUnread"] = true
	m.LastAccountID = accountID
	m.LastSrcFolder = folder
	m.LastUIDs = uids
	return nil
}
func (m *mockManageService) MoveEmails(accountID string, uids []uint32, src, dst string) error {
	m.Called["MoveEmails"] = true
	m.LastAccountID = accountID
	m.LastUIDs = uids
	m.LastSrcFolder = src
	m.LastDstFolder = dst
	return nil
}
func (m *mockManageService) Close() error { return nil }

func TestRunManage(t *testing.T) {
	cfg := &config.Config{
		Accounts: []config.Account{
			{Email: "test@example.com", FetchEmail: "test@example.com", ID: "acc1"},
		},
	}
	mockSvc := &mockManageService{}

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

	testCases := []struct {
		name         string
		args         []string
		expectedCall string
		expectedUIDs []uint32
		expectedSrc  string
		expectedDst  string
		expectedMsg  string
	}{
		{"archive single", []string{"archive", "INBOX", "123"}, "ArchiveEmails", []uint32{123}, "INBOX", "", "Success: Archived 1 email."},
		{"archive multiple", []string{"archive", "INBOX", "1,2,3"}, "ArchiveEmails", []uint32{1, 2, 3}, "INBOX", "", "Success: Archived 3 emails."},
		{"delete", []string{"delete", "Trash", "4,5"}, "DeleteEmails", []uint32{4, 5}, "Trash", "", "Success: Deleted 2 emails."},
		{"mark-read", []string{"mark-read", "INBOX", "6"}, "MarkRead", []uint32{6}, "INBOX", "", "Success: Marked 1 email as read."},
		{"mark-unread", []string{"mark-unread", "INBOX", "7"}, "MarkUnread", []uint32{7}, "INBOX", "", "Success: Marked 1 email as unread."},
		{"move", []string{"move", "INBOX", "Archive", "8,9"}, "MoveEmails", []uint32{8, 9}, "INBOX", "Archive", "Success: Moved 2 emails to Archive."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSvc.Reset()
			var buf bytes.Buffer
			Out = &buf

			if err := RunManage(tc.args); err != nil {
				t.Fatalf("RunManage: %v", err)
			}

			if !mockSvc.Called[tc.expectedCall] {
				t.Errorf("%s should have been called", tc.expectedCall)
			}
			if mockSvc.LastAccountID != "acc1" {
				t.Errorf("account = %q, want acc1", mockSvc.LastAccountID)
			}
			if !slices.Equal(mockSvc.LastUIDs, tc.expectedUIDs) {
				t.Errorf("uids = %v, want %v", mockSvc.LastUIDs, tc.expectedUIDs)
			}
			if mockSvc.LastSrcFolder != tc.expectedSrc {
				t.Errorf("src folder = %q, want %q", mockSvc.LastSrcFolder, tc.expectedSrc)
			}
			if tc.expectedDst != "" && mockSvc.LastDstFolder != tc.expectedDst {
				t.Errorf("dst folder = %q, want %q", mockSvc.LastDstFolder, tc.expectedDst)
			}
			if got := strings.TrimSpace(buf.String()); got != tc.expectedMsg {
				t.Errorf("message = %q, want %q", got, tc.expectedMsg)
			}
		})
	}

	errorCases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"invalid subcommand", []string{"unknown", "INBOX", "123"}, "unknown subcommand"},
		{"invalid uids", []string{"archive", "INBOX", "abc"}, "invalid uid"},
		{"empty uids archive", []string{"archive", "INBOX", ""}, "no valid UIDs provided"},
		{"empty uids move", []string{"move", "INBOX", "Archive", ""}, "no valid UIDs provided"},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			err := RunManage(tc.args)
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}
