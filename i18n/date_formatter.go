package i18n

import (
	"fmt"
	"time"
)

// DateFormatter formats dates and times according to locale rules.
type DateFormatter struct {
	locale *Locale
}

// NewDateFormatter creates a date formatter for a locale.
func NewDateFormatter(locale *Locale) *DateFormatter {
	return &DateFormatter{
		locale: locale,
	}
}

// FormatDate formats a time according to the given layout.
func (f *DateFormatter) FormatDate(t time.Time, layout string) string {
	return t.Format(layout)
}

// FormatTime formats just the time portion.
func (f *DateFormatter) FormatTime(t time.Time) string {
	return t.Format("15:04")
}

// FormatDateTime formats both date and time.
func (f *DateFormatter) FormatDateTime(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

// FormatRelative formats a time relative to now (e.g., "5 minutes ago").
// This should use translated strings from the message catalog.
func (f *DateFormatter) FormatRelative(t time.Time) string {
	now := time.Now()
	duration := now.Sub(t)

	// Future times
	if duration < 0 {
		duration = -duration
		return formatFutureDuration(duration)
	}

	// Past times
	return formatPastDuration(duration)
}

// formatPastDuration formats a duration as "X ago".
func formatPastDuration(d time.Duration) string {
	seconds := int(d.Seconds())
	minutes := seconds / 60
	hours := minutes / 60
	days := hours / 24

	switch {
	case seconds < 60:
		return "just now"
	case minutes == 1:
		return "1 minute ago"
	case minutes < 60:
		return fmt.Sprintf("%d minutes ago", minutes)
	case hours == 1:
		return "1 hour ago"
	case hours < 24:
		return fmt.Sprintf("%d hours ago", hours)
	case days == 1:
		return "1 day ago"
	case days < 7:
		return fmt.Sprintf("%d days ago", days)
	case days < 30:
		weeks := days / 7
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	case days < 365:
		months := days / 30
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := days / 365
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

// formatFutureDuration formats a duration as "in X".
func formatFutureDuration(d time.Duration) string {
	seconds := int(d.Seconds())
	minutes := seconds / 60
	hours := minutes / 60
	days := hours / 24

	switch {
	case seconds < 60:
		return "in a moment"
	case minutes == 1:
		return "in 1 minute"
	case minutes < 60:
		return fmt.Sprintf("in %d minutes", minutes)
	case hours == 1:
		return "in 1 hour"
	case hours < 24:
		return fmt.Sprintf("in %d hours", hours)
	case days == 1:
		return "in 1 day"
	case days < 7:
		return fmt.Sprintf("in %d days", days)
	default:
		return "in the future"
	}
}
