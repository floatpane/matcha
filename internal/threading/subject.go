package threading

import (
	"regexp"
	"strings"
)

var subjectPrefixRE = regexp.MustCompile(`(?i)^(Re|Fwd|Fw|AW|WG|Tr|Reé|Resp|SV|VS|RV|ENC|Antw|Odp|R|I)\s*:\s*`)

func canonicalSubject(s string) string {
	s = strings.TrimSpace(s)
	for {
		next := subjectPrefixRE.ReplaceAllString(s, "")
		if next == s {
			break
		}
		s = strings.TrimSpace(next)
	}
	return strings.ToLower(strings.TrimSpace(s))
}
