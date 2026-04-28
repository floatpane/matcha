package threading

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

type EmailHeader struct {
	ID         string
	InReplyTo  string
	References []string
	Subject    string
	Date       time.Time
	EmailID    string
	Sender     string
}

type Thread struct {
	Root     *ThreadNode
	LatestAt time.Time
	Count    int
	Subject  string
	Senders  []string
}

type ThreadNode struct {
	EmailID  string
	Children []*ThreadNode
	Date     time.Time
	Sender   string
	Subject  string
}

type container struct {
	id       string
	node     *ThreadNode
	parent   *container
	children []*container
}

var messageIDRE = regexp.MustCompile(`<[^>]+>`)

func Build(headers []EmailHeader) []Thread {
	containers := make(map[string]*container)
	ordered := make([]*container, 0, len(headers))

	get := func(id string) *container {
		if c := containers[id]; c != nil {
			return c
		}
		c := &container{id: id}
		containers[id] = c
		ordered = append(ordered, c)
		return c
	}

	for _, h := range headers {
		msgID := normalizeMessageID(h.ID)
		if msgID == "" {
			msgID = "email:" + h.EmailID
		}
		c := get(msgID)
		if c.node != nil {
			msgID = msgID + "#email:" + h.EmailID
			c = get(msgID)
		}
		c.node = &ThreadNode{
			EmailID: h.EmailID,
			Date:    h.Date,
			Sender:  h.Sender,
			Subject: h.Subject,
		}

		var prev *container
		refs := normalizeReferences(h.References)
		for _, ref := range refs {
			refc := get(ref)
			if prev != nil {
				link(prev, refc)
			}
			prev = refc
		}

		parentID := normalizeMessageID(h.InReplyTo)
		if parentID == "" && len(refs) > 0 {
			parentID = refs[len(refs)-1]
		}
		if parentID != "" {
			link(get(parentID), c)
		}
	}

	var roots []*container
	for _, c := range ordered {
		if c.parent == nil {
			if root := prune(c); root != nil {
				roots = append(roots, root)
			}
		}
	}
	roots = groupBySubject(roots)

	threads := make([]Thread, 0, len(roots))
	for _, root := range roots {
		sortContainer(root)
		thread := buildThread(root)
		if thread.Count > 0 {
			threads = append(threads, thread)
		}
	}

	sort.SliceStable(threads, func(i, j int) bool {
		if !threads[i].LatestAt.Equal(threads[j].LatestAt) {
			return threads[i].LatestAt.After(threads[j].LatestAt)
		}
		return threadKey(threads[i].Root) < threadKey(threads[j].Root)
	})

	return threads
}

func normalizeReferences(refs []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, ref := range refs {
		for _, id := range extractMessageIDs(ref) {
			if !seen[id] {
				out = append(out, id)
				seen[id] = true
			}
		}
	}
	return out
}

func extractMessageIDs(s string) []string {
	matches := messageIDRE.FindAllString(s, -1)
	if len(matches) == 0 {
		if id := normalizeMessageID(s); id != "" {
			return []string{id}
		}
		return nil
	}
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		if id := normalizeMessageID(match); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func normalizeMessageID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if matches := messageIDRE.FindAllString(id, -1); len(matches) > 0 {
		id = matches[len(matches)-1]
	}
	id = strings.TrimSpace(id)
	id = strings.TrimPrefix(id, "<")
	id = strings.TrimSuffix(id, ">")
	id = strings.TrimSpace(id)
	return strings.ToLower(id)
}

func link(parent, child *container) {
	if parent == nil || child == nil || parent == child {
		return
	}
	if child.parent != nil || child.hasDescendant(parent) {
		return
	}
	child.parent = parent
	for _, existing := range parent.children {
		if existing == child {
			return
		}
	}
	parent.children = append(parent.children, child)
}

func (c *container) hasDescendant(target *container) bool {
	for _, child := range c.children {
		if child == target || child.hasDescendant(target) {
			return true
		}
	}
	return false
}

func prune(c *container) *container {
	if c == nil {
		return nil
	}
	var children []*container
	for _, child := range c.children {
		if pruned := prune(child); pruned != nil {
			pruned.parent = c
			children = append(children, pruned)
		}
	}
	c.children = children

	if c.node != nil {
		return c
	}
	switch len(c.children) {
	case 0:
		return nil
	case 1:
		child := c.children[0]
		child.parent = c.parent
		return child
	default:
		return c
	}
}

func groupBySubject(roots []*container) []*container {
	subjects := make(map[string]*container)
	var grouped []*container
	for _, root := range roots {
		subject := firstSubject(root)
		if subject == "" {
			grouped = append(grouped, root)
			continue
		}
		if existing := subjects[subject]; existing != nil {
			link(existing, root)
			continue
		}
		subjects[subject] = root
		grouped = append(grouped, root)
	}
	return grouped
}

func firstSubject(c *container) string {
	if c == nil {
		return ""
	}
	if c.node != nil {
		return canonicalSubject(c.node.Subject)
	}
	for _, child := range c.children {
		if subject := firstSubject(child); subject != "" {
			return subject
		}
	}
	return ""
}

func sortContainer(c *container) {
	for _, child := range c.children {
		sortContainer(child)
	}
	sort.SliceStable(c.children, func(i, j int) bool {
		a, b := c.children[i], c.children[j]
		ad, bd := containerDate(a), containerDate(b)
		if !ad.Equal(bd) {
			return ad.Before(bd)
		}
		return containerKey(a) < containerKey(b)
	})
}

func buildThread(root *container) Thread {
	node := toThreadNode(root)
	thread := Thread{Root: node, Subject: canonicalSubject(firstDisplaySubject(node))}
	seenSenders := make(map[string]bool)
	walkThread(node, &thread, seenSenders)
	return thread
}

func toThreadNode(c *container) *ThreadNode {
	node := &ThreadNode{}
	if c.node != nil {
		*node = *c.node
		node.Children = nil
	}
	for _, child := range c.children {
		node.Children = append(node.Children, toThreadNode(child))
	}
	return node
}

func walkThread(node *ThreadNode, thread *Thread, seenSenders map[string]bool) {
	if node == nil {
		return
	}
	if node.EmailID != "" {
		thread.Count++
		if node.Date.After(thread.LatestAt) {
			thread.LatestAt = node.Date
		}
		if node.Sender != "" && !seenSenders[node.Sender] {
			thread.Senders = append(thread.Senders, node.Sender)
			seenSenders[node.Sender] = true
		}
	}
	for _, child := range node.Children {
		walkThread(child, thread, seenSenders)
	}
}

func containerDate(c *container) time.Time {
	if c == nil {
		return time.Time{}
	}
	if c.node != nil {
		return c.node.Date
	}
	var earliest time.Time
	for _, child := range c.children {
		date := containerDate(child)
		if earliest.IsZero() || (!date.IsZero() && date.Before(earliest)) {
			earliest = date
		}
	}
	return earliest
}

func containerKey(c *container) string {
	if c == nil {
		return ""
	}
	if c.node != nil && c.node.EmailID != "" {
		return c.node.EmailID
	}
	return c.id
}

func threadKey(n *ThreadNode) string {
	if n == nil {
		return ""
	}
	if n.EmailID != "" {
		return n.EmailID
	}
	for _, child := range n.Children {
		if key := threadKey(child); key != "" {
			return key
		}
	}
	return ""
}

func firstDisplaySubject(node *ThreadNode) string {
	if node == nil {
		return ""
	}
	if node.Subject != "" {
		return node.Subject
	}
	for _, child := range node.Children {
		if subject := firstDisplaySubject(child); subject != "" {
			return subject
		}
	}
	return ""
}
