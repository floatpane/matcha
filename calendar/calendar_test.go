package calendar

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseICS_Simple(t *testing.T) {
	data, err := os.ReadFile("../testdata/invites/simple.ics")
	if err != nil {
		t.Fatalf("Failed to read test fixture: %v", err)
	}

	event, err := ParseICS(data)
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}

	if event.UID != "test-event-123@example.com" {
		t.Errorf("Expected UID test-event-123@example.com, got %s", event.UID)
	}

	if event.Summary != "Q2 Planning Meeting" {
		t.Errorf("Expected summary 'Q2 Planning Meeting', got %s", event.Summary)
	}

	if event.Location != "Conference Room A" {
		t.Errorf("Expected location 'Conference Room A', got %s", event.Location)
	}

	if event.Organizer != "alice@company.com" {
		t.Errorf("Expected organizer alice@company.com, got %s", event.Organizer)
	}

	if event.Status != "CONFIRMED" {
		t.Errorf("Expected status CONFIRMED, got %s", event.Status)
	}

	if event.Method != "REQUEST" {
		t.Errorf("Expected method REQUEST, got %s", event.Method)
	}

	expectedStart := time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC)
	if !event.Start.Equal(expectedStart) {
		t.Errorf("Expected start %v, got %v", expectedStart, event.Start)
	}

	expectedEnd := time.Date(2026, 4, 21, 15, 30, 0, 0, time.UTC)
	if !event.End.Equal(expectedEnd) {
		t.Errorf("Expected end %v, got %v", expectedEnd, event.End)
	}
}

func TestParseICS_NoEvent(t *testing.T) {
	data := []byte(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//Test//EN
END:VCALENDAR`)

	_, err := ParseICS(data)
	if err == nil {
		t.Error("Expected error for calendar with no VEVENT")
	}
	if !strings.Contains(err.Error(), "no VEVENT") {
		t.Errorf("Expected 'no VEVENT' error, got: %v", err)
	}
}

func TestParseICS_Malformed(t *testing.T) {
	data := []byte(`INVALID ICAL DATA`)

	_, err := ParseICS(data)
	if err == nil {
		t.Error("Expected error for malformed iCalendar data")
	}
}

func TestGenerateRSVP(t *testing.T) {
	data, err := os.ReadFile("../testdata/invites/simple.ics")
	if err != nil {
		t.Fatalf("Failed to read test fixture: %v", err)
	}

	responses := []string{"ACCEPTED", "DECLINED", "TENTATIVE"}

	for _, response := range responses {
		t.Run(response, func(t *testing.T) {
			rsvpData, err := GenerateRSVP(data, "bob@company.com", response)
			if err != nil {
				t.Fatalf("GenerateRSVP failed for %s: %v", response, err)
			}

			rsvpStr := string(rsvpData)

			// Check METHOD:REPLY is set
			if !strings.Contains(rsvpStr, "METHOD:REPLY") {
				t.Error("Expected METHOD:REPLY in RSVP")
			}

			// Check PARTSTAT is updated
			if !strings.Contains(rsvpStr, "PARTSTAT="+response) {
				t.Errorf("Expected PARTSTAT=%s in RSVP", response)
			}

			// RFC 6047: only the responding attendee should remain
			attendeeCount := strings.Count(rsvpStr, "ATTENDEE")
			if attendeeCount != 1 {
				t.Errorf("Expected exactly 1 ATTENDEE in RSVP, got %d", attendeeCount)
			}

			// Should contain responding user's email
			if !strings.Contains(rsvpStr, "bob@company.com") {
				t.Error("Expected bob@company.com in RSVP attendee")
			}

			// Should NOT contain other attendees
			if strings.Contains(rsvpStr, "carol@company.com") {
				t.Error("RSVP should not contain other attendees")
			}

			// Verify it's still valid iCalendar
			_, err = ParseICS(rsvpData)
			if err != nil {
				t.Errorf("Generated RSVP is not valid iCalendar: %v", err)
			}
		})
	}
}

// buildICS wraps DTSTART/DTEND lines into a minimal VCALENDAR for ParseICS.
func buildICS(dtstart, dtend string) []byte {
	return []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Test//Test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:date-only@example.com\r\n" +
		"DTSTAMP:20260415T120000Z\r\n" +
		dtstart + "\r\n" +
		dtend + "\r\n" +
		"SUMMARY:Test\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")
}

func TestParseICS_DateOnly(t *testing.T) {
	wantStart := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		dtstart string
		dtend   string
	}{
		{
			name:    "VALUE=DATE without TZID",
			dtstart: "DTSTART;VALUE=DATE:20260421",
			dtend:   "DTEND;VALUE=DATE:20260422",
		},
		{
			// Regression: TZID present on a date-only value must be ignored
			// (RFC 5545 forbids TZID with VALUE=DATE; some producers emit it anyway).
			name:    "VALUE=DATE with TZID is ignored",
			dtstart: "DTSTART;TZID=America/New_York;VALUE=DATE:20260421",
			dtend:   "DTEND;TZID=America/New_York;VALUE=DATE:20260422",
		},
		{
			// Shape-only detection: no VALUE param, but YYYYMMDD value with TZID.
			name:    "YYYYMMDD shape with TZID is treated as date-only",
			dtstart: "DTSTART;TZID=America/Los_Angeles:20260421",
			dtend:   "DTEND;TZID=America/Los_Angeles:20260422",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseICS(buildICS(tt.dtstart, tt.dtend))
			if err != nil {
				t.Fatalf("ParseICS failed: %v", err)
			}
			if !event.Start.Equal(wantStart) {
				t.Errorf("Start = %v, want %v", event.Start.UTC(), wantStart)
			}
			if !event.End.Equal(wantEnd) {
				t.Errorf("End = %v, want %v", event.End.UTC(), wantEnd)
			}
		})
	}
}

func TestParseICS_TimedWithTZID(t *testing.T) {
	// Existing behavior: timed values with TZID keep their zone semantics.
	event, err := ParseICS(buildICS(
		"DTSTART;TZID=America/New_York:20260421T140000",
		"DTEND;TZID=America/New_York:20260421T153000",
	))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("America/New_York unavailable on this system: %v", err)
	}
	wantStart := time.Date(2026, 4, 21, 14, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 4, 21, 15, 30, 0, 0, loc)

	if !event.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", event.Start, wantStart)
	}
	if !event.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", event.End, wantEnd)
	}
}

func TestExtractEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"mailto:user@example.com", "user@example.com"},
		{"MAILTO:user@example.com", "user@example.com"},
		{"CN=John Doe:user@example.com", "user@example.com"},
		{"user@example.com", "user@example.com"},
		{"  user@example.com  ", "user@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractEmail(tt.input)
			if result != tt.expected {
				t.Errorf("extractEmail(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
