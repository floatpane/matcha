package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	overlay "github.com/floatpane/bubble-overlay"
	"github.com/floatpane/matcha/theme"
)

const (
	setupStepWelcome  = iota
	setupStepFeatures // what Matcha can do
	setupStepHelper   // macOS-only
	setupStepMailto   // macOS + Linux
	setupStepShowcase // interactive inbox demo
	setupStepMouse    // ask about mouse support
	setupStepDone
)

type setupActionStatus int

const (
	setupStatusPending setupActionStatus = iota
	setupStatusRunning
	setupStatusDone
	setupStatusError
	setupStatusSkipped
)

// SetupGuide is the first-run wizard shown to new users.
type SetupGuide struct {
	step    int
	isMac   bool
	isLinux bool
	width   int
	height  int

	// Per-step statuses
	helperStatus setupActionStatus
	helperErr    string
	mailtoStatus setupActionStatus
	mailtoErr    string

	// Cursor for yes/no options (0 = Yes, 1 = Skip)
	cursor int

	// Showcase step state
	showcaseCursor   int // selected row in fake inbox
	showcaseMode     int // 0 = inbox list, 1 = email detail
	showcaseTourStep int // 0-4, which tooltip is showing

	// Spinner for background ops
	spin spinner.Model

	// Callbacks injected from main
	doInstallHelper func() error
	doSetupMailto   func() error
}

type demoEmail struct {
	from     string
	subject  string
	date     string
	isRead   bool
	bodyFrom string
	body     string
}

var demoEmails = []demoEmail{
	{
		from: "Sarah Chen", subject: "Q4 roadmap — let's sync",
		date: "10:24 AM", isRead: false,
		bodyFrom: "Sarah Chen <sarah@work.io>",
		body:     "Hey,\n\nHere's a quick summary of where we landed on the Q4\nroadmap. A few items still need sign-off before end of week.\n\nCan we hop on a call Thursday afternoon?\n\n— Sarah",
	},
	{
		from: "GitHub", subject: "[matcha] PR #1448 merged",
		date: "9:15 AM", isRead: false,
		bodyFrom: "GitHub <noreply@github.com>",
		body:     "PR #1448 \"feat: startup guide\" was merged into master\nby @drew.\n\nView on GitHub →\ngithub.com/floatpane/matcha/pull/1448",
	},
	{
		from: "Mom", subject: "Dinner on Sunday?",
		date: "Yesterday", isRead: true,
		bodyFrom: "Mom <mom@home.net>",
		body:     "Hi honey,\n\nAre you free for dinner this Sunday?\nI'm making that pasta you like.\n\nLove, Mom ❤️",
	},
	{
		from: "Stripe", subject: "Your receipt from Fly.io",
		date: "Mon", isRead: true,
		bodyFrom: "Stripe <receipts@stripe.com>",
		body:     "Receipt  #INV-2026-0089\nAmount:   $19.00\nDate:     June 3, 2026\nCard:     •••• 4242\n\nThank you for your business.",
	},
	{
		from: "Linear", subject: "[Bug] Cursor stuck in setup guide",
		date: "Mon", isRead: false,
		bodyFrom: "Linear <notify@linear.app>",
		body:     "IMP-421 has been assigned to you.\n\nTitle:    Cursor stuck in typewriter animation\nPriority: High\nStatus:   In Progress\n\nView on Linear →",
	},
}

type setupGuideHelperDoneMsg struct{ err error }
type setupGuideMailtoDoneMsg struct{ err error }

// NewSetupGuide creates the first-run wizard. Pass nil callbacks for
// platforms where the corresponding feature is unavailable.
func NewSetupGuide(isMac, isLinux bool, installHelper, setupMailto func() error) *SetupGuide {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

	return &SetupGuide{
		step:            setupStepWelcome,
		isMac:           isMac,
		isLinux:         isLinux,
		spin:            sp,
		doInstallHelper: installHelper,
		doSetupMailto:   setupMailto,
	}
}

func (m *SetupGuide) Init() tea.Cmd {
	return m.spin.Tick
}

func (m *SetupGuide) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case setupGuideHelperDoneMsg:
		if msg.err != nil {
			m.helperStatus = setupStatusError
			m.helperErr = msg.err.Error()
		} else {
			m.helperStatus = setupStatusDone
		}
		return m, nil

	case setupGuideMailtoDoneMsg:
		if msg.err != nil {
			m.mailtoStatus = setupStatusError
			m.mailtoErr = msg.err.Error()
		} else {
			m.mailtoStatus = setupStatusDone
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m *SetupGuide) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case setupStepWelcome, setupStepFeatures:
		if msg.String() == keyEnter || msg.String() == " " {
			m.advanceStep()
			return m, nil
		}

	case setupStepHelper:
		return m.handleHelperKey(msg)

	case setupStepMailto:
		return m.handleMailtoKey(msg)

	case setupStepShowcase:
		m.handleShowcaseKey(msg)

	case setupStepMouse:
		return m.handleMouseKey(msg)

	case setupStepDone:
		if msg.String() == keyEnter || msg.String() == " " {
			return m, func() tea.Msg { return SetupGuideDoneMsg{} }
		}
	}

	return m, nil
}

func (m *SetupGuide) handleHelperKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.helperStatus == setupStatusRunning {
		return m, nil
	}
	if m.helperStatus != setupStatusPending {
		switch msg.String() {
		case keyEnter, " ":
			m.advanceStep()
		case "q", keyCtrlC:
			return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
		}
		return m, nil
	}
	switch msg.String() {
	case "up", "k":
		m.cursor = 0
	case keyDown, "j":
		m.cursor = 1
	case keyEnter:
		if m.cursor == 0 && m.doInstallHelper != nil {
			m.helperStatus = setupStatusRunning
			fn := m.doInstallHelper
			return m, func() tea.Msg {
				err := fn()
				return setupGuideHelperDoneMsg{err: err}
			}
		}
		if m.cursor == 1 {
			m.helperStatus = setupStatusSkipped
			m.advanceStep()
		}
	case "q", keyCtrlC:
		return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
	}
	return m, nil
}

func (m *SetupGuide) handleMailtoKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mailtoStatus == setupStatusRunning {
		return m, nil
	}
	if m.mailtoStatus != setupStatusPending {
		switch msg.String() {
		case keyEnter, " ":
			m.advanceStep()
		case "q", keyCtrlC:
			return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
		}
		return m, nil
	}
	switch msg.String() {
	case "up", "k":
		m.cursor = 0
	case keyDown, "j":
		m.cursor = 1
	case keyEnter:
		if m.cursor == 0 && m.doSetupMailto != nil {
			m.mailtoStatus = setupStatusRunning
			fn := m.doSetupMailto
			return m, func() tea.Msg {
				err := fn()
				return setupGuideMailtoDoneMsg{err: err}
			}
		}
		if m.cursor == 1 {
			m.mailtoStatus = setupStatusSkipped
			m.advanceStep()
		}
	case "q", keyCtrlC:
		return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
	}
	return m, nil
}

func (m *SetupGuide) handleShowcaseKey(msg tea.KeyPressMsg) {
	switch m.showcaseTourStep {
	case 0: // overview — any key advances
		if msg.String() == keyEnter || msg.String() == " " {
			m.showcaseTourStep = 1
		}
	case 1: // navigation — j/k live, space/enter to advance
		switch msg.String() {
		case "up", "k":
			if m.showcaseCursor > 0 {
				m.showcaseCursor--
			}
		case keyDown, "j":
			if m.showcaseCursor < len(demoEmails)-1 {
				m.showcaseCursor++
			}
		case keyEnter, " ":
			m.showcaseTourStep = 2
		}
	case 2: // opening — enter opens email, space skips to commands
		switch msg.String() {
		case "up", "k":
			if m.showcaseCursor > 0 {
				m.showcaseCursor--
			}
		case keyDown, "j":
			if m.showcaseCursor < len(demoEmails)-1 {
				m.showcaseCursor++
			}
		case keyEnter:
			m.showcaseMode = 1
			m.showcaseTourStep = 3
		case " ":
			m.showcaseTourStep = 4
		}
	case 3: // email view — any key returns to inbox + advances
		if msg.String() == keyEnter || msg.String() == " " || msg.String() == "esc" {
			m.showcaseMode = 0
			m.showcaseTourStep = 4
		}
	case 4: // commands — any key finishes
		if msg.String() == keyEnter || msg.String() == " " {
			m.advanceStep()
		}
	}
}

func (m *SetupGuide) handleMouseKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", keyEnter:
		m.advanceStep()
		return m, func() tea.Msg { return MouseSupportChosenMsg{Enabled: true} }
	case "n", "N":
		m.advanceStep()
		return m, func() tea.Msg { return MouseSupportChosenMsg{Enabled: false} }
	case "q", keyCtrlC:
		return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
	}
	return m, nil
}

func (m *SetupGuide) advanceStep() {
	m.cursor = 0
	next := m.step + 1

	// Skip helper step on non-macOS
	if next == setupStepHelper && !m.isMac {
		next++
	}
	// Skip mailto step on unsupported OS
	if next == setupStepMailto && !m.isMac && !m.isLinux {
		next++
	}

	m.step = next
}

// ── View ──────────────────────────────────────────────────────────────────────

var (
	sgAccent = lipgloss.Color("42")
	sgDim    = lipgloss.Color("240")
	sgWarn   = lipgloss.Color("214")
	sgOk     = lipgloss.Color("42")
	sgErr    = lipgloss.Color("196")

	sgTitleStyle = lipgloss.NewStyle().
			Foreground(sgAccent).
			Bold(true)

	sgSubtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	sgBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(sgAccent).
			Padding(1, 3)

	sgDimStyle  = lipgloss.NewStyle().Foreground(sgDim)
	sgOkStyle   = lipgloss.NewStyle().Foreground(sgOk).Bold(true)
	sgErrStyle  = lipgloss.NewStyle().Foreground(sgErr)
	sgWarnStyle = lipgloss.NewStyle().Foreground(sgWarn)
)

func (m *SetupGuide) View() tea.View {
	// Showcase step gets the full screen — no surrounding box.
	if m.step == setupStepShowcase {
		return tea.NewView(m.viewShowcaseFull())
	}

	var content string
	switch m.step {
	case setupStepWelcome:
		content = m.viewWelcome()
	case setupStepFeatures:
		content = m.viewFeatures()
	case setupStepHelper:
		content = m.viewHelper()
	case setupStepMailto:
		content = m.viewMailto()
	case setupStepMouse:
		content = m.viewMouse()
	case setupStepDone:
		content = m.viewDone()
	}

	boxW := 64
	if m.width > 0 && m.width-6 < boxW {
		boxW = m.width - 6
	}
	box := sgBoxStyle.Width(boxW).Render(content)
	progress := m.viewProgress()
	combined := box + "\n\n" + progress

	if m.width > 0 && m.height > 0 {
		combined = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, combined)
	}

	return tea.NewView(combined)
}

func (m *SetupGuide) viewProgress() string {
	steps := []int{setupStepWelcome, setupStepFeatures, setupStepShowcase, setupStepMouse, setupStepDone}
	if m.isMac {
		steps = []int{setupStepWelcome, setupStepFeatures, setupStepHelper, setupStepMailto, setupStepShowcase, setupStepMouse, setupStepDone}
	} else if m.isLinux {
		steps = []int{setupStepWelcome, setupStepFeatures, setupStepMailto, setupStepShowcase, setupStepMouse, setupStepDone}
	}

	var dots []string
	for _, s := range steps {
		switch {
		case s == m.step:
			dots = append(dots, sgOkStyle.Render("●"))
		case s < m.step:
			dots = append(dots, sgDimStyle.Render("◉"))
		default:
			dots = append(dots, sgDimStyle.Render("○"))
		}
	}
	return sgDimStyle.Render("  ") + strings.Join(dots, sgDimStyle.Render(" · "))
}

func (m *SetupGuide) viewWelcome() string {
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	logo := accentStyle.Render(choiceLogo)
	body := sgSubtitleStyle.Render(
		"Welcome to Matcha.\n\n" +
			"A fast, keyboard-driven email client that\n" +
			"lives right in your terminal.\n\n" +
			"Let's get you set up in a few steps.",
	)
	hint := "\n\n" + sgDimStyle.Render("  press enter to continue →")
	return logo + "\n" + body + hint
}

func (m *SetupGuide) viewFeatures() string {
	icon := func(i, s string) string {
		return sgDimStyle.Render(i) + "  " + sgSubtitleStyle.Render(s)
	}

	var b strings.Builder
	b.WriteString(sgTitleStyle.Render("  What Matcha can do") + "\n\n")

	features := []struct{ icon, text string }{
		{"📬", "Multiple accounts — IMAP, JMAP, POP3, and local Maildir"},
		{"🧵", "Threaded conversations and split-pane reading"},
		{"🔌", "Lua plugin system — automate, extend, and customise"},
		{"🎨", "Themes, including automatic sync with macOS appearance"},
		{"🔐", "PGP encryption and secure mode with master password"},
		{"✏️", "Spell-check with suggestions as you compose"},
		{"🔍", "Full-text search across all your mail"},
		{"📇", "Contacts sync from macOS Contacts"},
	}

	for _, f := range features {
		b.WriteString("  " + icon(f.icon, f.text) + "\n")
	}

	b.WriteString("\n" + sgDimStyle.Render("  press enter to continue →"))
	return b.String()
}

func (m *SetupGuide) viewHelper() string {
	var b strings.Builder

	b.WriteString(sgTitleStyle.Render("  Menu Bar Helper") + "\n\n")
	b.WriteString(sgSubtitleStyle.Render(
		"The macOS helper adds a 🍵 icon to your menu bar\n"+
			"that shows your unread count and posts native\n"+
			"notifications — without opening a terminal.\n",
	) + "\n")

	switch m.helperStatus {
	case setupStatusPending:
		b.WriteString(m.renderChoice("Install helper", "Skip for now"))
	case setupStatusRunning:
		b.WriteString(m.spin.View() + " Installing… (this compiles a small Swift app)\n")
	case setupStatusDone:
		b.WriteString(sgOkStyle.Render("✓ Helper installed!") + "\n\n")
		b.WriteString(sgDimStyle.Render("  press enter to continue →"))
	case setupStatusError:
		b.WriteString(sgErrStyle.Render("✗ Install failed: "+m.helperErr) + "\n\n")
		b.WriteString(sgDimStyle.Render("  press enter to continue →"))
	case setupStatusSkipped:
		b.WriteString(sgDimStyle.Render("  Skipped. You can run 'matcha helper install' later.") + "\n\n")
		b.WriteString(sgDimStyle.Render("  press enter to continue →"))
	}

	return b.String()
}

func (m *SetupGuide) viewMailto() string {
	var b strings.Builder

	b.WriteString(sgTitleStyle.Render("  mailto: Redirect") + "\n\n")
	b.WriteString(sgSubtitleStyle.Render(
		"Register Matcha as your default mailto: handler\n"+
			"so clicking email links opens a compose window\n"+
			"in your terminal automatically.\n",
	) + "\n")

	switch m.mailtoStatus {
	case setupStatusPending:
		b.WriteString(m.renderChoice("Set up mailto redirect", "Skip for now"))
	case setupStatusRunning:
		b.WriteString(m.spin.View() + " Setting up…\n")
	case setupStatusDone:
		b.WriteString(sgOkStyle.Render("✓ mailto redirect configured!") + "\n")
		if m.isMac {
			b.WriteString(sgWarnStyle.Render("  → Open Mail.app → Settings → General\n    and set Default email reader to MatchaMail.\n") + "\n")
		}
		b.WriteString("\n" + sgDimStyle.Render("  press enter to continue →"))
	case setupStatusError:
		b.WriteString(sgErrStyle.Render("✗ Setup failed: "+m.mailtoErr) + "\n\n")
		b.WriteString(sgDimStyle.Render("  press enter to continue →"))
	case setupStatusSkipped:
		b.WriteString(sgDimStyle.Render("  Skipped. You can run 'matcha setup-mailto' later.") + "\n\n")
		b.WriteString(sgDimStyle.Render("  press enter to continue →"))
	}

	return b.String()
}

// ── Full-screen showcase / interactive tour ───────────────────────────────────

var (
	tourSidebarStyle = lipgloss.NewStyle().
				Width(sidebarWidth).
				BorderStyle(lipgloss.NormalBorder()).
				BorderRight(true).
				PaddingRight(1).
				PaddingLeft(1)

	tourSidebarTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true).
				PaddingBottom(1)

	tourFolderActive = lipgloss.NewStyle().
				PaddingLeft(1).PaddingRight(1).
				Background(lipgloss.Color("42")).
				Foreground(lipgloss.Color("#000000")).
				Bold(true)

	tourFolderIdle = lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)

	tourTooltipStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#25A065")).
				Padding(1, 3).
				Width(46)

	tourTabBarStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			PaddingBottom(1).
			MarginBottom(1)

	tourActiveTab = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("42")).
			Bold(true).
			Underline(true)

	tourDimRow   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	tourUnread   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	tourRead     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	tourSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
)

func (m *SetupGuide) viewShowcaseFull() string {
	w, h := m.width, m.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}

	sidebar := m.viewTourSidebar(h)
	sbW := lipgloss.Width(sidebar)
	paneW := w - sbW
	if paneW < 20 {
		paneW = 20
	}

	var mainPane string
	if m.showcaseMode == 1 {
		mainPane = m.viewTourEmailPane(paneW, h)
	} else {
		mainPane = m.viewTourInboxPane(paneW, h)
	}

	background := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainPane)
	tooltip := m.viewTourTooltip()
	return overlay.Center(background, tooltip, w, h)
}

func (m *SetupGuide) viewTourSidebar(h int) string {
	var b strings.Builder
	b.WriteString(tourSidebarTitleStyle.Render("Alex"))
	b.WriteString("\n")

	folders := []struct {
		name   string
		unread int
		active bool
	}{
		{"Inbox", 3, true},
		{"Sent", 0, false},
		{"Drafts", 1, false},
		{"Trash", 0, false},
		{"Archive", 0, false},
	}

	for i, f := range folders {
		label := f.name
		if f.unread > 0 {
			label = fmt.Sprintf("%s (%d)", f.name, f.unread)
		}
		if f.active {
			b.WriteString(tourFolderActive.Width(sidebarWidth - 4).Render(label))
		} else {
			b.WriteString(tourFolderIdle.Render(label))
		}
		if i < len(folders)-1 {
			b.WriteString("\n")
		}
	}

	return tourSidebarStyle.Height(h).Render(b.String())
}

func (m *SetupGuide) viewTourInboxPane(w, h int) string {
	spotlight := m.showcaseTourStep == 1 || m.showcaseTourStep == 2
	innerW := w - 2

	var rows strings.Builder
	for i, e := range demoEmails {
		isSelected := i == m.showcaseCursor

		var st lipgloss.Style
		switch {
		case spotlight && !isSelected:
			st = tourDimRow
		case !e.isRead:
			st = tourUnread
		default:
			st = tourRead
		}

		icon := "\uf2b6"
		if !e.isRead {
			icon = "\uf0e0"
		}
		prefix := fmt.Sprintf("%d. ", i+1)
		sender := e.from
		if len([]rune(sender)) > 14 {
			sender = string([]rune(sender)[:13]) + "\u2026"
		}
		subject := e.subject
		dateStr := e.date

		styledDate := st.Render(dateStr)
		dateW := lipgloss.Width(styledDate)
		cursorW := 0
		if isSelected {
			cursorW = 2
		}
		leftBudget := innerW - dateW - 2 - cursorW
		used := len(prefix) + 2 + lipgloss.Width(sender) + 3
		subjectBudget := leftBudget - used
		if subjectBudget < 4 {
			subjectBudget = 4
		}
		if lipgloss.Width(subject) > subjectBudget {
			runes := []rune(subject)
			for lipgloss.Width(string(runes)) > subjectBudget-1 && len(runes) > 0 {
				runes = runes[:len(runes)-1]
			}
			subject = string(runes) + "\u2026"
		}

		row := prefix + st.Render(icon+" "+sender) + sgDimStyle.Render(" \u00b7 ") + st.Render(subject)
		pad := innerW - lipgloss.Width(row) - dateW - cursorW
		if pad < 1 {
			pad = 1
		}
		row += strings.Repeat(" ", pad) + styledDate

		if isSelected {
			rows.WriteString(" " + tourSelected.Render("> "+row))
		} else {
			rows.WriteString("   " + row)
		}
		rows.WriteString("\n")
	}

	tab := tourTabBarStyle.Width(w - 1).Render(tourActiveTab.Render("Inbox"))
	body := tab + "\n" + rows.String()
	used := lipgloss.Height(body)
	for i := used; i < h; i++ {
		body += "\n"
	}
	return body
}

func (m *SetupGuide) viewTourEmailPane(w, h int) string {
	e := demoEmails[m.showcaseCursor]
	dim := sgDimStyle
	sep := dim.Render(strings.Repeat("\u2500", w-2))
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Width(10)

	var b strings.Builder
	b.WriteString(dim.Render(" \u2190 esc  back to inbox") + "\n")
	b.WriteString(sep + "\n")
	b.WriteString(" " + label.Render("From:") + sgSubtitleStyle.Render(e.bodyFrom) + "\n")
	b.WriteString(" " + label.Render("Subject:") + sgTitleStyle.Render(e.subject) + "\n")
	b.WriteString(" " + label.Render("Date:") + sgSubtitleStyle.Render(e.date) + "\n")
	b.WriteString(sep + "\n\n")
	for _, line := range strings.Split(e.body, "\n") {
		b.WriteString(" " + sgSubtitleStyle.Render(line) + "\n")
	}
	body := b.String()
	used := lipgloss.Height(body)
	for i := used; i < h; i++ {
		body += "\n"
	}
	return body
}

func (m *SetupGuide) viewTourTooltip() string {
	key := func(s string) string {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1).Render(s)
	}
	dim := sgDimStyle
	title := sgTitleStyle
	body := sgSubtitleStyle
	step := m.showcaseTourStep

	var b strings.Builder
	switch step {
	case 0:
		b.WriteString(title.Render("Welcome to Matcha!") + "\n\n")
		b.WriteString(body.Render(
			"This is your inbox — all your emails\n"+
				"appear here, sorted by date.\n\n"+
				"Unread emails are highlighted in green.\n"+
				"Read emails are shown in grey.",
		) + "\n")
	case 1:
		b.WriteString(title.Render("Navigating your inbox") + "\n\n")
		b.WriteString(body.Render("Move between emails with:") + "\n\n")
		b.WriteString("  " + key("j") + "  or  " + key("\u2193") + "   " + dim.Render("move down") + "\n")
		b.WriteString("  " + key("k") + "  or  " + key("\u2191") + "   " + dim.Render("move up") + "\n\n")
		b.WriteString(body.Render("Try it \u2014 the selected row\nis spotlit above.") + "\n")
	case 2:
		b.WriteString(title.Render("Opening an email") + "\n\n")
		b.WriteString(body.Render(
			"Press enter to open the\nselected email and read it.",
		) + "\n\n")
		b.WriteString("  " + key("enter") + "   " + dim.Render("open email") + "\n")
	case 3:
		b.WriteString(title.Render("Reading & replying") + "\n\n")
		b.WriteString(body.Render("From the email view:") + "\n\n")
		b.WriteString("  " + key("r") + "   " + dim.Render("reply") + "\n")
		b.WriteString("  " + key("a") + "   " + dim.Render("archive") + "\n")
		b.WriteString("  " + key("f") + "   " + dim.Render("forward") + "\n")
		b.WriteString("  " + key("d") + "   " + dim.Render("delete / trash") + "\n")
	case 4:
		b.WriteString(title.Render("Power features") + "\n\n")
		b.WriteString("  " + key("/") + "        " + dim.Render("search inbox") + "\n")
		b.WriteString("  " + key("ctrl+k") + "   " + dim.Render("command palette") + "\n")
		b.WriteString("  " + key("?") + "        " + dim.Render("full key reference") + "\n\n")
		b.WriteString(body.Render("You're ready. Your real\ninbox is one step away.") + "\n")
	}

	b.WriteString("\n" + dim.Render(strings.Repeat("\u2500", 40)) + "\n")
	b.WriteString(dim.Render(fmt.Sprintf("step %d of 5", step+1)))

	switch step {
	case 1:
		b.WriteString(dim.Render("   \u00b7   ") + key("space") + dim.Render(" next"))
	case 2:
		b.WriteString(dim.Render("   \u00b7   ") + key("enter") + dim.Render(" open  ") + key("space") + dim.Render(" skip"))
	case 4:
		b.WriteString(dim.Render("   \u00b7   ") + key("enter") + dim.Render(" done"))
	default:
		b.WriteString(dim.Render("   \u00b7   ") + key("enter") + dim.Render(" next"))
	}

	return tourTooltipStyle.Render(b.String())
}

func (m *SetupGuide) viewMouse() string {
	var b strings.Builder
	b.WriteString(sgTitleStyle.Render("  🖱  Mouse Support") + "\n\n")
	b.WriteString(sgSubtitleStyle.Render(
		"  Would you like to enable mouse support?\n"+
			"  Click to select, scroll to navigate, double-click\n"+
			"  to open emails.\n",
	) + "\n\n")
	b.WriteString(sgDimStyle.Render("  You can change this later in Settings → General.\n\n"))
	b.WriteString(sgOkStyle.Render("  y") + sgDimStyle.Render(" — enable mouse support\n"))
	b.WriteString(sgDimStyle.Render("  n — keyboard only  ·  enter — enable"))
	return b.String()
}

func (m *SetupGuide) viewDone() string {
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	var b strings.Builder

	teacup := accentStyle.Render("  🍵")

	b.WriteString(teacup + "\n\n")
	b.WriteString(sgTitleStyle.Render("  You're all set!") + "\n\n")
	b.WriteString(sgSubtitleStyle.Render(
		"  Matcha is ready.\n"+
			"  Your inbox awaits — enjoy the quiet.\n",
	) + "\n\n")

	_ = theme.ActiveTheme // ensure theme imported
	b.WriteString(sgDimStyle.Render("  press enter to open Matcha →"))
	return b.String()
}

func (m *SetupGuide) renderChoice(yes, skip string) string {
	var b strings.Builder
	options := []string{yes, skip}
	for i, opt := range options {
		if m.cursor == i {
			b.WriteString(sgOkStyle.Render("  › " + opt))
		} else {
			b.WriteString(sgDimStyle.Render("    " + opt))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + sgDimStyle.Render("  ↑/↓ to move · enter to select"))
	return b.String()
}
