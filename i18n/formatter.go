package i18n

import (
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// NumberFormatter formats numbers according to locale rules.
type NumberFormatter struct {
	locale  *Locale
	printer *message.Printer
}

// NewNumberFormatter creates a formatter for a locale.
func NewNumberFormatter(locale *Locale) *NumberFormatter {
	tag := locale.Tag
	if tag == language.Und {
		tag = language.English
	}

	return &NumberFormatter{
		locale:  locale,
		printer: message.NewPrinter(tag),
	}
}

// FormatInt formats an integer according to locale rules.
func (f *NumberFormatter) FormatInt(n int) string {
	return f.printer.Sprintf("%d", n)
}

// FormatInt64 formats an int64 according to locale rules.
func (f *NumberFormatter) FormatInt64(n int64) string {
	return f.printer.Sprintf("%d", n)
}

// FormatFloat formats a float64 with the specified precision.
func (f *NumberFormatter) FormatFloat(n float64, precision int) string {
	format := "%." + string(rune(precision+'0')) + "f"
	return f.printer.Sprintf(format, n)
}

// FormatPercent formats a number as a percentage (0.5 -> "50%").
func (f *NumberFormatter) FormatPercent(n float64) string {
	return f.printer.Sprintf("%.0f%%", n*100)
}

// FormatFileSize formats a byte count as a human-readable size.
func (f *NumberFormatter) FormatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return f.printer.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB", "PB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}

	return f.printer.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}
