package tui

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	overlay "github.com/floatpane/bubble-overlay"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/theme"
)

// labelOverlayStyle matches the move overlay styling.
var labelOverlayStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#25A065")).
	Padding(1, 2)

var labelOverlayTitleStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("42")).
	Bold(true).
	PaddingBottom(1)

// labelMeta holds the display icon, foreground, and background color for a
// Gmail label.
type labelMeta struct {
	icon       string
	foreground color.Color
	background color.Color
}

// gmailLabelColors maps known Gmail system labels to their Gmail-style colors,
// background colors, and unicode icons. Keys are the canonical label names
// WITHOUT any leading backslash.
var gmailLabelColors = map[string]labelMeta{
	"Inbox":     {"\uF01C", lipgloss.Color("#1a73e8"), lipgloss.Color("#e8f0fe")},
	"Starred":   {"\u2605", lipgloss.Color("#b45f06"), lipgloss.Color("#fce8b2")},
	"Important": {"\u26A1", lipgloss.Color("#c35a00"), lipgloss.Color("#fedfc2")},
	"Sent":      {"\uF1D8", lipgloss.Color("#1e8e3e"), lipgloss.Color("#ceead6")},
	"Drafts":    {"\uF040", lipgloss.Color("#5f6368"), lipgloss.Color("#e8eaed")},
	"Trash":     {"\uF1F8", lipgloss.Color("#d93025"), lipgloss.Color("#fad2cf")},
	"Spam":      {"\uF071", lipgloss.Color("#9aa0a6"), lipgloss.Color("#e8eaed")},
	"All":       {"\uF0C0", lipgloss.Color("#1a73e8"), lipgloss.Color("#e8f0fe")},
}

// Color palette for user labels (non-system). Each gets a deterministic
// foreground/background pair based on a hash of the label name.
var userLabelPalette = []labelMeta{
	{"", lipgloss.Color("#1a73e8"), lipgloss.Color("#e8f0fe")}, // blue
	{"", lipgloss.Color("#9334e6"), lipgloss.Color("#f3e8fd")}, // purple
	{"", lipgloss.Color("#e8710a"), lipgloss.Color("#feefe3")}, // orange
	{"", lipgloss.Color("#1e8e3e"), lipgloss.Color("#ceead6")}, // green
	{"", lipgloss.Color("#d93025"), lipgloss.Color("#fad2cf")}, // red
	{"", lipgloss.Color("#c5221f"), lipgloss.Color("#fad2cf")}, // dark red
	{"", lipgloss.Color("#b45f06"), lipgloss.Color("#fce8b2")}, // yellow
	{"", lipgloss.Color("#137333"), lipgloss.Color("#ceead6")}, // dark green
	{"", lipgloss.Color("#8430ce"), lipgloss.Color("#f3e8fd")}, // dark purple
	{"", lipgloss.Color("#b31412"), lipgloss.Color("#fad2cf")}, // brick
	{"", lipgloss.Color("#185abc"), lipgloss.Color("#e8f0fe")}, // dark blue
	{"", lipgloss.Color("#b80672"), lipgloss.Color("#fde7f3")}, // pink
}

// labelMetaFor returns the icon, foreground, and background for a given Gmail
// label. System labels have fixed styling; user labels get a deterministic
// pair from a hash of the name. The label may or may not have leading
// backslashes (legacy cache data sometimes contains them).
func labelMetaFor(label string) labelMeta {
	stripped := stripBackslashes(label)
	if meta, ok := gmailLabelColors[stripped]; ok {
		return meta
	}
	// User label: deterministic color from hash
	h := fnv1aHash(stripped)
	meta := userLabelPalette[int(h)%len(userLabelPalette)]
	meta.icon = "\uF02B" // tag icon
	return meta
}

// stripBackslashes removes all leading backslashes from a Gmail label name.
func stripBackslashes(s string) string {
	for strings.HasPrefix(s, "\\") {
		s = strings.TrimPrefix(s, "\\")
	}
	return s
}

// fnv1aHash returns a 32-bit FNV-1a hash of the given string.
func fnv1aHash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// displayLabelName returns the canonical display name for a label: all leading
// backslashes are stripped.
func displayLabelName(label string) string {
	return stripBackslashes(label)
}

// renderLabelTags renders a list of labels as a single space-separated string of
// background-colored pill tags.
func renderLabelTags(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	var parts []string
	seen := make(map[string]bool)
	for _, l := range labels {
		name := displayLabelName(l)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		parts = append(parts, renderLabelTag(name))
	}
	return strings.Join(parts, " ")
}

// renderLabelTag renders a single label as a background-colored pill tag.
func renderLabelTag(label string) string {
	meta := labelMetaFor(label)
	name := displayLabelName(label)
	style := lipgloss.NewStyle().
		Foreground(meta.foreground).
		Background(meta.background).
		Padding(0, 1)
	return style.Render(meta.icon + " " + name)
}

// --- Label overlay state (embedded in FolderInbox) ---

// LabelOverlayState holds the state for the Gmail label editing overlay. It is
// embedded in FolderInbox and rendered as a bubble-overlay on top of the inbox
// content, the same way the move-to-folder overlay works.
type LabelOverlayState struct {
	email     fetcher.Email
	account   config.Account
	folder    string
	available []string // all known labels
	filtered  []string // labels matching the filter
	selected  int      // cursor position
	input     textinput.Model
	active    bool
}

// NewLabelOverlayState creates a label overlay state for the given email.
func NewLabelOverlayState(email fetcher.Email, account config.Account, folder string) LabelOverlayState {
	ti := textinput.New()
	ti.Placeholder = "Type a label name to add or toggle..."
	ti.Prompt = "> "
	ti.CharLimit = 80
	ti.SetStyles(ThemedTextInputStyles())
	ti.Focus()

	available := collectKnownLabels(email)

	return LabelOverlayState{
		email:     email,
		account:   account,
		folder:    folder,
		available: available,
		filtered:  available,
		input:     ti,
		active:    true,
	}
}

// UpdateLabelOverlay handles input for the label overlay. It returns the
// updated state, a tea.Cmd, and a bool indicating whether the overlay was
// dismissed (esc pressed).
func (s LabelOverlayState) UpdateLabelOverlay(msg tea.Msg) (LabelOverlayState, tea.Cmd, bool) { //nolint:gocyclo
	kb := config.Keybinds
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case kb.Global.Cancel:
			s.active = false
			return s, nil, true
		case keyEnter:
			input := strings.TrimSpace(s.input.Value())
			if input != "" {
				label := input
				s.active = false
				return s, func() tea.Msg {
					return GmailLabelModifiedMsg{
						UID:       s.email.UID,
						AccountID: s.email.AccountID,
						Folder:    s.folder,
						Label:     label,
						Add:       !s.hasLabel(label),
					}
				}, true
			}
			if len(s.filtered) > 0 && s.selected < len(s.filtered) {
				label := s.filtered[s.selected]
				s.active = false
				return s, func() tea.Msg {
					return GmailLabelModifiedMsg{
						UID:       s.email.UID,
						AccountID: s.email.AccountID,
						Folder:    s.folder,
						Label:     label,
						Add:       !s.hasLabel(label),
					}
				}, true
			}
			return s, nil, false
		case "up", kb.Global.NavUp:
			if len(s.filtered) > 0 {
				s.selected--
				if s.selected < 0 {
					s.selected = len(s.filtered) - 1
				}
			}
			return s, nil, false
		case keyDown, kb.Global.NavDown:
			if len(s.filtered) > 0 {
				s.selected++
				if s.selected >= len(s.filtered) {
					s.selected = 0
				}
			}
			return s, nil, false
		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			s.applyFilter()
			s.selected = 0
			return s, cmd, false
		}
	}
	return s, nil, false
}

// RenderLabelOverlay renders the label overlay box on top of the given content
// using bubble-overlay's Center compositor.
func (s LabelOverlayState) RenderLabelOverlay(content string, width, height int) string {
	var b strings.Builder

	title := "Gmail Labels"
	b.WriteString(labelOverlayTitleStyle.Render(title))
	b.WriteString("\n")

	// Show current labels
	if len(s.email.Labels) > 0 {
		b.WriteString("Current: ")
		for i, l := range s.email.Labels {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(renderLabelTag(l))
		}
		b.WriteString("\n\n")
	}

	// Show filter input
	s.input.SetWidth(50)
	b.WriteString(s.input.View())
	b.WriteString("\n\n")

	// Show filtered list
	if len(s.filtered) > 0 {
		maxVisible := 8
		startIdx := 0
		if s.selected >= maxVisible {
			startIdx = s.selected - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(s.filtered) {
			endIdx = len(s.filtered)
		}

		for i := startIdx; i < endIdx; i++ {
			label := s.filtered[i]
			prefix := "  "
			if i == s.selected {
				prefix = "> "
			}

			marker := "  "
			if s.hasLabel(label) {
				marker = "✓ "
			}

			labelPart := renderLabelTag(label)

			b.WriteString(prefix + marker + labelPart)
			if i < endIdx-1 {
				b.WriteString("\n")
			}
		}
	} else if strings.TrimSpace(s.input.Value()) != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(theme.ActiveTheme.MutedText).Render(
			fmt.Sprintf("Press enter to add new label: \"%s\"", strings.TrimSpace(s.input.Value()))))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("enter: toggle/add • j/k: navigate • esc: cancel"))

	box := labelOverlayStyle.Render(b.String())
	return overlay.Center(content, box, width, height)
}

// hasLabel reports whether the email currently has the given label.
func (s LabelOverlayState) hasLabel(label string) bool {
	stripped := strings.TrimPrefix(label, "\\")
	for _, l := range s.email.Labels {
		existing := strings.TrimPrefix(l, "\\")
		if strings.EqualFold(existing, stripped) {
			return true
		}
	}
	return false
}

// applyFilter filters the available labels based on the input text.
func (s *LabelOverlayState) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(s.input.Value()))
	if query == "" {
		s.filtered = s.available
		return
	}

	var filtered []string
	for _, l := range s.available {
		name := strings.ToLower(displayLabelName(l))
		if strings.Contains(name, query) {
			filtered = append(filtered, l)
		}
	}
	s.filtered = filtered
}

// collectKnownLabels returns the list of labels currently on the email plus
// common Gmail system labels, so the user can toggle them.
func collectKnownLabels(email fetcher.Email) []string {
	seen := make(map[string]bool)
	var labels []string

	// Common Gmail system labels (stored without backslash)
	systemLabels := []string{
		"Inbox",
		"Starred",
		"Important",
		"Sent",
		"Drafts",
		"Trash",
		"Spam",
	}

	for _, l := range systemLabels {
		if !seen[l] {
			labels = append(labels, l)
			seen[l] = true
		}
	}

	// Add labels from the current email
	for _, l := range email.Labels {
		stripped := strings.TrimPrefix(l, "\\")
		if !seen[stripped] {
			labels = append(labels, stripped)
			seen[stripped] = true
		}
	}

	return labels
}

// IsGmailAccount reports whether the given account is configured as a Gmail
// provider. Used by TUI views to gate label-related UI.
func IsGmailAccount(account *config.Account) bool {
	return account != nil && strings.EqualFold(account.ServiceProvider, config.ProviderGmail)
}

// IsRecipientOfEmail reports whether the given email address appears in the
// To, Cc, or Bcc recipients of the email. For Gmail accounts, subaddress and
// dot variants also match.
func IsRecipientOfEmail(email fetcher.Email, addr string, account *config.Account) bool {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" {
		return false
	}
	for _, to := range email.To {
		candidate := extractEmailAddress(to)
		if addressMatchesGmail(candidate, addr, account) {
			return true
		}
	}
	return false
}

// addressMatchesGmail checks if candidate matches target, using Gmail
// normalization for Gmail accounts.
func addressMatchesGmail(candidate, target string, account *config.Account) bool {
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	if candidate == "" || target == "" {
		return false
	}
	if candidate == target {
		return true
	}
	if account != nil && strings.EqualFold(account.ServiceProvider, config.ProviderGmail) {
		return normalizeGmailAddressPub(candidate) == normalizeGmailAddressPub(target)
	}
	return false
}

// normalizeGmailAddressPub canonicalizes a Gmail address by stripping the
// "+tag" subaddress and removing dots from the local part.
func normalizeGmailAddressPub(addr string) string {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return addr
	}
	local, domain := addr[:at], addr[at:]
	if plus := strings.Index(local, "+"); plus >= 0 {
		local = local[:plus]
	}
	local = strings.ReplaceAll(local, ".", "")
	return local + domain
}
