package app

import (
	"bytes"
	"log"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &buf
}

// runRecovering runs f and returns what it panicked with.
func runRecovering(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
}

func TestGuardedCommandPanicsAreLoggedAndReraised(t *testing.T) {
	buf := captureLog(t)
	panicky := func() tea.Msg { panic("boom") }
	batch := guardCmd(tea.Batch(func() tea.Msg { return nil }, panicky))
	msg := batch()
	inner, ok := msg.(tea.BatchMsg)
	if !ok || len(inner) != 2 {
		t.Fatalf("a guarded batch returned %#v", msg)
	}
	if r := runRecovering(func() { inner[1]() }); r != "boom" {
		t.Fatalf("the panic was not re-raised for Bubble Tea: %v", r)
	}
	if out := buf.String(); !strings.Contains(out, "panic in command: boom") || !strings.Contains(out, "panics_test.go") {
		t.Fatalf("the panic and its stack were not logged:\n%s", out)
	}
}

func TestUpdatePanicsAreLogged(t *testing.T) {
	buf := captureLog(t)
	m := newTestModel(t)
	m.current = panickingView{}
	if r := runRecovering(func() { m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}) }); r == nil {
		t.Fatal("the view's panic was swallowed")
	}
	if !strings.Contains(buf.String(), "panic in update") {
		t.Fatalf("the update panic was not logged:\n%s", buf.String())
	}
}

type panickingView struct{}

func (panickingView) Init() tea.Cmd                       { return nil }
func (panickingView) Update(tea.Msg) (tea.Model, tea.Cmd) { panic("view update failed") }
func (panickingView) View() tea.View                      { return tea.NewView("") }
