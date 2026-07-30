package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/backend/repoapi"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/github"
	"github.com/floatpane/matcha/view"
)

var (
	githubHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(0, 1)

	githubRepoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	githubIssueNumStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	githubTitleStyle = lipgloss.NewStyle().
				Bold(true).
				PaddingTop(1)

	githubBadgeOpenStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true).
				Padding(0, 1).
				Background(lipgloss.Color("235"))

	githubBadgeClosedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true).
				Padding(0, 1).
				Background(lipgloss.Color("235"))

	githubBadgeMergedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("99")).
				Bold(true).
				Padding(0, 1).
				Background(lipgloss.Color("235"))

	githubStatsStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				MarginTop(1)

	githubBranchStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("250")).
				Background(lipgloss.Color("235")).
				Padding(0, 1).
				MarginRight(1)

	githubCommentBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				Padding(1, 2).
				MarginTop(1).
				MarginBottom(1)

	githubSystemEventStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Italic(true).
				Padding(0, 1).
				MarginTop(1)

	githubAuthorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	githubTimestampStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	githubBodyStyle = lipgloss.NewStyle().
			MarginTop(1)

	githubDividerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				MarginTop(1).
				MarginBottom(1)

	githubSectionHeaderStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("42")).
					Background(lipgloss.Color("235")).
					Padding(0, 1).
					MarginTop(1).
					MarginBottom(1)
)

type GitHubConversationView struct {
	group    *github.NotificationGroup
	width    int
	height   int
	email    fetcher.Email
	mailbox  MailboxKind
	viewport viewport.Model
}

func NewGitHubConversationView(group *github.NotificationGroup, email fetcher.Email, width, height int, mailbox MailboxKind) *GitHubConversationView {
	vp := viewport.New()
	vp.SetWidth(width)
	vp.SetHeight(height - 2)
	return &GitHubConversationView{
		group:    group,
		width:    width,
		height:   height,
		email:    email,
		mailbox:  mailbox,
		viewport: vp,
	}
}

func (m *GitHubConversationView) Init() tea.Cmd {
	return nil
}

func (m *GitHubConversationView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		kb := config.Keybinds
		if msg.String() == kb.Global.Cancel {
			ClearKittyGraphics()
			return m, func() tea.Msg { return BackToMailboxMsg{Mailbox: m.mailbox} }
		}
		if cmd := m.handlePRActionKey(msg); cmd != nil {
			return m, cmd
		}
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - 2)
		m.viewport.SetContent(m.RenderContent())
	case PRActionResultMsg:
		if msg.Err != nil {
			return m, func() tea.Msg {
				return NotifyMsg{Message: fmt.Sprintf("PR action failed: %v", msg.Err)}
			}
		}
		actionName := "completed"
		switch msg.Action {
		case repoapi.ReviewApprove:
			actionName = "approved"
		case repoapi.ReviewRequestChanges:
			actionName = "changes requested"
		case repoapi.ReviewComment:
			actionName = "comment posted"
		}
		return m, func() tea.Msg {
			return InfoNotifyMsg{Message: fmt.Sprintf("PR review %s", actionName), Duration: 3}
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// handlePRActionKey checks whether the pressed key maps to a direct PR action
// (approve, request changes, or leave comment). It returns a tea.Cmd that
// emits a PREditorOpenMsg for the app layer to open the editor and then call
// the repo API. Returns nil if the key is not a PR action or the notification
// is not a PR.
func (m *GitHubConversationView) handlePRActionKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.group == nil || !m.group.IsPR {
		return nil
	}
	kb := config.Keybinds
	var action repoapi.ReviewEvent
	switch msg.String() {
	case kb.Email.ApprovePR:
		action = repoapi.ReviewApprove
	case kb.Email.RequestChanges:
		action = repoapi.ReviewRequestChanges
	case kb.Email.LeaveComment:
		action = repoapi.ReviewComment
	default:
		return nil
	}

	key := m.group.Key
	host := repoapi.ParseHostFromEmailSender(m.email.From)
	commitSHA := ""
	if m.group.PRDetails != nil && m.group.PRDetails.Head.Sha != "" {
		commitSHA = m.group.PRDetails.Head.Sha
	}

	return func() tea.Msg {
		return PREditorOpenMsg{
			Action:     action,
			Owner:      key.OrgName,
			Repo:       key.RepoName,
			PRNumber:   key.IssueNumber,
			Host:       host,
			CommitSHA:  commitSHA,
			LineTarget: nil,
		}
	}
}

func (m *GitHubConversationView) View() tea.View {
	if m.group == nil {
		return tea.NewView("Invalid GitHub notification")
	}
	m.viewport.SetContent(m.RenderContent())
	return tea.NewView(m.viewport.View())
}

func (m *GitHubConversationView) RenderContent() string {
	var sb strings.Builder

	sb.WriteString(m.renderHeader())
	sb.WriteString("\n")

	if m.group.IsPR && m.group.PRDetails != nil {
		sb.WriteString(m.renderPRDetails())
		sb.WriteString("\n")
	} else if m.group.IssueDetails != nil {
		sb.WriteString(m.renderIssueDetails())
		sb.WriteString("\n")
	}

	sb.WriteString(githubDividerStyle.Render(strings.Repeat("─", m.width-2)))
	sb.WriteString("\n")

	filtered := filterEvents(m.group.Events)
	grouped := groupEventsByType(filtered)
	for i, section := range grouped {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(githubSectionHeaderStyle.Render(section.Header))
		sb.WriteString("\n")
		for _, event := range section.Events {
			sb.WriteString(m.renderEvent(event))
		}
	}

	sb.WriteString("\n")
	helpParts := []string{fmt.Sprintf("%s back", config.Keybinds.Global.Cancel)}
	if m.group.IsPR {
		kb := config.Keybinds
		helpParts = append(helpParts,
			fmt.Sprintf("%s approve", kb.Email.ApprovePR),
			fmt.Sprintf("%s request changes", kb.Email.RequestChanges),
			fmt.Sprintf("%s comment", kb.Email.LeaveComment),
		)
	}
	sb.WriteString(HelpStyle.Render(strings.Join(helpParts, "  ")))

	return sb.String()
}

func (m *GitHubConversationView) renderHeader() string {
	var parts []string

	repoDisplay := m.group.Key.RepoName
	if m.group.Key.OrgName != "" {
		repoDisplay = fmt.Sprintf("%s/%s", m.group.Key.OrgName, m.group.Key.RepoName)
	}
	repoURL := fmt.Sprintf("https://github.com/%s", repoDisplay)
	parts = append(parts, githubRepoStyle.Render(view.Hyperlink(repoURL, repoDisplay)))

	if m.group.Key.IssueNumber > 0 {
		var itemURL string
		prefix := "#"
		if m.group.IsPR {
			prefix = "PR #"
			itemURL = fmt.Sprintf("https://github.com/%s/pull/%d", repoDisplay, m.group.Key.IssueNumber)
		} else {
			itemURL = fmt.Sprintf("https://github.com/%s/issues/%d", repoDisplay, m.group.Key.IssueNumber)
		}
		parts = append(parts, githubIssueNumStyle.Render(view.Hyperlink(itemURL, fmt.Sprintf("%s%d", prefix, m.group.Key.IssueNumber))))
	}

	headerLine := strings.Join(parts, " ")

	title := m.group.Title
	if title == "" {
		title = m.email.Subject
	}

	badge := m.renderStateBadge()

	var header strings.Builder
	header.WriteString(githubHeaderStyle.Render(headerLine))
	header.WriteString("\n")
	header.WriteString(githubTitleStyle.Render(title))
	header.WriteString("\n")
	header.WriteString(badge)

	return header.String()
}

func (m *GitHubConversationView) renderStateBadge() string {
	state := strings.ToLower(m.group.State)
	switch state {
	case "open":
		return githubBadgeOpenStyle.Render("● Open")
	case "closed":
		return githubBadgeClosedStyle.Render("● Closed")
	case "merged":
		return githubBadgeMergedStyle.Render("● Merged")
	default:
		return ""
	}
}

func (m *GitHubConversationView) renderPRDetails() string {
	pr := m.group.PRDetails
	if pr == nil {
		return ""
	}

	var sb strings.Builder

	if pr.User.Login != "" {
		userURL := fmt.Sprintf("https://github.com/%s", pr.User.Login)
		sb.WriteString(githubAuthorStyle.Render(view.Hyperlink(userURL, pr.User.Login)))
		sb.WriteString("\n")
	}

	if pr.Head.Ref != "" && pr.Base.Ref != "" {
		sb.WriteString(githubBranchStyle.Render(pr.Head.Ref))
		sb.WriteString("→ ")
		sb.WriteString(githubBranchStyle.Render(pr.Base.Ref))
		sb.WriteString("\n")
	}

	var stats []string
	if pr.Commits > 0 {
		stats = append(stats, fmt.Sprintf("%d commits", pr.Commits))
	}
	if pr.ChangedFiles > 0 {
		stats = append(stats, fmt.Sprintf("%d files changed", pr.ChangedFiles))
	}
	if pr.Additions > 0 || pr.Deletions > 0 {
		stats = append(stats, fmt.Sprintf("+%d -%d", pr.Additions, pr.Deletions))
	}
	if pr.Comments > 0 {
		stats = append(stats, fmt.Sprintf("%d comments", pr.Comments))
	}
	if pr.ReviewComments > 0 {
		stats = append(stats, fmt.Sprintf("%d review comments", pr.ReviewComments))
	}
	if len(stats) > 0 {
		sb.WriteString(githubStatsStyle.Render(strings.Join(stats, " • ")))
		sb.WriteString("\n")
	}

	if pr.Body != "" {
		renderedBody, _, err := view.ProcessBody(pr.Body, "", H1Style, H2Style, BodyStyle, false)
		if err != nil {
			renderedBody = pr.Body
		}
		sb.WriteString(githubBodyStyle.Render(renderedBody))
		sb.WriteString("\n")
	}

	if len(pr.Labels) > 0 {
		var labels []string
		for _, label := range pr.Labels {
			labels = append(labels, label.Name)
		}
		sb.WriteString(githubStatsStyle.Render("Labels: " + strings.Join(labels, ", ")))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m *GitHubConversationView) renderIssueDetails() string {
	issue := m.group.IssueDetails
	if issue == nil {
		return ""
	}

	var sb strings.Builder

	if issue.User.Login != "" {
		userURL := fmt.Sprintf("https://github.com/%s", issue.User.Login)
		sb.WriteString(githubAuthorStyle.Render(view.Hyperlink(userURL, issue.User.Login)))
		sb.WriteString("\n")
	}

	var stats []string
	if issue.Comments > 0 {
		stats = append(stats, fmt.Sprintf("%d comments", issue.Comments))
	}
	if len(stats) > 0 {
		sb.WriteString(githubStatsStyle.Render(strings.Join(stats, " • ")))
		sb.WriteString("\n")
	}

	if issue.Body != "" {
		renderedBody, _, err := view.ProcessBody(issue.Body, "", H1Style, H2Style, BodyStyle, false)
		if err != nil {
			renderedBody = issue.Body
		}
		sb.WriteString(githubBodyStyle.Render(renderedBody))
		sb.WriteString("\n")
	}

	if len(issue.Labels) > 0 {
		var labels []string
		for _, label := range issue.Labels {
			labels = append(labels, label.Name)
		}
		sb.WriteString(githubStatsStyle.Render("Labels: " + strings.Join(labels, ", ")))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m *GitHubConversationView) renderEvent(event github.Event) string {
	if event.IsSystem {
		return githubSystemEventStyle.Render(event.SystemMsg) + "\n"
	}

	var sb strings.Builder

	timestamp := formatTime(event.Timestamp)
	action := m.formatEventType(event.EventType)

	login := event.ActorLogin
	if login == "" {
		login = event.Actor
	}
	actorURL := fmt.Sprintf("https://github.com/%s", login)
	styledActor := githubAuthorStyle.Render(view.Hyperlink(actorURL, event.Actor))

	if event.EventType == github.EventCommented {
		commentURL := m.commentURL(event)
		action = view.Hyperlink(commentURL, action)
	}

	sb.WriteString(styledActor)
	sb.WriteString(" ")
	sb.WriteString(githubTimestampStyle.Render(fmt.Sprintf("%s %s", action, timestamp)))
	sb.WriteString("\n")

	if event.Body != "" {
		renderedBody, _, err := view.ProcessBody(event.Body, "", H1Style, H2Style, BodyStyle, false)
		if err != nil {
			renderedBody = event.Body
		}
		sb.WriteString(githubCommentBoxStyle.Render(renderedBody))
	}

	return sb.String()
}

func (m *GitHubConversationView) commentURL(event github.Event) string {
	repoDisplay := m.group.Key.RepoName
	if m.group.Key.OrgName != "" {
		repoDisplay = fmt.Sprintf("%s/%s", m.group.Key.OrgName, m.group.Key.RepoName)
	}
	base := fmt.Sprintf("https://github.com/%s", repoDisplay)
	if m.group.IsPR {
		base = fmt.Sprintf("%s/pull/%d", base, m.group.Key.IssueNumber)
	} else {
		base = fmt.Sprintf("%s/issues/%d", base, m.group.Key.IssueNumber)
	}
	commentID := extractCommentID(event.RawEmail.MessageID)
	if commentID != "" {
		return fmt.Sprintf("%s#issuecomment-%s", base, commentID)
	}
	return base
}

func extractCommentID(messageID string) string {
	if messageID == "" {
		return ""
	}
	messageID = strings.Trim(messageID, "<>")
	parts := strings.Split(messageID, "/")
	for i, p := range parts {
		if p == "pull" || p == "issues" {
			if i+2 < len(parts) {
				commentPart := parts[i+2]
				commentPart = strings.TrimSuffix(commentPart, "@github.com")
				commentPart = strings.TrimPrefix(commentPart, "c")
				if commentPart != "" {
					return commentPart
				}
			}
		}
	}
	return ""
}

func (m *GitHubConversationView) formatEventType(eventType github.EventType) string {
	switch eventType {
	case github.EventOpened:
		return "opened"
	case github.EventClosed:
		return "closed"
	case github.EventMerged:
		return "merged"
	case github.EventCommented:
		return "commented"
	case github.EventReview:
		return "reviewed"
	case github.EventReviewRequest:
		return "requested review"
	case github.EventPush:
		return "pushed commits"
	case github.EventLabel:
		return "added label"
	case github.EventAssign:
		return "was assigned"
	default:
		return "notified"
	}
}

func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		minutes := int(diff.Minutes())
		return fmt.Sprintf("%d minute%s ago", minutes, plural(minutes))
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		return fmt.Sprintf("%d hour%s ago", hours, plural(hours))
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d day%s ago", days, plural(days))
	default:
		return t.Format("Jan 2, 2006")
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func filterEvents(events []github.Event) []github.Event {
	var filtered []github.Event
	for _, event := range events {
		if event.IsSystem && event.SystemMsg != "" {
			filtered = append(filtered, event)
			continue
		}
		if strings.TrimSpace(event.Body) != "" {
			filtered = append(filtered, event)
			continue
		}
		if event.EventType != github.EventUnknown {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

type eventSection struct {
	Header string
	Events []github.Event
}

func groupEventsByType(events []github.Event) []eventSection {
	if len(events) == 0 {
		return nil
	}

	groups := make(map[github.EventType][]github.Event)
	order := []github.EventType{}
	seen := make(map[github.EventType]bool)

	for _, event := range events {
		if !seen[event.EventType] {
			seen[event.EventType] = true
			order = append(order, event.EventType)
		}
		groups[event.EventType] = append(groups[event.EventType], event)
	}

	var sections []eventSection
	for _, eventType := range order {
		sectionEvents := groups[eventType]
		if len(sectionEvents) == 0 {
			continue
		}
		sections = append(sections, eventSection{
			Header: sectionHeader(eventType, len(sectionEvents)),
			Events: sectionEvents,
		})
	}
	return sections
}

func sectionHeader(eventType github.EventType, count int) string {
	name := eventTypeName(eventType)
	if count == 1 {
		return fmt.Sprintf("1 %s", name)
	}
	return fmt.Sprintf("%d %s", count, pluralize(name))
}

func pluralize(word string) string {
	if strings.HasSuffix(strings.ToLower(word), "y") {
		return word[:len(word)-1] + "ies"
	}
	return word + "s"
}

func eventTypeName(eventType github.EventType) string {
	switch eventType {
	case github.EventOpened:
		return "Opened"
	case github.EventClosed:
		return "Closed"
	case github.EventMerged:
		return "Merged"
	case github.EventCommented:
		return "Comment"
	case github.EventReview:
		return "Review"
	case github.EventReviewRequest:
		return "Review Request"
	case github.EventPush:
		return "Push"
	case github.EventLabel:
		return "Label Change"
	case github.EventAssign:
		return "Assignment"
	default:
		return "Activity"
	}
}
