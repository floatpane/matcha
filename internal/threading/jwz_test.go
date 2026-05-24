package threading

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildThreeMessageChain(t *testing.T) {
	base := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	threads := Build([]EmailHeader{
		{ID: "<a@example>", Subject: "Foo", Date: base, EmailID: "1", Sender: "a"},
		{ID: "<b@example>", References: []string{"<a@example>"}, Subject: "Re: Foo", Date: base.Add(time.Minute), EmailID: "2", Sender: "b"},
		{ID: "<c@example>", References: []string{"<a@example>", "<b@example>"}, Subject: "Re: Re: Foo", Date: base.Add(2 * time.Minute), EmailID: "3", Sender: "c"}, //nolint:dupword
	})

	if len(threads) != 1 {
		t.Fatalf("got %d threads, want 1", len(threads))
	}
	if threads[0].Count != 3 {
		t.Fatalf("got count %d, want 3", threads[0].Count)
	}
	if got := threads[0].Root.Children[0].Children[0].EmailID; got != "3" {
		t.Fatalf("got chain leaf %q, want 3", got)
	}
}

func TestBuildForkedThread(t *testing.T) {
	base := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	threads := Build([]EmailHeader{
		{ID: "<a@example>", Subject: "Foo", Date: base, EmailID: "1"},
		{ID: "<c@example>", References: []string{"<a@example>"}, Subject: "Re: Foo", Date: base.Add(2 * time.Minute), EmailID: "3"},
		{ID: "<b@example>", References: []string{"<a@example>"}, Subject: "Re: Foo", Date: base.Add(time.Minute), EmailID: "2"},
	})

	if len(threads) != 1 {
		t.Fatalf("got %d threads, want 1", len(threads))
	}
	children := threads[0].Root.Children
	if len(children) != 2 {
		t.Fatalf("got %d children, want 2", len(children))
	}
	if children[0].EmailID != "2" || children[1].EmailID != "3" {
		t.Fatalf("got child order %q, %q; want 2, 3", children[0].EmailID, children[1].EmailID)
	}
}

func TestBuildMissingParentPlaceholderRoot(t *testing.T) {
	base := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	threads := Build([]EmailHeader{
		{ID: "<child@example>", References: []string{"<missing@example>"}, Subject: "Re: Foo", Date: base, EmailID: "child"},
		{ID: "<other@example>", References: []string{"<missing@example>"}, Subject: "Re: Foo", Date: base.Add(time.Minute), EmailID: "other"},
	})

	if len(threads) != 1 {
		t.Fatalf("got %d threads, want 1", len(threads))
	}
	if threads[0].Root.EmailID != "" {
		t.Fatalf("got root EmailID %q, want placeholder", threads[0].Root.EmailID)
	}
	if len(threads[0].Root.Children) != 2 {
		t.Fatalf("got %d placeholder children, want 2", len(threads[0].Root.Children))
	}
}

func TestBuildSubjectFallbackGroupingForOrphans(t *testing.T) {
	base := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	threads := Build([]EmailHeader{
		{ID: "<a@example>", Subject: "Re: Foo", Date: base, EmailID: "1"},
		{ID: "<b@example>", Subject: "Fwd: foo", Date: base.Add(time.Minute), EmailID: "2"},
		{ID: "<c@example>", Subject: "Bar", Date: base.Add(2 * time.Minute), EmailID: "3"},
	})

	if len(threads) != 2 {
		t.Fatalf("got %d threads, want 2", len(threads))
	}
	var grouped Thread
	for _, thread := range threads {
		if thread.Subject == "foo" {
			grouped = thread
			break
		}
	}
	if grouped.Count != 2 {
		t.Fatalf("got grouped count %d, want 2", grouped.Count)
	}
}

func TestBuildSubjectFallbackGroupsLocalePrefixes(t *testing.T) {
	base := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	threads := Build([]EmailHeader{
		{ID: "<a@example>", Subject: "Foo", Date: base, EmailID: "1"},
		{ID: "<b@example>", Subject: "SV: Foo", Date: base.Add(time.Minute), EmailID: "2"},
		{ID: "<c@example>", Subject: "RV: Foo", Date: base.Add(2 * time.Minute), EmailID: "3"},
		{ID: "<d@example>", Subject: "Antw: Foo", Date: base.Add(3 * time.Minute), EmailID: "4"},
	})

	if len(threads) != 1 {
		t.Fatalf("got %d threads, want 1", len(threads))
	}
	if threads[0].Subject != "foo" {
		t.Fatalf("got subject %q, want foo", threads[0].Subject)
	}
	if threads[0].Count != 4 {
		t.Fatalf("got grouped count %d, want 4", threads[0].Count)
	}
}

func TestBuildEmptyReferencesList(t *testing.T) {
	threads := Build([]EmailHeader{
		{ID: "<a@example>", References: nil, Subject: "Foo", Date: time.Now(), EmailID: "1"},
	})

	if len(threads) != 1 {
		t.Fatalf("got %d threads, want 1", len(threads))
	}
	if threads[0].Root.EmailID != "1" {
		t.Fatalf("got root %q, want 1", threads[0].Root.EmailID)
	}
}

func TestBuildStableOrderingAcrossCalls(t *testing.T) {
	base := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	headers := []EmailHeader{
		{ID: "<a@example>", Subject: "Foo", Date: base, EmailID: "1"},
		{ID: "<b@example>", Subject: "Bar", Date: base, EmailID: "2"},
		{ID: "<c@example>", References: []string{"<a@example>"}, Subject: "Re: Foo", Date: base, EmailID: "3"},
	}

	first := Build(headers)
	second := Build(headers)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Build output differed across calls:\n%#v\n%#v", first, second)
	}
}

func TestCanonicalSubjectNormalizesReplyAndForwardPrefixes(t *testing.T) {
	tests := map[string]string{
		"Re: Re: Foo":     "foo", //nolint:dupword
		"Fwd: FW: Foo":    "foo",
		"AW: WG: Tr: Foo": "foo",
		"Reé: Resp: Foo":  "foo",
		"SV: VS: RV: Foo": "foo",
		"ENC: Antw: Foo":  "foo",
		"Odp: R: I: Foo":  "foo",
		"  Foo  ":         "foo",
	}

	for in, want := range tests {
		if got := canonicalSubject(in); got != want {
			t.Fatalf("canonicalSubject(%q) = %q, want %q", in, got, want)
		}
	}
}
