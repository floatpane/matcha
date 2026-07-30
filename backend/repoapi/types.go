// Package repoapi provides a unified client for submitting pull-request
// reviews and line-level comments to GitHub and GitLab repositories.
//
// The email parsing layer extracts the repository owner, repo name, PR
// number, and optional line/file context from notification emails. This
// package turns that context into the appropriate HTTP API calls.
package repoapi

import (
	"errors"
	"fmt"
	"strings"
)

// Host identifies the repository hosting platform.
type Host int

const (
	HostUnknown Host = iota
	HostGitHub
	HostGitLab
)

// String returns the human-readable name of the host.
func (h Host) String() string {
	switch h {
	case HostGitHub:
		return "github"
	case HostGitLab:
		return "gitlab"
	default:
		return "unknown"
	}
}

// ReviewEvent corresponds to the GitHub "event" field in a review submission.
// GitLab maps these to its own state names via reviewEventToGitLab.
type ReviewEvent string

const (
	// ReviewApprove submits an approving review.
	ReviewApprove ReviewEvent = "APPROVE"
	// ReviewRequestChanges submits a "changes requested" review.
	ReviewRequestChanges ReviewEvent = "REQUEST_CHANGES"
	// ReviewComment submits a general comment without approving or rejecting.
	ReviewComment ReviewEvent = "COMMENT"
)

// LineCommentTarget describes a specific line of code that a comment targets.
type LineCommentTarget struct {
	Path      string // file path relative to repo root
	Line      int    // line number in the PR diff (new/after side)
	Side      string // GitHub: "RIGHT" or "LEFT"; GitLab: ignored (uses new position)
	StartLine int    // optional multi-line comment start; 0 for single-line
}

// ReviewRequest is the unified, host-agnostic description of a PR review
// action. The caller fills this from parsed email context and user input.
type ReviewRequest struct {
	Host        Host
	Token       string             // OAuth2 access token or personal access token
	Owner       string             // GitHub: owner; GitLab: namespace (user/group)
	Repo        string             // GitHub: repo; GitLab: project path within namespace
	PRNumber    int                // PR/MR number
	Event       ReviewEvent        // approve / request_changes / comment
	Body        string             // markdown summary body for the review
	LineComment *LineCommentTarget // optional line-level comment; nil for review-only
	CommitSHA   string             // optional: commit SHA for line comment anchoring
}

// Validate checks that the request has the minimum required fields for the
// given host. It returns nil if the request is usable.
func (r *ReviewRequest) Validate() error {
	if r == nil {
		return errors.New("review request is nil")
	}
	if r.Host == HostUnknown {
		return errors.New("repo host not determined")
	}
	if r.Token == "" {
		return errors.New("no API token: set GITHUB_TOKEN or configure OAuth")
	}
	if r.Owner == "" || r.Repo == "" {
		return errors.New("repository owner/name not parsed from email")
	}
	if r.PRNumber <= 0 {
		return fmt.Errorf("invalid PR number: %d", r.PRNumber)
	}
	if r.LineComment != nil {
		if r.LineComment.Path == "" {
			return errors.New("line comment missing file path")
		}
		if r.LineComment.Line <= 0 {
			return fmt.Errorf("line comment has invalid line number: %d", r.LineComment.Line)
		}
	}
	return nil
}

// GitHubReviewPayload is the JSON body for
// POST /repos/{owner}/{repo}/pulls/{pr}/reviews.
type GitHubReviewPayload struct {
	CommitID string                `json:"commit_id,omitempty"`
	Body     string                `json:"body"`
	Event    string                `json:"event"`
	Comments []GitHubReviewComment `json:"comments,omitempty"`
}

// GitHubReviewComment is a single line-level comment in a GitHub review.
type GitHubReviewComment struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
	Body      string `json:"body"`
}

// GitHubIssueCommentPayload is the JSON body for
// POST /repos/{owner}/{repo}/issues/{number}/comments (general PR comment).
type GitHubIssueCommentPayload struct {
	Body string `json:"body"`
}

// GitLabReviewPayload is the JSON body for
// POST /projects/{id}/merge_requests/{iid}/approve or /notes.
type GitLabReviewPayload struct {
	Body string `json:"body,omitempty"`
	// SHA is required by the approve endpoint when present.
	SHA string `json:"sha,omitempty"`
}

// GitLabDiscussionPayload is the JSON body for
// POST /projects/{id}/merge_requests/{iid}/discussions (line comment).
type GitLabDiscussionPayload struct {
	Body     string         `json:"body"`
	Position GitLabPosition `json:"position"`
}

// GitLabPosition describes the diff anchor for a GitLab line comment.
type GitLabPosition struct {
	BaseSHA      string `json:"base_sha"`
	HeadSHA      string `json:"head_sha"`
	StartSHA     string `json:"start_sha"`
	NewPath      string `json:"new_path"`
	NewLine      int    `json:"new_line"`
	PositionType string `json:"position_type"` // always "text"
}

// APIError holds an error message returned by the repository API.
type APIError struct {
	Host       Host
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s API returned %d: %s", e.Host, e.StatusCode, e.Message)
}

// ParseHostFromEmailSender determines whether an email originated from GitHub
// or GitLab based on the sender address. Returns HostUnknown if unrecognized.
func ParseHostFromEmailSender(from string) Host {
	from = strings.ToLower(from)
	switch {
	case strings.Contains(from, "notifications@github.com"),
		strings.Contains(from, "noreply.github.com"),
		strings.Contains(from, "@github.com"):
		return HostGitHub
	case strings.Contains(from, "gitlab"),
		strings.Contains(from, "@gitlab.com"):
		return HostGitLab
	}
	return HostUnknown
}

// reviewEventToGitLab maps a unified ReviewEvent to the GitLab approval state.
// GitLab has no explicit "request changes"; unapproving with a comment serves
// the same purpose.
func reviewEventToGitLab(event ReviewEvent) string {
	switch event {
	case ReviewApprove:
		return "approve"
	case ReviewRequestChanges:
		return "unapprove"
	default:
		return "comment"
	}
}
