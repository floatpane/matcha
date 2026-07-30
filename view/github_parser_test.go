package view

import (
	"testing"
	"time"

	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/github"
)

func TestParseGitHubNotification(t *testing.T) {
	tests := []struct {
		name          string
		email         fetcher.Email
		expectGitHub  bool
		expectOrgName string
		expectRepo    string
		expectTitle   string
		expectNumber  int
	}{
		{
			name: "GitHub PR notification",
			email: fetcher.Email{
				From:    "John Doe <notifications@github.com>",
				To:      []string{"user@example.com"},
				Subject: "[org/repo] Add new feature #123",
				Body:    "John opened this pull request",
				Date:    time.Now(),
			},
			expectGitHub:  true,
			expectOrgName: "org",
			expectRepo:    "repo",
			expectTitle:   "Add new feature",
			expectNumber:  123,
		},
		{
			name: "GitHub repo-specific notification",
			email: fetcher.Email{
				From:    "matcha <matcha@noreply.github.com>",
				To:      []string{"user@example.com"},
				Subject: "[floatpane/matcha] Fix email parsing #456",
				Body:    "Jane commented on this issue",
				Date:    time.Now(),
			},
			expectGitHub:  true,
			expectOrgName: "floatpane",
			expectRepo:    "matcha",
			expectTitle:   "Fix email parsing",
			expectNumber:  456,
		},
		{
			name: "GitHub issue notification",
			email: fetcher.Email{
				From:    "Jane Smith <notifications@github.com>",
				To:      []string{"user@example.com"},
				Subject: "[myorg/myrepo] Bug in login flow #456",
				Body:    "Jane commented on this issue",
				Date:    time.Now(),
			},
			expectGitHub:  true,
			expectOrgName: "myorg",
			expectRepo:    "myrepo",
			expectTitle:   "Bug in login flow",
			expectNumber:  456,
		},
		{
			name: "Non-GitHub email",
			email: fetcher.Email{
				From:    "Someone <someone@example.com>",
				To:      []string{"user@example.com"},
				Subject: "Regular email",
				Body:    "This is not from GitHub",
				Date:    time.Now(),
			},
			expectGitHub: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseGitHubNotification(tt.email)
			if tt.expectGitHub && result == nil {
				t.Fatal("expected GitHub notification but got nil")
			}
			if !tt.expectGitHub && result != nil {
				t.Fatal("expected nil but got GitHub notification")
			}
			if result != nil {
				if result.Key.OrgName != tt.expectOrgName {
					t.Errorf("expected org %q, got %q", tt.expectOrgName, result.Key.OrgName)
				}
				if result.Key.RepoName != tt.expectRepo {
					t.Errorf("expected repo %q, got %q", tt.expectRepo, result.Key.RepoName)
				}
				if result.Title != tt.expectTitle {
					t.Errorf("expected title %q, got %q", tt.expectTitle, result.Title)
				}
				if result.Key.IssueNumber != tt.expectNumber {
					t.Errorf("expected number %d, got %d", tt.expectNumber, result.Key.IssueNumber)
				}
			}
		})
	}
}

func TestIsGitHubEmail(t *testing.T) {
	tests := []struct {
		name   string
		email  fetcher.Email
		expect bool
	}{
		{
			name: "From notifications@github.com",
			email: fetcher.Email{
				From: "GitHub <notifications@github.com>",
			},
			expect: true,
		},
		{
			name: "To notifications@github.com",
			email: fetcher.Email{
				To: []string{"notifications@github.com"},
			},
			expect: true,
		},
		{
			name: "From repo@no-reply.github.com",
			email: fetcher.Email{
				From: "matcha <matcha@no-reply.github.com>",
			},
			expect: true,
		},
		{
			name: "To repo@no-reply.github.com",
			email: fetcher.Email{
				To: []string{"myrepo@no-reply.github.com"},
			},
			expect: true,
		},
		{
			name: "To repo@noreply.github.com (no hyphen)",
			email: fetcher.Email{
				To: []string{"matcha@noreply.github.com"},
			},
			expect: true,
		},
		{
			name: "Regular email",
			email: fetcher.Email{
				From: "User <user@example.com>",
				To:   []string{"recipient@example.com"},
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGitHubEmail(tt.email)
			if result != tt.expect {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestExtractBodyContent(t *testing.T) {
	email := fetcher.Email{
		Body: `John commented:

This is a great feature!

---
Reply to this email directly
View it on GitHub
Unsubscribe`,
	}

	result := extractBodyContent(email)

	expected := "John commented:\n\nThis is a great feature!"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestDetermineEventType(t *testing.T) {
	tests := []struct {
		name          string
		subject       string
		body          string
		expected      github.EventType
		expectedState string
		isPR          bool
	}{
		{
			name:          "Closed PR",
			subject:       "[org/repo] PR closed #123",
			body:          "",
			expected:      github.EventClosed,
			expectedState: "closed",
		},
		{
			name:          "Merged PR",
			subject:       "[org/repo] PR merged #123",
			body:          "",
			expected:      github.EventMerged,
			expectedState: "merged",
			isPR:          true,
		},
		{
			name:          "Opened PR",
			subject:       "[org/repo] Pull request #123",
			body:          "",
			expected:      github.EventOpened,
			expectedState: "open",
			isPR:          true,
		},
		{
			name:          "Opened issue",
			subject:       "[org/repo] Issue #123",
			body:          "",
			expected:      github.EventOpened,
			expectedState: "open",
		},
		{
			name:     "Comment",
			subject:  "[org/repo] Something",
			body:     "John commented on this",
			expected: github.EventCommented,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email := fetcher.Email{
				Subject: tt.subject,
				Body:    tt.body,
			}
			eventType, state := determineEventType(email)
			if eventType != tt.expected {
				t.Errorf("expected event type %v, got %v", tt.expected, eventType)
			}
			if tt.expectedState != "" && state != tt.expectedState {
				t.Errorf("expected state %q, got %q", tt.expectedState, state)
			}
		})
	}
}

func TestGroupingByPR(t *testing.T) {
	key := github.EventKey{
		OrgName:     "floatpane",
		RepoName:    "matcha",
		IssueNumber: 1655,
		IsPR:        true,
	}

	emails := []fetcher.Email{
		{
			From:    "Lea <notifications@github.com>",
			To:      []string{"matcha@noreply.github.com"},
			Subject: "Re: [floatpane/matcha] fix: remove limit for headers (PR #1655)",
			Body:    "LeaWhoCodes left a comment\n\nThis is a demo feature.",
			Date:    time.Now().Add(-2 * time.Hour),
		},
		{
			From:    "Drew <notifications@github.com>",
			To:      []string{"matcha@noreply.github.com"},
			Subject: "Re: [floatpane/matcha] fix: remove limit for headers (PR #1655)",
			Body:    "Drew commented\n\nLooks good to me.",
			Date:    time.Now().Add(-1 * time.Hour),
		},
	}

	for _, email := range emails {
		ParseGitHubNotification(email)
	}

	group := github.GetGroup(key)
	if group == nil {
		t.Fatal("expected group to exist")
	}
	if len(group.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(group.Events))
	}
	if group.Key.IssueNumber != 1655 {
		t.Errorf("expected issue number 1655, got %d", group.Key.IssueNumber)
	}
}
