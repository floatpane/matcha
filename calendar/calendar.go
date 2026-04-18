package calendar

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// Event represents a parsed calendar event from an .ics attachment
type Event struct {
	UID         string
	Summary     string // Event title
	Description string
	Location    string
	Start       time.Time
	End         time.Time
	Organizer   string // Organizer email
	Status      string // CONFIRMED, TENTATIVE, CANCELLED
	Method      string // REQUEST, REPLY, CANCEL
}

// ParseICS extracts the first VEVENT from .ics data
func ParseICS(data []byte) (*Event, error) {
	cal, err := ics.ParseCalendar(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse calendar: %w", err)
	}

	events := cal.Events()
	if len(events) == 0 {
		return nil, fmt.Errorf("no VEVENT found")
	}

	vevent := events[0]

	// Extract properties
	uid := getEventProperty(vevent, ics.ComponentPropertyUniqueId)
	summary := getEventProperty(vevent, ics.ComponentPropertySummary)
	description := getEventProperty(vevent, ics.ComponentPropertyDescription)
	location := getEventProperty(vevent, ics.ComponentPropertyLocation)
	organizer := extractEmail(getEventProperty(vevent, ics.ComponentPropertyOrganizer))
	status := getEventProperty(vevent, ics.ComponentPropertyStatus)

	// Get METHOD from calendar level
	method := ""
	for _, prop := range cal.CalendarProperties {
		if prop.IANAToken == string(ics.PropertyMethod) {
			method = prop.Value
			break
		}
	}

	// Parse timestamps
	start, _ := parseEventTimestamp(vevent, ics.ComponentPropertyDtStart)
	end, _ := parseEventTimestamp(vevent, ics.ComponentPropertyDtEnd)

	return &Event{
		UID:         uid,
		Summary:     summary,
		Description: description,
		Location:    location,
		Start:       start,
		End:         end,
		Organizer:   organizer,
		Status:      status,
		Method:      method,
	}, nil
}

// GenerateRSVP creates a RFC 6047 (iMIP) compliant reply .ics.
// Google Calendar requires:
// - METHOD:REPLY at calendar level
// - Only the responding attendee in VEVENT (others removed)
// - Updated PARTSTAT on the attendee
// - Current DTSTAMP
func GenerateRSVP(originalData []byte, userEmail, response string) ([]byte, error) {
	// response: "ACCEPTED", "DECLINED", "TENTATIVE"

	cal, err := ics.ParseCalendar(bytes.NewReader(originalData))
	if err != nil {
		return nil, fmt.Errorf("parse calendar: %w", err)
	}

	// Set METHOD:REPLY
	cal.SetMethod(ics.MethodReply)

	userEmail = strings.ToLower(strings.TrimSpace(userEmail))

	for _, vevent := range cal.Events() {
		// Update DTSTAMP to current time
		vevent.SetDtStampTime(time.Now().UTC())

		// Find the responding attendee and remove all others
		var matchedAttendee *ics.Attendee
		attendees := vevent.Attendees()
		for _, attendee := range attendees {
			attendeeEmail := strings.ToLower(extractEmail(attendee.Email()))
			if strings.Contains(attendeeEmail, userEmail) || strings.Contains(userEmail, attendeeEmail) {
				matchedAttendee = attendee
				break
			}
		}

		// Remove all ATTENDEE properties
		vevent.RemoveProperty(ics.ComponentPropertyAttendee)

		// Re-add only the responding attendee with updated PARTSTAT and RSVP=TRUE
		if matchedAttendee != nil {
			matchedAttendee.ICalParameters[string(ics.ParameterParticipationStatus)] = []string{response}
			matchedAttendee.ICalParameters["RSVP"] = []string{"TRUE"}
			vevent.Properties = append(vevent.Properties, matchedAttendee.IANAProperty)
		} else {
			// Attendee not found in original - add ourselves with full parameters
			vevent.AddAttendee("mailto:"+userEmail,
				ics.WithRSVP(true),
				ics.ParticipationStatus(ics.ParticipationStatusNeedsAction),
				ics.CalendarUserTypeIndividual,
				ics.ParticipationRoleReqParticipant,
			)
			for _, att := range vevent.Attendees() {
				att.ICalParameters[string(ics.ParameterParticipationStatus)] = []string{response}
			}
		}
	}

	return []byte(cal.Serialize()), nil
}

// getEventProperty extracts a property value from a VEVENT
func getEventProperty(vevent *ics.VEvent, prop ics.ComponentProperty) string {
	p := vevent.GetProperty(prop)
	if p == nil {
		return ""
	}
	return p.Value
}

// parseEventTimestamp parses DTSTART or DTEND with timezone handling
func parseEventTimestamp(vevent *ics.VEvent, prop ics.ComponentProperty) (time.Time, error) {
	p := vevent.GetProperty(prop)
	if p == nil {
		return time.Time{}, fmt.Errorf("property not found")
	}

	value := p.Value
	var tzid string
	if params := p.ICalParameters; params != nil {
		if tzids := params["TZID"]; len(tzids) > 0 {
			tzid = tzids[0]
		}
	}

	// Try parsing with timezone
	var t time.Time
	var err error

	// RFC 5545 formats
	formats := []string{
		"20060102T150405Z", // UTC
		"20060102T150405",  // Local/TZID
		"20060102",         // Date only (all-day)
		time.RFC3339,       // Fallback
	}

	for _, format := range formats {
		t, err = time.Parse(format, value)
		if err == nil {
			break
		}
	}

	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp: %w", err)
	}

	// Apply timezone if specified
	if tzid != "" && !strings.HasSuffix(value, "Z") {
		if loc, locErr := time.LoadLocation(tzid); locErr == nil {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
		}
	}

	return t, nil
}

// extractEmail strips "mailto:" prefix and CN parameter from organizer/attendee fields
func extractEmail(mailto string) string {
	// Strip mailto: prefix
	email := strings.TrimPrefix(mailto, "mailto:")
	email = strings.TrimPrefix(email, "MAILTO:")

	// Strip CN and other parameters (format: CN=Name:email@example.com)
	if idx := strings.Index(email, ":"); idx != -1 {
		email = email[idx+1:]
	}

	return strings.TrimSpace(email)
}
