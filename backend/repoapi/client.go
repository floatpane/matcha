package repoapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/floatpane/matcha/internal/httpclient"
)

const (
	// defaultGitHubAPIBase is the standard GitHub REST API root.
	defaultGitHubAPIBase = "https://api.github.com"
	// defaultGitLabAPIBase is the standard GitLab REST API root.
	defaultGitLabAPIBase = "https://gitlab.com/api/v4"
)

var (
	// githubAPIBase and gitlabAPIBase are var so tests can point them
	// at httptest servers. They default to the real API roots.
	githubAPIBase = defaultGitHubAPIBase
	gitlabAPIBase = defaultGitLabAPIBase
)

// Client sends review and comment requests to GitHub or GitLab.
// It uses the httpclient package for consistent timeout behaviour.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a repo API client with the standard timeout.
func NewClient() *Client {
	return &Client{httpClient: httpclient.New(httpclient.IMAPBatchActionTimeout)}
}

// NewClientWithHTTP allows injecting a custom http.Client (used in tests).
func NewClientWithHTTP(hc *http.Client) *Client {
	return &Client{httpClient: hc}
}

// SubmitReview dispatches the review request to the appropriate platform API.
// It returns the raw response body on success.
func (c *Client) SubmitReview(req *ReviewRequest) ([]byte, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	switch req.Host {
	case HostGitHub:
		return c.submitGitHubReview(req)
	case HostGitLab:
		return c.submitGitLabReview(req)
	default:
		return nil, fmt.Errorf("unsupported host: %s", req.Host)
	}
}

// --- GitHub ---

func (c *Client) submitGitHubReview(req *ReviewRequest) ([]byte, error) {
	payload := GitHubReviewPayload{
		CommitID: req.CommitSHA,
		Body:     req.Body,
		Event:    string(req.Event),
	}

	if req.LineComment != nil {
		comment := GitHubReviewComment{
			Path: req.LineComment.Path,
			Line: req.LineComment.Line,
			Body: req.Body,
		}
		if req.LineComment.Side != "" {
			comment.Side = req.LineComment.Side
		} else {
			comment.Side = "RIGHT"
		}
		if req.LineComment.StartLine > 0 {
			comment.StartLine = req.LineComment.StartLine
			comment.StartSide = comment.Side
		}
		payload.Comments = []GitHubReviewComment{comment}
	}

	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews",
		githubAPIBase, req.Owner, req.Repo, req.PRNumber)
	return c.doPost(url, req.Token, "Bearer", "application/vnd.github.v3+json", payload)
}

// SubmitGitHubIssueComment posts a general (non-line) comment to a PR.
// This uses the issues comments endpoint, which works for PRs too.
func (c *Client) SubmitGitHubIssueComment(req *ReviewRequest) ([]byte, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.Host != HostGitHub {
		return nil, fmt.Errorf("issue comment only supported for GitHub, got %s", req.Host)
	}
	payload := GitHubIssueCommentPayload{Body: req.Body}
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments",
		githubAPIBase, req.Owner, req.Repo, req.PRNumber)
	return c.doPost(url, req.Token, "Bearer", "application/vnd.github.v3+json", payload)
}

// --- GitLab ---

func (c *Client) submitGitLabReview(req *ReviewRequest) ([]byte, error) {
	projectID := urlEncodeProjectID(req.Owner, req.Repo)

	// For line-level comments, use the discussions endpoint.
	if req.LineComment != nil {
		return c.submitGitLabLineComment(req, projectID)
	}

	// For approval or general comment, use the appropriate endpoint.
	switch req.Event {
	case ReviewApprove:
		url := fmt.Sprintf("%s/projects/%s/merge_requests/%d/approve",
			gitlabAPIBase, projectID, req.PRNumber)
		payload := GitLabReviewPayload{SHA: req.CommitSHA}
		return c.doPost(url, req.Token, "Bearer", "application/json", payload)

	case ReviewRequestChanges:
		url := fmt.Sprintf("%s/projects/%s/merge_requests/%d/unapprove",
			gitlabAPIBase, projectID, req.PRNumber)
		return c.doPost(url, req.Token, "Bearer", "application/json", nil)

	default: // ReviewComment
		url := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes",
			gitlabAPIBase, projectID, req.PRNumber)
		payload := GitLabReviewPayload{Body: req.Body}
		return c.doPost(url, req.Token, "Bearer", "application/json", payload)
	}
}

func (c *Client) submitGitLabLineComment(req *ReviewRequest, projectID string) ([]byte, error) {
	if req.CommitSHA == "" {
		return nil, fmt.Errorf("GitLab line comments require a commit SHA")
	}
	pos := GitLabPosition{
		HeadSHA:      req.CommitSHA,
		NewPath:      req.LineComment.Path,
		NewLine:      req.LineComment.Line,
		PositionType: "text",
	}
	payload := GitLabDiscussionPayload{
		Body:     req.Body,
		Position: pos,
	}
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions",
		gitlabAPIBase, projectID, req.PRNumber)
	return c.doPost(url, req.Token, "Bearer", "application/json", payload)
}

// --- HTTP helper ---

// doPost sends a JSON POST request with auth and returns the response body.
func (c *Client) doPost(url, token, authScheme, accept string, payload interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	httpReq, err := http.NewRequest(http.MethodPost, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Accept", accept)
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", authScheme+" "+token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("reading response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			Host:       parseHostFromURL(url),
			StatusCode: resp.StatusCode,
			Message:    truncateMessage(string(respBody), 500),
		}
	}

	return respBody, nil
}

// urlEncodeProjectID builds the URL-encoded "namespace/project" ID used by
// the GitLab API. e.g. ("mygroup", "myrepo") → "mygroup%2Fmyrepo".
func urlEncodeProjectID(owner, repo string) string {
	return strings.ReplaceAll(owner+"/"+repo, "/", "%2F")
}

func parseHostFromURL(url string) Host {
	switch {
	case strings.Contains(url, "api.github.com"):
		return HostGitHub
	case strings.Contains(url, "gitlab.com"):
		return HostGitLab
	default:
		return HostUnknown
	}
}

func truncateMessage(msg string, max int) string {
	if len(msg) <= max {
		return msg
	}
	return msg[:max] + "…"
}

// PRNumberFromURL extracts the PR/MR number from a GitHub or GitLab URL.
// Returns 0 if the number cannot be parsed.
func PRNumberFromURL(url string) int {
	url = strings.TrimRight(url, "/")
	parts := strings.Split(url, "/")
	for i, p := range parts {
		if (p == "pull" || p == "pulls" || p == "merge_requests") && i+1 < len(parts) {
			n, err := strconv.Atoi(parts[i+1])
			if err == nil {
				return n
			}
		}
	}
	return 0
}
