package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/floatpane/matcha/internal/httpclient"
)

const apiTimeout = 10 * time.Second

type PRDetails struct {
	Number         int          `json:"number"`
	Title          string       `json:"title"`
	State          string       `json:"state"`
	Body           string       `json:"body"`
	User           GitHubUser   `json:"user"`
	HTMLURL        string       `json:"html_url"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	Merged         bool         `json:"merged"`
	Mergeable      *bool        `json:"mergeable"`
	Head           BranchRef    `json:"head"`
	Base           BranchRef    `json:"base"`
	Additions      int          `json:"additions"`
	Deletions      int          `json:"deletions"`
	ChangedFiles   int          `json:"changed_files"`
	Commits        int          `json:"commits"`
	Comments       int          `json:"comments"`
	ReviewComments int          `json:"review_comments"`
	Draft          bool         `json:"draft"`
	Labels         []Label      `json:"labels"`
	Assignees      []GitHubUser `json:"assignees"`
}

type GitHubUser struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

type BranchRef struct {
	Ref  string     `json:"ref"`
	Sha  string     `json:"sha"`
	Repo Repository `json:"repo"`
}

type Repository struct {
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Client struct {
	token string
}

func NewClient() *Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = tokenFromGHCLI()
	}
	return &Client{token: token}
}

// tokenFromGHCLI runs `gh auth token` to retrieve the GitHub OAuth token
// stored by the GitHub CLI. Returns "" if gh is not installed or not authed.
func tokenFromGHCLI() string {
	out, err := exec.Command("gh", "auth", "token").Output() //nolint:noctx
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func NewClientWithToken(token string) *Client {
	return &Client{token: token}
}

func (c *Client) FetchPRDetails(owner, repo string, number int) (*PRDetails, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, number)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	client := httpclient.New(apiTimeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var details PRDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, err
	}
	return &details, nil
}

func (c *Client) FetchIssueDetails(owner, repo string, number int) (*IssueDetails, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, number)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	client := httpclient.New(apiTimeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var details IssueDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, err
	}
	return &details, nil
}

type IssueDetails struct {
	Number    int          `json:"number"`
	Title     string       `json:"title"`
	State     string       `json:"state"`
	Body      string       `json:"body"`
	User      GitHubUser   `json:"user"`
	HTMLURL   string       `json:"html_url"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Labels    []Label      `json:"labels"`
	Assignees []GitHubUser `json:"assignees"`
	Comments  int          `json:"comments"`
}
