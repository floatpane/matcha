package tui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/list"
	threading "github.com/floatpane/jwz-go"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/github"
	"github.com/floatpane/matcha/view"
)

func (m *Inbox) itemForGitHubGroup(entry gitHubGroupEntry, showAccountLabel bool) item {
	accountEmail := ""
	if showAccountLabel {
		accountEmail = m.accountLabelForEmail(entry.Email)
	}

	repoDisplay := entry.Key.RepoName
	if entry.Key.OrgName != "" {
		repoDisplay = fmt.Sprintf("%s/%s", entry.Key.OrgName, entry.Key.RepoName)
	}
	prefix := "#"
	if entry.Key.IsPR {
		prefix = "PR #"
	}

	title := entry.Title
	if title == "" {
		title = entry.Email.Subject
	}
	title = fmt.Sprintf("%s%d: %s", prefix, entry.Key.IssueNumber, title)

	desc := fmt.Sprintf("%s • %d notifications", repoDisplay, entry.Count)
	if entry.LatestSender != "" {
		desc = fmt.Sprintf("%s • latest by %s • %d notifications", repoDisplay, entry.LatestSender, entry.Count)
	}

	return item{
		title:            title,
		desc:             desc,
		originalIndex:    entry.Index,
		uid:              entry.Email.UID,
		accountID:        entry.Email.AccountID,
		accountEmail:     accountEmail,
		date:             entry.LatestDate,
		isRead:           entry.AllRead,
		isGitHubGroup:    true,
		githubGroupKey:   entry.Key,
		githubGroupCount: entry.Count,
		githubGroupPR:    entry.Key.IsPR,
	}
}

type gitHubGroupEntry struct {
	Key          github.EventKey
	Title        string
	Email        fetcher.Email
	Index        int
	Count        int
	LatestDate   time.Time
	LatestSender string
	AllRead      bool
	IsGroup      bool
}

func groupGitHubEmails(emails []fetcher.Email) []gitHubGroupEntry {
	groups := make(map[github.EventKey]*gitHubGroupEntry)
	order := []github.EventKey{}

	for i, email := range emails {
		key, ok := view.ExtractGitHubKey(email)
		if !ok {
			order = append(order, github.EventKey{IssueNumber: -1 - i})
			groups[github.EventKey{IssueNumber: -1 - i}] = &gitHubGroupEntry{
				Email:   email,
				Index:   i,
				IsGroup: false,
			}
			continue
		}

		view.ParseGitHubNotification(email)

		existing, found := groups[key]
		if !found {
			order = append(order, key)
			groups[key] = &gitHubGroupEntry{
				Key:          key,
				Title:        extractGitHubTitle(email),
				Email:        email,
				Index:        i,
				Count:        1,
				LatestDate:   email.Date,
				LatestSender: parseSenderName(email.From),
				AllRead:      email.IsRead,
				IsGroup:      true,
			}
		} else {
			existing.Count++
			if email.Date.After(existing.LatestDate) {
				existing.LatestDate = email.Date
				existing.LatestSender = parseSenderName(email.From)
				existing.Email = email
				existing.Index = i
			}
			if !email.IsRead {
				existing.AllRead = false
			}
		}
		github.AddEmailToGroup(key, email.UID, email.AccountID)
	}

	result := make([]gitHubGroupEntry, 0, len(order))
	for _, key := range order {
		result = append(result, *groups[key])
	}
	return result
}

func partitionGitHubEmails(emails []fetcher.Email) (githubEmails []fetcher.Email, regularEmails []fetcher.Email, githubIndices map[int]int) {
	githubIndices = make(map[int]int)
	for i, email := range emails {
		if _, ok := view.ExtractGitHubKey(email); ok {
			githubIndices[len(githubEmails)] = i
			githubEmails = append(githubEmails, email)
		} else {
			regularEmails = append(regularEmails, email)
		}
	}
	return githubEmails, regularEmails, githubIndices
}

func extractGitHubTitle(email fetcher.Email) string {
	_, _, _, title, _ := view.ExtractGitHubMetadata(email)
	if title != "" {
		return title
	}
	return email.Subject
}

func (m *Inbox) itemsForEmailsThreaded(displayEmails []fetcher.Email, showAccountLabel bool) []list.Item {
	emailIndex := make(map[string]int, len(displayEmails))
	headers := make([]threading.EmailHeader, 0, len(displayEmails))
	for i, email := range displayEmails {
		id := inboxEmailID(email)
		emailIndex[id] = i
		headers = append(headers, threading.EmailHeader{
			ID:         email.MessageID,
			InReplyTo:  email.InReplyTo,
			References: email.References,
			Subject:    email.Subject,
			Date:       email.Date,
			EmailID:    id,
			Sender:     email.From,
		})
	}

	var items []list.Item
	for _, thread := range threading.Build(headers) {
		key := threadItemKey(thread.Root)
		root := firstEmailNode(thread.Root)
		if root == nil {
			continue
		}
		idx := emailIndex[root.EmailID]
		rootEmail := displayEmails[idx]
		latest := latestEmailNode(thread.Root)
		if latest == nil {
			latest = root
		}

		rootItem := m.itemForEmail(rootEmail, idx, showAccountLabel)
		rootItem.title = firstNonEmpty(root.Subject, thread.Subject)
		rootItem.desc = latest.Sender
		rootItem.date = thread.LatestAt
		rootItem.isRead = threadRead(displayEmails, emailIndex, thread.Root)
		rootItem.threadKey = key
		rootItem.threadCount = thread.Count
		rootItem.threadRoot = true
		rootItem.expanded = m.expanded[key]

		if m.expanded[key] {
			headerItem := item{
				title:        firstNonEmpty(root.Subject, thread.Subject),
				desc:         latest.Sender,
				date:         thread.LatestAt,
				isRead:       threadRead(displayEmails, emailIndex, thread.Root),
				threadKey:    key,
				threadCount:  thread.Count,
				threadRoot:   true,
				threadHeader: true,
				expanded:     true,
			}
			items = append(items, headerItem)

			clickableRoot := m.itemForEmail(rootEmail, idx, showAccountLabel)
			clickableRoot.threadKey = key
			clickableRoot.threadCount = thread.Count
			clickableRoot.threadChild = true
			clickableRoot.threadDepth = 1
			items = append(items, clickableRoot)

			items = appendThreadChildren(items, m, displayEmails, emailIndex, showAccountLabel, thread.Root.Children, 2)
		} else {
			items = append(items, rootItem)
		}
	}
	return items
}
