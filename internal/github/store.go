package github

import (
	"sort"
	"sync"
	"time"

	"github.com/floatpane/matcha/fetcher"
)

type EventType string

const (
	EventOpened        EventType = "opened"
	EventClosed        EventType = "closed"
	EventMerged        EventType = "merged"
	EventCommented     EventType = "commented"
	EventReview        EventType = "review"
	EventReviewRequest EventType = "review_requested"
	EventPush          EventType = "push"
	EventLabel         EventType = "label"
	EventAssign        EventType = "assign"
	EventUnknown       EventType = "unknown"
)

type EventKey struct {
	OrgName     string
	RepoName    string
	IssueNumber int
	IsPR        bool
}

type Event struct {
	EventType  EventType
	Actor      string
	ActorLogin string
	Body       string
	Timestamp  time.Time
	IsSystem   bool
	SystemMsg  string
	RawEmail   fetcher.Email
}

type NotificationGroup struct {
	Key          EventKey
	Title        string
	State        string
	IsPR         bool
	PRDetails    *PRDetails
	IssueDetails *IssueDetails
	Events       []Event
	EmailUIDs    []EmailUID
	LastUpdated  time.Time
}

type EmailUID struct {
	UID       uint32
	AccountID string
}

var (
	store     = make(map[EventKey]*NotificationGroup)
	storeMu   sync.RWMutex
	maxGroups = 100
)

func GetOrCreateGroup(key EventKey, title, state string, isPR bool) *NotificationGroup {
	storeMu.Lock()
	defer storeMu.Unlock()

	group, exists := store[key]
	if !exists {
		group = &NotificationGroup{
			Key:   key,
			Title: title,
			State: state,
			IsPR:  isPR,
		}
		store[key] = group
		pruneStore()
	}
	return group
}

func GetGroup(key EventKey) *NotificationGroup {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return store[key]
}

func AddEmailToGroup(key EventKey, uid uint32, accountID string) {
	if uid == 0 {
		return
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	group, exists := store[key]
	if !exists {
		return
	}
	for _, existing := range group.EmailUIDs {
		if existing.UID == uid && existing.AccountID == accountID {
			return
		}
	}
	group.EmailUIDs = append(group.EmailUIDs, EmailUID{UID: uid, AccountID: accountID})
}

func GetGroupEmailUIDs(key EventKey) []EmailUID {
	storeMu.RLock()
	defer storeMu.RUnlock()
	group, exists := store[key]
	if !exists {
		return nil
	}
	return append([]EmailUID(nil), group.EmailUIDs...)
}

func AddEvent(key EventKey, title, state string, isPR bool, event Event) {
	group := GetOrCreateGroup(key, title, state, isPR)
	storeMu.Lock()
	defer storeMu.Unlock()

	if event.RawEmail.UID != 0 {
		for _, existing := range group.Events {
			if existing.RawEmail.UID == event.RawEmail.UID && existing.RawEmail.AccountID == event.RawEmail.AccountID {
				return
			}
		}
	}

	group.Events = append(group.Events, event)
	group.LastUpdated = event.Timestamp
	if event.Timestamp.After(group.LastUpdated) {
		group.LastUpdated = event.Timestamp
	}
	if title != "" && group.Title == "" {
		group.Title = title
	}
	if state != "" {
		group.State = state
	}
	sort.Slice(group.Events, func(i, j int) bool {
		return group.Events[i].Timestamp.Before(group.Events[j].Timestamp)
	})
}

func UpdateEventBody(key EventKey, uid uint32, accountID string, body string, eventType EventType) {
	if uid == 0 {
		return
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	group, exists := store[key]
	if !exists {
		return
	}
	for i, event := range group.Events {
		if event.RawEmail.UID == uid && event.RawEmail.AccountID == accountID {
			group.Events[i].Body = body
			group.Events[i].EventType = eventType
			return
		}
	}
}

func UpdateEventSystem(key EventKey, uid uint32, accountID string, systemMsg string, eventType EventType) {
	if uid == 0 {
		return
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	group, exists := store[key]
	if !exists {
		return
	}
	for i, event := range group.Events {
		if event.RawEmail.UID == uid && event.RawEmail.AccountID == accountID {
			group.Events[i].IsSystem = true
			group.Events[i].SystemMsg = systemMsg
			group.Events[i].Body = ""
			group.Events[i].EventType = eventType
			return
		}
	}
}

func SetPRDetails(key EventKey, details *PRDetails) {
	storeMu.Lock()
	defer storeMu.Unlock()
	group, exists := store[key]
	if !exists {
		return
	}
	group.PRDetails = details
	if details.Title != "" {
		group.Title = details.Title
	}
	if details.State != "" {
		group.State = details.State
	}
	if details.Merged {
		group.State = "merged"
	}
}

func SetIssueDetails(key EventKey, details *IssueDetails) {
	storeMu.Lock()
	defer storeMu.Unlock()
	group, exists := store[key]
	if !exists {
		return
	}
	group.IssueDetails = details
	if details.Title != "" {
		group.Title = details.Title
	}
	if details.State != "" {
		group.State = details.State
	}
}

func pruneStore() {
	if len(store) <= maxGroups {
		return
	}
	var keys []EventKey
	for k := range store {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return store[keys[i]].LastUpdated.Before(store[keys[j]].LastUpdated)
	})
	for i := 0; i < len(keys)-maxGroups; i++ {
		delete(store, keys[i])
	}
}
