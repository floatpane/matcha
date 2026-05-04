package fetcher

import (
	"fmt"
	"sort"

	"github.com/emersion/go-imap/v2"
	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
)

// SearchMailbox searches a mailbox server-side and fetches matching envelopes.
func SearchMailbox(account *config.Account, folder string, query backend.SearchQuery) ([]Email, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if _, err := c.Select(folder, nil).Wait(); err != nil {
		return nil, err
	}

	criteria := buildSearchCriteria(query)
	options := (*imap.SearchOptions)(nil)
	if caps := c.Caps(); caps.Has(imap.CapESearch) || caps.Has(imap.CapIMAP4rev2) {
		options = &imap.SearchOptions{ReturnAll: true}
	}

	searchData, err := c.UIDSearch(criteria, options).Wait()
	if err != nil && options != nil {
		searchData, err = c.UIDSearch(criteria, nil).Wait()
	}
	if err != nil {
		return nil, fmt.Errorf("imap search: %w", err)
	}

	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return []Email{}, nil
	}

	sort.Slice(uids, func(i, j int) bool {
		return uids[i] > uids[j]
	})
	if limit := searchLimit(query); len(uids) > int(limit) {
		uids = uids[:limit]
	}

	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(uid)
	}

	msgs, err := c.Fetch(uidSet, &imap.FetchOptions{
		Envelope: true,
		UID:      true,
		Flags:    true,
	}).Collect()
	if err != nil {
		return nil, fmt.Errorf("imap search fetch: %w", err)
	}

	emails := make([]Email, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Envelope == nil {
			continue
		}
		email := Email{
			UID:       uint32(msg.UID),
			Subject:   decodeHeader(msg.Envelope.Subject),
			Date:      msg.Envelope.Date,
			IsRead:    hasSeenFlag(msg.Flags),
			MessageID: msg.Envelope.MessageID,
			AccountID: account.ID,
		}
		if len(msg.Envelope.From) > 0 {
			email.From = formatAddress(msg.Envelope.From[0])
		}
		for _, addr := range msg.Envelope.To {
			email.To = append(email.To, addr.Addr())
		}
		for _, addr := range msg.Envelope.Cc {
			email.To = append(email.To, addr.Addr())
		}
		for _, addr := range msg.Envelope.ReplyTo {
			email.ReplyTo = append(email.ReplyTo, addr.Addr())
		}
		emails = append(emails, email)
	}
	sort.Slice(emails, func(i, j int) bool {
		return emails[i].UID > emails[j].UID
	})

	return emails, nil
}

func buildSearchCriteria(query backend.SearchQuery) *imap.SearchCriteria {
	criteria := &imap.SearchCriteria{}
	if query.From != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{Key: "From", Value: query.From})
	}
	if query.To != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{Key: "To", Value: query.To})
	}
	if query.Subject != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{Key: "Subject", Value: query.Subject})
	}
	if query.Body != "" {
		criteria.Body = []string{query.Body}
	}
	if !query.Since.IsZero() {
		criteria.Since = query.Since
	}
	if !query.Before.IsZero() {
		criteria.Before = query.Before
	}
	if query.LargerThan > 0 {
		criteria.Larger = int64(query.LargerThan)
	}
	return criteria
}

func searchLimit(query backend.SearchQuery) uint32 {
	if query.Limit > 0 {
		return query.Limit
	}
	return 100
}
