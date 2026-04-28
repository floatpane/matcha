package backend

import (
	"testing"
	"time"
)

func TestParseSearchQuery(t *testing.T) {
	q := ParseSearchQuery(`from:alice@example.com to:bob@example.com subject:report body:revenue since:2026-01-01 before:2026-02-01 larger:10240`)
	if q.From != "alice@example.com" || q.To != "bob@example.com" || q.Subject != "report" || q.Body != "revenue" {
		t.Fatalf("parsed fields = %+v", q)
	}
	if !q.Since.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !q.Before.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("parsed dates = since:%v before:%v", q.Since, q.Before)
	}
	if q.LargerThan != 10240 || q.Raw == "" {
		t.Fatalf("parsed size/raw = larger:%d raw:%q", q.LargerThan, q.Raw)
	}
}

func TestParseSearchQueryBareTerms(t *testing.T) {
	if got := ParseSearchQuery("quarterly revenue update").Body; got != "quarterly revenue update" {
		t.Fatalf("Body = %q", got)
	}
	if got := ParseSearchQuery("from:alice@example.com quarterly revenue").Body; got != "" {
		t.Fatalf("fielded search Body = %q, want empty", got)
	}
}
