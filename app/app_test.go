package app

import (
	"fmt"
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/internal/palette"
	"github.com/floatpane/matcha/tui"
)

// TestMain keeps the tests away from the developer's real matcha config and
// caches.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "matcha-app-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test home: %v\n", err)
		os.Exit(1)
	}
	_ = os.Setenv("HOME", home)
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

// recordingView is a minimal view that records the messages it receives.
type recordingView struct {
	msgs []tea.Msg
}

func (r *recordingView) Init() tea.Cmd { return nil }
func (r *recordingView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	r.msgs = append(r.msgs, msg)
	return r, nil
}
func (r *recordingView) View() tea.View { return tea.NewView("recording") }

func (r *recordingView) received(match func(tea.Msg) bool) bool {
	for _, msg := range r.msgs {
		if match(msg) {
			return true
		}
	}
	return false
}

func newTestModel(t *testing.T) *Model {
	t.Helper()
	m := NewModel(nil, nil, nil, false, nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func setMouse(t *testing.T, enabled *bool) {
	t.Helper()
	previous := config.MouseEnabled
	config.MouseEnabled = enabled
	t.Cleanup(func() { config.MouseEnabled = previous })
}

func TestViewAppliesMouseModeToEveryScreen(t *testing.T) {
	m := newTestModel(t)
	if _, ok := m.current.(*tui.Login); !ok {
		t.Fatalf("expected the login screen without a config, got %T", m.current)
	}

	on := true
	setMouse(t, &on)
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("mouse enabled: MouseMode = %v on the login screen, want cell motion", got)
	}
	off := false
	setMouse(t, &off)
	if got := m.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("mouse disabled: MouseMode = %v, want none", got)
	}
}

func TestPaletteSwallowsClicks(t *testing.T) {
	m := newTestModel(t)
	rec := &recordingView{}
	m.current = rec
	m.palette.Open(palette.BuildCommands(m.current, nil, nil), m.width, m.contentHeight())
	if !m.palette.IsOpen() {
		t.Fatal("palette did not open")
	}
	m.Update(tea.MouseClickMsg{X: 50, Y: 15, Button: tea.MouseLeft})
	if rec.received(func(msg tea.Msg) bool { _, ok := msg.(tea.MouseClickMsg); return ok }) {
		t.Fatal("a click on the open palette reached the view underneath")
	}
}

func TestStaleNotificationTimerKeepsNewerToast(t *testing.T) {
	m := newTestModel(t)
	m.showErrorCmd("first")
	firstGen := m.errorNotifGen
	m.showInfoCmd("second")
	m.Update(clearErrorNotifMsg{gen: firstGen})
	if !m.showErrorNotif {
		t.Fatal("the first toast's timer hid the second toast")
	}
	m.Update(clearErrorNotifMsg{gen: m.errorNotifGen})
	if m.showErrorNotif {
		t.Fatal("the toast's own timer did not hide it")
	}
}

func TestNotificationsFitNarrowScreens(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	m.showErrorCmd("Something went wrong while doing a thing")
	for _, r := range m.notificationRects() {
		if r[1] < 0 || r[1]+r[3] > 30+2 {
			t.Fatalf("toast rectangle %v does not fit a 30-column screen", r)
		}
	}
}

func TestReturningViewReceivesCurrentSize(t *testing.T) {
	m := newTestModel(t)
	back := &recordingView{}
	m.current = &recordingView{}
	m.previousModel = back
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.Update(tui.RestoreViewMsg{})
	if m.current != tea.Model(back) {
		t.Fatalf("RestoreViewMsg did not restore the previous view, current is %T", m.current)
	}
	if !back.received(func(msg tea.Msg) bool {
		size, ok := msg.(tea.WindowSizeMsg)
		return ok && size.Width == 120 && size.Height == 40
	}) {
		t.Fatal("restored view was not told the current window size")
	}
}

func TestViewChangedHandlesValueViews(t *testing.T) {
	a, b := &recordingView{}, &recordingView{}
	if !viewChanged(a, b) || viewChanged(a, a) {
		t.Fatal("pointer views must be compared by identity")
	}
	// Comparing non-comparable value views must not panic.
	if viewChanged(tui.NewStatus("one"), tui.NewStatus("two")) {
		t.Fatal("value views of the same type are treated as unchanged")
	}
	if !viewChanged(tui.NewStatus("one"), a) {
		t.Fatal("different view types must count as a change")
	}
}
