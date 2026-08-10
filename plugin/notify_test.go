package plugin

import (
	"testing"
)

func TestLuaNotify_BackwardCompat_Number(t *testing.T) {
	m := NewManager()
	defer m.Close()

	L := m.LuaState()
	if err := L.DoString(`require("matcha").notify("hello", 5)`); err != nil {
		t.Fatalf("lua error: %v", err)
	}

	n, ok := m.TakePendingNotification()
	if !ok {
		t.Fatal("expected pending notification")
	}
	if n.Message != "hello" {
		t.Fatalf("expected message %q, got %q", "hello", n.Message)
	}
	if n.Duration != 5 {
		t.Fatalf("expected duration 5, got %f", n.Duration)
	}
	if n.Kind != NotifyKindInfo {
		t.Fatalf("expected kind %q, got %q", NotifyKindInfo, n.Kind)
	}
	if !n.Closable {
		t.Fatal("expected closable true by default")
	}
	if n.Title != "" {
		t.Fatalf("expected empty title, got %q", n.Title)
	}
}

func TestLuaNotify_BackwardCompat_NoDuration(t *testing.T) {
	m := NewManager()
	defer m.Close()

	L := m.LuaState()
	if err := L.DoString(`require("matcha").notify("just a message")`); err != nil {
		t.Fatalf("lua error: %v", err)
	}

	n, ok := m.TakePendingNotification()
	if !ok {
		t.Fatal("expected pending notification")
	}
	if n.Duration != 2 {
		t.Fatalf("expected default duration 2, got %f", n.Duration)
	}
}

func TestLuaNotify_OptionsTable(t *testing.T) {
	m := NewManager()
	defer m.Close()

	L := m.LuaState()
	script := `require("matcha").notify("disk full", {
	    duration = 10,
	    kind = "error",
	    title = "Storage Alert",
	    closable = false,
	})`
	if err := L.DoString(script); err != nil {
		t.Fatalf("lua error: %v", err)
	}

	n, ok := m.TakePendingNotification()
	if !ok {
		t.Fatal("expected pending notification")
	}
	if n.Message != "disk full" {
		t.Fatalf("expected message %q, got %q", "disk full", n.Message)
	}
	if n.Duration != 10 {
		t.Fatalf("expected duration 10, got %f", n.Duration)
	}
	if n.Kind != NotifyKindError {
		t.Fatalf("expected kind %q, got %q", NotifyKindError, n.Kind)
	}
	if n.Title != "Storage Alert" {
		t.Fatalf("expected title %q, got %q", "Storage Alert", n.Title)
	}
	if n.Closable {
		t.Fatal("expected closable false")
	}
}

func TestLuaNotify_OptionsTable_Partial(t *testing.T) {
	m := NewManager()
	defer m.Close()

	L := m.LuaState()
	script := `require("matcha").notify("warning!", {
	    kind = "warning",
	})`
	if err := L.DoString(script); err != nil {
		t.Fatalf("lua error: %v", err)
	}

	n, ok := m.TakePendingNotification()
	if !ok {
		t.Fatal("expected pending notification")
	}
	if n.Kind != NotifyKindWarning {
		t.Fatalf("expected kind %q, got %q", NotifyKindWarning, n.Kind)
	}
	// Defaults should hold for unspecified fields
	if n.Duration != 2 {
		t.Fatalf("expected default duration 2, got %f", n.Duration)
	}
	if !n.Closable {
		t.Fatal("expected default closable true")
	}
}

func TestLuaNotify_InvalidKind(t *testing.T) {
	m := NewManager()
	defer m.Close()

	L := m.LuaState()
	err := L.DoString(`require("matcha").notify("msg", {kind = "invalid"})`)
	if err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

func TestTakePendingNotification_NilWhenEmpty(t *testing.T) {
	m := NewManager()
	defer m.Close()

	if _, ok := m.TakePendingNotification(); ok {
		t.Fatal("expected no pending notification on fresh manager")
	}
}

func TestTakePendingNotification_ConsumedOnce(t *testing.T) {
	m := NewManager()
	defer m.Close()

	m.pendingNotification = &PendingNotification{Message: "test"}

	if _, ok := m.TakePendingNotification(); !ok {
		t.Fatal("expected notification on first take")
	}
	if _, ok := m.TakePendingNotification(); ok {
		t.Fatal("expected no notification on second take")
	}
}
