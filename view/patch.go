package view

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	mailpatch "github.com/floatpane/go-mailpatch"
	"github.com/floatpane/matcha/theme"
	"github.com/floatpane/matcha/view/diffview"
)

// PatchInfo holds metadata about a detected patch in an email body.
type PatchInfo struct {
	// Subject is the cleaned patch subject (without [PATCH...] prefix).
	Subject string
	// Author is the patch author ("Name <email>").
	Author string
	// HasDiff is true when the email contains an actual diff.
	HasDiff bool
	// IsCoverLetter is true for "0/n" messages with no diff.
	IsCoverLetter bool
	// SeriesIndex is the patch position in a series (n in [PATCH n/m]).
	SeriesIndex int
	// SeriesTotal is the total patches in the series (m in [PATCH n/m]).
	SeriesTotal int
	// SeriesVersion is the series revision (1 default, 2 for v2).
	SeriesVersion int
	// Diff is the raw unified diff text.
	Diff string
	// CommitMessage is the commit message body (before the diffstat).
	CommitMessage string
	// DiffStatText is the raw diffstat block (lines between "---" and
	// "diff --git" in a format-patch), e.g. " main.go | 5 ++++\n 1 file
	// changed, 5 insertions(+)". May be empty.
	DiffStatText string
	// Stat holds the diffstat counts (files changed, additions, deletions).
	Stat mailpatch.DiffStat
}

// parseSubjectPrefix extracts the [PATCH ...] prefix info from a subject line.
func parseSubjectPrefix(subject string) (cleanSubject string, idx, total, version int) {
	cleanSubject = subject
	// Match [PATCH ...] or [RFC PATCH ...] prefixes.
	if !strings.HasPrefix(subject, "[") {
		return
	}
	end := strings.Index(subject, "]")
	if end < 0 {
		return
	}
	inner := subject[1:end]
	tokens := strings.Fields(inner)
	hasPatch := false
	for _, t := range tokens {
		if t == "PATCH" || t == "RFC" {
			hasPatch = true
		}
	}
	if !hasPatch {
		return
	}
	cleanSubject = strings.TrimSpace(subject[end+1:])
	for _, t := range tokens {
		if strings.HasPrefix(t, "v") && len(t) > 1 {
			v := 0
			for _, c := range t[1:] {
				if c >= '0' && c <= '9' {
					v = v*10 + int(c-'0')
				} else {
					v = 0
					break
				}
			}
			if v > 0 {
				version = v
			}
		}
		if strings.Contains(t, "/") {
			parts := strings.SplitN(t, "/", 2)
			if len(parts) == 2 {
				a, b := 0, 0
				for _, c := range parts[0] {
					if c >= '0' && c <= '9' {
						a = a*10 + int(c-'0')
					} else {
						a = 0
						break
					}
				}
				for _, c := range parts[1] {
					if c >= '0' && c <= '9' {
						b = b*10 + int(c-'0')
					} else {
						b = 0
						break
					}
				}
				if a > 0 && b > 0 {
					idx = a
					total = b
				}
			}
		}
	}
	return
}

// DetectPatch attempts to detect a git format-patch body in the given email
// body text. Unlike mailpatch.ParseBytes (which expects a full RFC 5322
// message with headers), this function works on the body text only — it
// looks for a "diff --git" line and uses SplitBodyDiff + ParseDiff to
// extract the commit message and structured diff.
//
// rawBody is the email body text (without headers).
// mimeType is the MIME type of the body.
// subject is the email subject (used to extract [PATCH n/m] prefix info).
// from is the email sender (used as the author).
func DetectPatch(rawBody, mimeType, subject, from string) *PatchInfo {
	// Only attempt patch detection on plain text bodies.
	if mimeType == BodyMIMETypeHTML {
		return nil
	}

	// Quick check: is there a diff in the body?
	if !strings.Contains(rawBody, "diff --git") {
		return nil
	}

	// Strip \r from \r\n line endings before splitting. The mailpatch
	// parser's separatorBefore check (TrimRight(line, " \t") == "---")
	// fails on "---\r", causing the diffstat block to leak into the
	// commit message — which then ends up in the git commit when the
	// patch is applied.
	cleanBody := strings.ReplaceAll(rawBody, "\r", "")

	// Split the body into commit message and diff.
	commitMsg, diff := mailpatch.SplitBodyDiff(cleanBody)
	if diff == "" {
		return nil
	}

	// Extract the diffstat block (lines between "---" and "diff --git").
	diffStatText := extractDiffStatText(cleanBody)

	// Parse the diff to get structured file changes and stats.
	files, err := mailpatch.ParseDiff(diff)
	if err != nil || len(files) == 0 {
		return nil
	}

	stat := mailpatch.DiffStat{}
	for _, f := range files {
		stat.FilesChanged++
		stat.Additions += f.Additions
		stat.Deletions += f.Deletions
	}

	// Parse subject prefix for series info.
	cleanSubject, idx, total, version := parseSubjectPrefix(subject)

	isCover := idx == 0 && total > 0

	return &PatchInfo{
		Subject:       cleanSubject,
		Author:        from,
		HasDiff:       true,
		IsCoverLetter: isCover,
		SeriesIndex:   idx,
		SeriesTotal:   total,
		SeriesVersion: version,
		Diff:          diff,
		CommitMessage: commitMsg,
		DiffStatText:  diffStatText,
		Stat:          stat,
	}
}

// RenderPatchBody renders an email body that contains a git patch. The
// commit message is rendered as normal text, and the diff portion is rendered
// as a solid block with background colors and line numbers. A patch metadata
// banner is prepended.
//
// availableWidth is the max width the content can occupy (e.g. the viewport
// width). The diff code box will fit inside this width.
//
// Returns the rendered body and true if a patch was detected and rendered.
// If no patch is detected, returns the original body and false.
func RenderPatchBody(rawBody, mimeType, subject, from string, availableWidth int) (string, bool) {
	info := DetectPatch(rawBody, mimeType, subject, from)
	if info == nil {
		return rawBody, false
	}

	var b strings.Builder

	// Patch metadata banner
	bannerStyle := patchBannerStyle()
	var banner strings.Builder
	banner.WriteString("📮 Git Patch\n")
	if info.Subject != "" {
		banner.WriteString("  Subject: " + info.Subject + "\n")
	}
	if info.Author != "" {
		banner.WriteString("  Author:  " + info.Author + "\n")
	}
	if info.SeriesTotal > 0 {
		version := ""
		if info.SeriesVersion > 1 {
			version = " v" + intToString(info.SeriesVersion)
		}
		banner.WriteString("  Series:  " + intToString(info.SeriesIndex) + "/" + intToString(info.SeriesTotal) + version + "\n")
	}
	if info.HasDiff && info.Stat.FilesChanged > 0 {
		statText := fmt.Sprintf("Files:   %d changed, %d insertions(+), %d deletions(-)",
			info.Stat.FilesChanged, info.Stat.Additions, info.Stat.Deletions)
		banner.WriteString("  " + highlightDiffStat(statText) + "\n")
	}
	if info.IsCoverLetter {
		banner.WriteString("  (Cover letter — no diff to apply)\n")
	}
	b.WriteString(bannerStyle.Render(banner.String()))
	b.WriteString("\n")

	// Commit message
	if info.CommitMessage != "" {
		b.WriteString(info.CommitMessage)
		b.WriteString("\n")
	}

	// Diffstat block (file table + summary)
	if info.DiffStatText != "" {
		b.WriteString("\n")
		b.WriteString(renderDiffStatBlock(info.DiffStatText))
		b.WriteString("\n")
	}

	// Render diff with background colors, line numbers, and truncation
	if info.Diff != "" {
		b.WriteString("\n")
		b.WriteString(renderDiffSection(info.Diff, availableWidth))
	}

	return b.String(), true
}

// renderDiffSection parses the raw unified diff text and renders it using
// the diffview package with background colors, line numbers, and proper
// ANSI-aware truncation.
func renderDiffSection(diffText string, availableWidth int) string {
	diffText = strings.TrimRight(diffText, "\n")
	diffText = strings.TrimLeft(diffText, "\n")
	if diffText == "" {
		return ""
	}

	files, err := mailpatch.ParseDiff(diffText)
	if err != nil || len(files) == 0 {
		return HighlightDiff(diffText)
	}

	// Account for the code box border and padding (4 chars total: 2 border + 2 padding)
	innerWidth := availableWidth - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	rendered := diffview.RenderUnified(files, innerWidth)

	label := codeLangStyle().Render(" DIFF ")
	content := label + "\n" + rendered

	return "\n" + codeBoxStyle().Render(content) + "\n"
}

func highlightDiffStat(text string) string {
	// Highlight: file count in cyan, insertions in green, deletions in red.
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF"))
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#56d364"))
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149"))

	// text format: "Files:   N changed, N insertions(+), N deletions(-)"
	parts := strings.Split(text, ", ")
	if len(parts) != 3 {
		return text
	}
	return fileStyle.Render(parts[0]) + ", " + addStyle.Render(parts[1]) + ", " + delStyle.Render(parts[2])
}

// extractDiffStatText extracts the raw diffstat block from a format-patch
// body — the lines between the "---" separator and the first "diff --git"
// line. Returns empty string if there is no diffstat.
func extractDiffStatText(body string) string {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	// Walk back to the "---" separator.
	sep := -1
	for i := start - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "---" {
			sep = i
			break
		}
	}
	if sep < 0 {
		return ""
	}
	statLines := lines[sep+1 : start]
	// Trim leading/trailing empty lines.
	for len(statLines) > 0 && strings.TrimSpace(statLines[0]) == "" {
		statLines = statLines[1:]
	}
	for len(statLines) > 0 && strings.TrimSpace(statLines[len(statLines)-1]) == "" {
		statLines = statLines[:len(statLines)-1]
	}
	if len(statLines) == 0 {
		return ""
	}
	return strings.Join(statLines, "\n")
}

// renderDiffStatBlock colorizes the raw diffstat block. Each line is a file
// entry like " main.go  | 5 ++++" or a summary like "2 files changed, 5
// insertions(+), 3 deletions(-)". File paths are rendered in the theme's
// secondary color, the bar separator in subtle text, additions in green,
// and deletions in red.
func renderDiffStatBlock(statText string) string {
	if statText == "" {
		return ""
	}
	t := theme.ActiveTheme
	pathStyle := lipgloss.NewStyle().Foreground(t.Secondary)
	barStyle := lipgloss.NewStyle().Foreground(t.SubtleText)
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#56d364"))
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149"))
	summaryStyle := lipgloss.NewStyle().Foreground(t.Secondary)

	lines := strings.Split(statText, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		// Summary line: "N files changed, N insertions(+), N deletions(-)"
		if isDiffStatSummary(line) {
			b.WriteString(highlightDiffStatSummary(line, summaryStyle, addStyle, delStyle))
			continue
		}
		// File line: " path/to/file | N +++++---"
		b.WriteString(highlightDiffStatFileLine(line, pathStyle, barStyle, addStyle, delStyle))
	}
	return b.String()
}

// isDiffStatSummary returns true for lines like "N files changed, ...".
func isDiffStatSummary(line string) bool {
	return strings.Contains(line, "file") && strings.Contains(line, "changed")
}

// highlightDiffStatSummary colorizes the summary line.
func highlightDiffStatSummary(line string, base, add, del lipgloss.Style) string {
	parts := strings.Split(line, ", ")
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString(", ")
		}
		switch {
		case strings.Contains(p, "insertion"):
			b.WriteString(add.Render(p))
		case strings.Contains(p, "deletion"):
			b.WriteString(del.Render(p))
		default:
			b.WriteString(base.Render(p))
		}
	}
	return b.String()
}

// highlightDiffStatFileLine colorizes a single file entry line.
func highlightDiffStatFileLine(line string, path, bar, add, del lipgloss.Style) string {
	pipeIdx := strings.Index(line, "|")
	if pipeIdx < 0 {
		return path.Render(line)
	}
	filePath := line[:pipeIdx]
	rest := line[pipeIdx:]

	var b strings.Builder
	b.WriteString(path.Render(filePath))

	// Find where the +/- bar starts (after the count number).
	barStart := -1
	for i := 1; i < len(rest); i++ {
		if rest[i] == '+' || rest[i] == '-' {
			barStart = i
			break
		}
	}
	if barStart < 0 {
		b.WriteString(bar.Render(rest))
		return b.String()
	}

	b.WriteString(bar.Render(rest[:barStart]))
	plusMinus := rest[barStart:]
	for _, c := range plusMinus {
		switch c {
		case '+':
			b.WriteString(add.Render("+"))
		case '-':
			b.WriteString(del.Render("-"))
		default:
			b.WriteString(bar.Render(string(c)))
		}
	}
	return b.String()
}

func patchBannerStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ActiveTheme.Accent).
		Padding(0, 1).
		MarginBottom(1)
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
