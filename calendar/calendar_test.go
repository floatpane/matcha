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
