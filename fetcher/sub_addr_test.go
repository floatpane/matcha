package fetcher

import (
	"testing"

	"github.com/floatpane/matcha/config"
)

func TestAddressMatches(t *testing.T) {
	gmail := &config.Account{ServiceProvider: "gmail"}
	custom := &config.Account{ServiceProvider: "custom"}

	cases := []struct {
		name      string
		candidate string
		fetch     string
		account   *config.Account
		want      bool
	}{
		{"exact match", "user@gmail.com", "user@gmail.com", gmail, true},
		{"case insensitive", "User@Gmail.com", "user@gmail.com", gmail, true},
		{"whitespace", "  user@gmail.com  ", "user@gmail.com", gmail, true},
		{"gmail subaddress matches", "user+work@gmail.com", "user@gmail.com", gmail, true},
		{"gmail subaddress on configured side", "user@gmail.com", "user+work@gmail.com", gmail, true},
		{"gmail dots ignored", "u.s.e.r@gmail.com", "user@gmail.com", gmail, true},
		{"gmail dots on configured side", "user@gmail.com", "u.ser@gmail.com", gmail, true},
		{"gmail dots and subaddress combined", "u.ser+work@gmail.com", "user@gmail.com", gmail, true},
		{"different local rejected", "other@gmail.com", "user@gmail.com", gmail, false},
		{"different domain rejected", "user+x@example.com", "user@gmail.com", gmail, false},
		{"non-gmail provider ignores plus", "user+work@example.com", "user@example.com", custom, false},
		{"non-gmail provider keeps dots", "u.ser@example.com", "user@example.com", custom, false},
		{"empty candidate", "", "user@gmail.com", gmail, false},
		{"empty fetch", "user@gmail.com", "", gmail, false},
		{"nil account exact still works", "user@example.com", "user@example.com", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := addressMatches(tc.candidate, tc.fetch, tc.account); got != tc.want {
				t.Errorf("addressMatches(%q, %q) = %v, want %v", tc.candidate, tc.fetch, got, tc.want)
			}
		})
	}
}

func TestNormalizeGmailAddress(t *testing.T) {
	cases := map[string]string{
		"user+tag@gmail.com":     "user@gmail.com",
		"user@gmail.com":         "user@gmail.com",
		"user+a+b@example.com":   "user@example.com",
		"u.s.e.r@gmail.com":      "user@gmail.com",
		"u.ser+work@gmail.com":   "user@gmail.com",
		"first.last@example.com": "firstlast@example.com",
		"no-at-sign":             "no-at-sign",
		"":                       "",
	}
	for in, want := range cases {
		if got := normalizeGmailAddress(in); got != want {
			t.Errorf("normalizeGmailAddress(%q) = %q, want %q", in, got, want)
		}
	}
}
