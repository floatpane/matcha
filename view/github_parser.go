package view

import (
	"regexp"
	"strings"
	"time"

	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/github"
)

type GitHubNotification = github.NotificationGroup

type GitHubComment struct {
	Author    string
	Body      string
	Timestamp time.Time
	IsSystem  bool
	SystemMsg string
}

var (
	githubSenderRe = regexp.MustCompile(`([^/<]+)<[^>]*>`)
	repoTitleRe    = regexp.MustCompile(`\[([^\]]+)\]\s*(.*)`)
	prNumberRe     = regexp.MustCompile(`#(\d+)`)
	githubFooterRe = regexp.MustCompile(`(?i)(You are receiving this because|Reply to this email directly|View it on GitHub|Unsubscribe)`)
	githubLoginRe  = regexp.MustCompile(`([A-Za-z0-9][A-Za-z0-9\-]*)\s+(?:left a comment|commented|opened|closed|merged|reviewed|requested|pushed|added|assigned|approved)`)
)

func ParseGitHubNotification(email fetcher.Email) *github.NotificationGroup {
	if !isGitHubEmail(email) {
		return nil
	}

	orgName, repoName, issueNumber, title, isPR := extractMetadata(email)
	if issueNumber == 0 {
		return nil
	}

	eventType, state := determineEventType(email)
	actor := extractActor(email)
	actorLogin := extractActorLogin(email)
	body := extractBodyContent(email)

	key := github.EventKey{
		OrgName:     orgName,
		RepoName:    repoName,
		IssueNumber: issueNumber,
		IsPR:        isPR,
	}

	event := github.Event{
		EventType:  eventType,
		Actor:      actor,
		ActorLogin: actorLogin,
		Body:       body,
		Timestamp:  email.Date,
		RawEmail:   email,
	}

	if systemMsg := detectSystemEvent(body); systemMsg != "" {
		event.IsSystem = true
		event.SystemMsg = systemMsg
		event.Body = ""
	}

	existing := github.GetGroup(key)
	if existing != nil && email.UID != 0 {
		github.AddEmailToGroup(key, email.UID, email.AccountID)
		for _, e := range existing.Events {
			if e.RawEmail.UID == email.UID && e.RawEmail.AccountID == email.AccountID {
				if body != "" && e.Body == "" {
					if event.IsSystem {
						github.UpdateEventSystem(key, email.UID, email.AccountID, event.SystemMsg, eventType)
					} else {
						github.UpdateEventBody(key, email.UID, email.AccountID, body, eventType)
					}
				}
				go fetchDetails(key, orgName, repoName, issueNumber, isPR)
				return existing
			}
		}
	}

	github.AddEvent(key, title, state, isPR, event)
	github.AddEmailToGroup(key, email.UID, email.AccountID)
	group := github.GetGroup(key)

	go fetchDetails(key, orgName, repoName, issueNumber, isPR)

	return group
}

func isGitHubEmail(email fetcher.Email) bool {
	for _, to := range email.To {
		lower := strings.ToLower(to)
		if strings.Contains(lower, "notifications@github.com") || strings.Contains(lower, "@noreply.github.com") || strings.Contains(lower, "@no-reply.github.com") {
			return true
		}
	}
	lowerFrom := strings.ToLower(email.From)
	if strings.Contains(lowerFrom, "notifications@github.com") || strings.Contains(lowerFrom, "@noreply.github.com") || strings.Contains(lowerFrom, "@no-reply.github.com") {
		return true
	}
	return false
}

func extractMetadata(email fetcher.Email) (orgName, repoName string, issueNumber int, title string, isPR bool) {
	subject := email.Subject

	matches := repoTitleRe.FindStringSubmatch(subject)
	if len(matches) >= 3 {
		repoPart := matches[1]
		parts := strings.Split(repoPart, "/")
		if len(parts) == 2 {
			orgName = parts[0]
			repoName = parts[1]
		} else {
			repoName = repoPart
		}
		title = strings.TrimSpace(matches[2])
		numMatches := prNumberRe.FindStringSubmatch(title)
		if len(numMatches) >= 2 {
			title = strings.TrimSpace(prNumberRe.ReplaceAllString(title, ""))
			title = strings.TrimSpace(strings.ReplaceAll(title, "(PR )", ""))
			title = strings.TrimSpace(strings.ReplaceAll(title, "PR ", ""))
		}
	}

	numMatches := prNumberRe.FindStringSubmatch(subject)
	if len(numMatches) >= 2 {
		var n int
		for _, c := range numMatches[1] {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		issueNumber = n
	}

	lowerSubject := strings.ToLower(subject)
	if strings.Contains(lowerSubject, "pull request") || strings.Contains(lowerSubject, "pr #") {
		isPR = true
	}

	return orgName, repoName, issueNumber, title, isPR
}

func extractActor(email fetcher.Email) string {
	fromMatch := githubSenderRe.FindStringSubmatch(email.From)
	if len(fromMatch) >= 2 {
		return strings.TrimSpace(fromMatch[1])
	}
	return email.From
}

func extractActorLogin(email fetcher.Email) string {
	loginMatch := githubLoginRe.FindStringSubmatch(email.Body)
	if len(loginMatch) >= 2 {
		return loginMatch[1]
	}
	return ""
}

func determineEventType(email fetcher.Email) (github.EventType, string) {
	subject := strings.ToLower(email.Subject)
	body := strings.ToLower(email.Body)
	state := ""

	switch {
	case strings.Contains(subject, "merged"):
		state = "merged"
		return github.EventMerged, state
	case strings.Contains(subject, "closed"):
		state = "closed"
		return github.EventClosed, state
	case strings.Contains(subject, "pull request"):
		state = "open"
		return github.EventOpened, state
	case strings.Contains(subject, "issue"):
		state = "open"
		return github.EventOpened, state
	case strings.Contains(body, "commented"):
		return github.EventCommented, state
	case strings.Contains(body, "review"):
		return github.EventReview, state
	case strings.Contains(body, "requested your review"):
		return github.EventReviewRequest, state
	case strings.Contains(body, "pushed"):
		return github.EventPush, state
	default:
		return github.EventUnknown, state
	}
}

func extractBodyContent(email fetcher.Email) string {
	body := email.Body

	lines := strings.Split(body, "\n")
	var cleanLines []string
	inFooter := false

	for _, line := range lines {
		if githubFooterRe.MatchString(line) {
			inFooter = true
			continue
		}
		if inFooter {
			continue
		}
		if strings.HasPrefix(line, ">") || strings.HasPrefix(line, "---") {
			continue
		}
		cleanLines = append(cleanLines, line)
	}

	return strings.TrimSpace(strings.Join(cleanLines, "\n"))
}

func detectSystemEvent(body string) string {
	lower := strings.ToLower(body)
	systemPatterns := []string{
		"added the label",
		"removed the label",
		"assigned",
		"unassigned",
		"changed the title",
		"marked as duplicate",
		"referenced this issue",
		"merged commit",
		"closed this",
		"reopened this",
	}
	for _, pattern := range systemPatterns {
		if strings.Contains(lower, pattern) {
			firstLine := strings.Split(body, "\n")[0]
			return strings.TrimSpace(firstLine)
		}
	}
	return ""
}

func ExtractGitHubKey(email fetcher.Email) (github.EventKey, bool) {
	if !isGitHubEmail(email) {
		return github.EventKey{}, false
	}
	orgName, repoName, issueNumber, _, isPR := extractMetadata(email)
	if issueNumber == 0 {
		return github.EventKey{}, false
	}
	return github.EventKey{
		OrgName:     orgName,
		RepoName:    repoName,
		IssueNumber: issueNumber,
		IsPR:        isPR,
	}, true
}

func ExtractGitHubMetadata(email fetcher.Email) (orgName, repoName string, issueNumber int, title string, isPR bool) {
	if !isGitHubEmail(email) {
		return "", "", 0, "", false
	}
	return extractMetadata(email)
}

func fetchDetails(key github.EventKey, orgName, repoName string, issueNumber int, isPR bool) {
	client := github.NewClient()
	if orgName == "" || repoName == "" {
		return
	}
	if isPR {
		details, err := client.FetchPRDetails(orgName, repoName, issueNumber)
		if err != nil {
			return
		}
		github.SetPRDetails(key, details)
	} else {
		details, err := client.FetchIssueDetails(orgName, repoName, issueNumber)
		if err != nil {
			return
		}
		github.SetIssueDetails(key, details)
	}
}
