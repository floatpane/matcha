package clib

import "strings"

// SplitEmails splits a comma-separated list of email addresses,
// trimming whitespace and dropping empty entries.
func SplitEmails(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var res []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

// ParseEmailAddress extracts a display name and email from a string
// of the form "Name <email@example.com>". If no angle brackets are
// present the whole string is returned as the email.
func ParseEmailAddress(addr string) (name, email string) {
	addr = strings.TrimSpace(addr)
	if idx := strings.Index(addr, "<"); idx != -1 {
		name = strings.TrimSpace(addr[:idx])
		endIdx := strings.Index(addr, ">")
		if endIdx > idx {
			email = strings.TrimSpace(addr[idx+1 : endIdx])
		} else {
			email = strings.TrimSpace(addr[idx+1:])
		}
	} else {
		email = addr
	}
	return name, email
}
