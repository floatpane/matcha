package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/backend/repoapi"
	"github.com/floatpane/matcha/internal/httpclient"
	"github.com/floatpane/matcha/tui"
)

// prActionLabel returns a human-readable label for the given review event,
// used as a header comment in the editor template.
func prActionLabel(action repoapi.ReviewEvent) string {
	switch action {
	case repoapi.ReviewApprove:
		return "Approve PR"
	case repoapi.ReviewRequestChanges:
		return "Request Changes"
	default:
		return "Leave Comment"
	}
}

// openPREditor writes a markdown template to a temp file, opens $EDITOR, and
// returns a tea.Cmd that produces a PREditorFinishedMsg with the user's input.
func openPREditor(msg tui.PREditorOpenMsg) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	var template strings.Builder
	template.WriteString(fmt.Sprintf("<!-- %s on %s/%s#%d -->\n",
		prActionLabel(msg.Action), msg.Owner, msg.Repo, msg.PRNumber))
	template.WriteString("<!-- Write your markdown below. Save and quit to submit. -->\n")
	if msg.LineTarget != nil {
		template.WriteString(fmt.Sprintf("<!-- Line comment on %s:%d -->\n",
			msg.LineTarget.Path, msg.LineTarget.Line))
	}
	template.WriteString("\n")

	tmpFile, err := os.CreateTemp("", "matcha-pr-*.md")
	if err != nil {
		return func() tea.Msg {
			return tui.PREditorFinishedMsg{
				Action: msg.Action,
				Err:    fmt.Errorf("creating temp file: %w", err),
			}
		}
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(template.String()); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return func() tea.Msg {
			return tui.PREditorFinishedMsg{
				Action: msg.Action,
				Err:    fmt.Errorf("writing temp file: %w", err),
			}
		}
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return func() tea.Msg {
			return tui.PREditorFinishedMsg{
				Action: msg.Action,
				Err:    fmt.Errorf("closing temp file: %w", err),
			}
		}
	}

	parts := strings.Fields(editor)
	args := append(parts[1:], tmpPath)   //nolint:gocritic
	c := exec.Command(parts[0], args...) //nolint:noctx
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer func() {
			_ = os.Remove(tmpPath)
		}()
		result := tui.PREditorFinishedMsg{
			Action:     msg.Action,
			Owner:      msg.Owner,
			Repo:       msg.Repo,
			PRNumber:   msg.PRNumber,
			Host:       msg.Host,
			CommitSHA:  msg.CommitSHA,
			LineTarget: msg.LineTarget,
		}
		if err != nil {
			result.Err = err
			return result
		}
		content, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			result.Err = readErr
			return result
		}
		result.Body = stripEditorComments(string(content))
		return result
	})
}

// submitPRReview builds a ReviewRequest from the editor output and calls the
// repo API client asynchronously, returning a PRActionResultMsg.
func submitPRReview(msg tui.PREditorFinishedMsg) tea.Cmd {
	body := strings.TrimSpace(msg.Body)
	if body == "" {
		return func() tea.Msg {
			return tui.PRActionResultMsg{
				Action: msg.Action,
				Err:    fmt.Errorf("empty comment body — nothing to submit"),
			}
		}
	}

	host := msg.Host
	if host == repoapi.HostUnknown {
		host = repoapi.HostGitHub
	}

	token := resolveAPIToken(host)

	req := &repoapi.ReviewRequest{
		Host:        host,
		Token:       token,
		Owner:       msg.Owner,
		Repo:        msg.Repo,
		PRNumber:    msg.PRNumber,
		Event:       msg.Action,
		Body:        body,
		LineComment: msg.LineTarget,
		CommitSHA:   msg.CommitSHA,
	}

	return func() tea.Msg {
		client := repoapi.NewClientWithHTTP(httpclient.New(httpclient.IMAPBatchActionTimeout))
		_, err := client.SubmitReview(req)
		return tui.PRActionResultMsg{
			Action: msg.Action,
			Err:    err,
		}
	}
}

// resolveAPIToken returns the best available token for the given host.
// For GitHub it checks GITHUB_TOKEN, then falls back to `gh auth token`.
// For GitLab it checks GITLAB_TOKEN, then falls back to `glab auth token`.
func resolveAPIToken(host repoapi.Host) string {
	switch host {
	case repoapi.HostGitHub:
		if t := os.Getenv("GITHUB_TOKEN"); t != "" {
			return t
		}
		return tokenFromCLI("gh", "auth", "token")
	case repoapi.HostGitLab:
		if t := os.Getenv("GITLAB_TOKEN"); t != "" {
			return t
		}
		return tokenFromCLI("glab", "auth", "token")
	}
	return ""
}

// tokenFromCLI runs a CLI tool (e.g. `gh auth token`) and returns the trimmed
// stdout. Returns "" if the tool is not installed or exits non-zero.
func tokenFromCLI(name string, args ...string) string {
	cmd := exec.Command(name, args...) //nolint:noctx
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// stripEditorComments removes HTML comment lines that were part of the
// template so they don't get submitted as the review body.
func stripEditorComments(content string) string {
	var sb strings.Builder
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!--") {
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}
