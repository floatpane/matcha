package repoapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseHostFromEmailSender(t *testing.T) {
	tests := []struct {
		from string
		want Host
	}{
		{"notifications@github.com", HostGitHub},
		{"noreply@github.com", HostGitHub},
		{"foo@noreply.github.com", HostGitHub},
		{"noreply@gitlab.com", HostGitLab},
		{"reply@mg.gitlab.com", HostGitLab},
		{"someone@example.com", HostUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.from, func(t *testing.T) {
			got := ParseHostFromEmailSender(tc.from)
			if got != tc.want {
				t.Errorf("ParseHostFromEmailSender(%q) = %v, want %v", tc.from, got, tc.want)
			}
		})
	}
}

func TestReviewRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     *ReviewRequest
		wantErr bool
	}{
		{"nil", nil, true},
		{"unknown host", &ReviewRequest{Host: HostUnknown, Token: "x", Owner: "o", Repo: "r", PRNumber: 1}, true},
		{"no token", &ReviewRequest{Host: HostGitHub, Owner: "o", Repo: "r", PRNumber: 1}, true},
		{"no owner", &ReviewRequest{Host: HostGitHub, Token: "x", Repo: "r", PRNumber: 1}, true},
		{"no repo", &ReviewRequest{Host: HostGitHub, Token: "x", Owner: "o", PRNumber: 1}, true},
		{"bad pr number", &ReviewRequest{Host: HostGitHub, Token: "x", Owner: "o", Repo: "r", PRNumber: 0}, true},
		{"valid approve", &ReviewRequest{Host: HostGitHub, Token: "x", Owner: "o", Repo: "r", PRNumber: 1, Event: ReviewApprove, Body: "lgtm"}, false},
		{"line comment no path", &ReviewRequest{Host: HostGitHub, Token: "x", Owner: "o", Repo: "r", PRNumber: 1, Event: ReviewComment, LineComment: &LineCommentTarget{Line: 5}}, true},
		{"line comment no line", &ReviewRequest{Host: HostGitHub, Token: "x", Owner: "o", Repo: "r", PRNumber: 1, Event: ReviewComment, LineComment: &LineCommentTarget{Path: "main.go"}}, true},
		{"valid line comment", &ReviewRequest{Host: HostGitHub, Token: "x", Owner: "o", Repo: "r", PRNumber: 1, Event: ReviewComment, Body: "nit", LineComment: &LineCommentTarget{Path: "main.go", Line: 5}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestPRNumberFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want int
	}{
		{"https://github.com/owner/repo/pull/42", 42},
		{"https://gitlab.com/group/project/-/merge_requests/7", 7},
		{"https://github.com/owner/repo/pulls/100", 100},
		{"https://github.com/owner/repo/issues/5", 0},
		{"not a url", 0},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			got := PRNumberFromURL(tc.url)
			if got != tc.want {
				t.Errorf("PRNumberFromURL(%q) = %d, want %d", tc.url, got, tc.want)
			}
		})
	}
}

func TestSubmitGitHubReview_Approve(t *testing.T) {
	var receivedBody GitHubReviewPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer testtoken" {
			t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("unexpected Accept header: %q", r.Header.Get("Accept"))
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":123}`))
	}))
	defer srv.Close()

	githubAPIBase = srv.URL
	defer func() { githubAPIBase = defaultGitHubAPIBase }()

	c := NewClientWithHTTP(srv.Client())
	req := &ReviewRequest{
		Host:     HostGitHub,
		Token:    "testtoken",
		Owner:    "octocat",
		Repo:     "hello-world",
		PRNumber: 1,
		Event:    ReviewApprove,
		Body:     "Looks good to me!",
	}
	resp, err := c.SubmitReview(req)
	if err != nil {
		t.Fatalf("SubmitReview failed: %v", err)
	}
	if string(resp) != `{"id":123}` {
		t.Errorf("unexpected response: %q", string(resp))
	}
	if receivedBody.Event != "APPROVE" {
		t.Errorf("expected event APPROVE, got %q", receivedBody.Event)
	}
	if receivedBody.Body != "Looks good to me!" {
		t.Errorf("unexpected body: %q", receivedBody.Body)
	}
}

func TestSubmitGitHubReview_LineComment(t *testing.T) {
	var receivedBody GitHubReviewPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":456}`))
	}))
	defer srv.Close()

	githubAPIBase = srv.URL
	defer func() { githubAPIBase = defaultGitHubAPIBase }()

	c := NewClientWithHTTP(srv.Client())
	req := &ReviewRequest{
		Host:     HostGitHub,
		Token:    "tok",
		Owner:    "o",
		Repo:     "r",
		PRNumber: 2,
		Event:    ReviewRequestChanges,
		Body:     "Please fix this line",
		LineComment: &LineCommentTarget{
			Path: "main.go",
			Line: 42,
			Side: "RIGHT",
		},
		CommitSHA: "abc123",
	}
	if _, err := c.SubmitReview(req); err != nil {
		t.Fatalf("SubmitReview failed: %v", err)
	}
	if receivedBody.Event != "REQUEST_CHANGES" {
		t.Errorf("expected REQUEST_CHANGES, got %q", receivedBody.Event)
	}
	if len(receivedBody.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(receivedBody.Comments))
	}
	cmt := receivedBody.Comments[0]
	if cmt.Path != "main.go" || cmt.Line != 42 || cmt.Side != "RIGHT" {
		t.Errorf("unexpected comment: %+v", cmt)
	}
	if cmt.Body != "Please fix this line" {
		t.Errorf("unexpected comment body: %q", cmt.Body)
	}
	if receivedBody.CommitID != "abc123" {
		t.Errorf("unexpected commit_id: %q", receivedBody.CommitID)
	}
}

func TestSubmitReview_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"Validation Failed"}`))
	}))
	defer srv.Close()

	githubAPIBase = srv.URL
	defer func() { githubAPIBase = defaultGitHubAPIBase }()

	c := NewClientWithHTTP(srv.Client())
	req := &ReviewRequest{
		Host:     HostGitHub,
		Token:    "tok",
		Owner:    "o",
		Repo:     "r",
		PRNumber: 1,
		Event:    ReviewApprove,
		Body:     "lgtm",
	}
	_, err := c.SubmitReview(req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", apiErr.StatusCode)
	}
}

func TestSubmitGitHubIssueComment(t *testing.T) {
	var receivedBody GitHubIssueCommentPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &receivedBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":789}`))
	}))
	defer srv.Close()

	githubAPIBase = srv.URL
	defer func() { githubAPIBase = defaultGitHubAPIBase }()

	c := NewClientWithHTTP(srv.Client())
	req := &ReviewRequest{
		Host:     HostGitHub,
		Token:    "tok",
		Owner:    "o",
		Repo:     "r",
		PRNumber: 5,
		Event:    ReviewComment,
		Body:     "General comment here",
	}
	if _, err := c.SubmitGitHubIssueComment(req); err != nil {
		t.Fatalf("SubmitGitHubIssueComment failed: %v", err)
	}
	if receivedBody.Body != "General comment here" {
		t.Errorf("unexpected body: %q", receivedBody.Body)
	}
}

func TestSubmitGitLabReview_Approve(t *testing.T) {
	var receivedBody GitLabReviewPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/approve") {
			t.Errorf("expected /approve in path, got %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	gitlabAPIBase = srv.URL
	defer func() { gitlabAPIBase = defaultGitLabAPIBase }()

	c := NewClientWithHTTP(srv.Client())
	req := &ReviewRequest{
		Host:      HostGitLab,
		Token:     "tok",
		Owner:     "group",
		Repo:      "project",
		PRNumber:  3,
		Event:     ReviewApprove,
		Body:      "Good work",
		CommitSHA: "sha123",
	}
	if _, err := c.SubmitReview(req); err != nil {
		t.Fatalf("SubmitReview failed: %v", err)
	}
	if receivedBody.SHA != "sha123" {
		t.Errorf("expected sha sha123, got %q", receivedBody.SHA)
	}
}

func TestUrlEncodeProjectID(t *testing.T) {
	got := urlEncodeProjectID("mygroup", "myrepo")
	if got != "mygroup%2Fmyrepo" {
		t.Errorf("expected mygroup%%2Fmyrepo, got %q", got)
	}
}
