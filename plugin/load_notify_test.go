package plugin

import (
	"strings"
	"testing"
)

func TestLoadPluginErrorQueuesErrorNotification(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()

	bad := writePlugin(t, t.TempDir(), "broken.lua", `this is not valid lua !!!`)
	m.loadPlugin("broken", bad)

	n, ok := m.TakePendingNotification()
	if !ok {
		t.Fatal("expected a pending notification for load error")
	}
	if n.Kind != NotifyKindError {
		t.Fatalf("expected kind %q, got %q", NotifyKindError, n.Kind)
	}
	if n.Title != "Plugin load error" {
		t.Fatalf("expected title %q, got %q", "Plugin load error", n.Title)
	}
	if !strings.Contains(n.Message, "broken") {
		t.Fatalf("expected message to contain plugin name, got %q", n.Message)
	}
	if !n.Closable {
		t.Fatal("expected closable true so user can dismiss")
	}
	if n.Duration <= 0 {
		t.Fatalf("expected positive duration, got %f", n.Duration)
	}
}

func TestLoadPluginMultipleErrorsAreQueuedAndDrained(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()

	dir := t.TempDir()
	m.loadPlugin("first", writePlugin(t, dir, "first.lua", `!!! bad`))
	m.loadPlugin("second", writePlugin(t, dir, "second.lua", `!!! also bad`))

	// Plugins should NOT be registered when they fail to load.
	if len(m.Plugins()) != 0 {
		t.Fatalf("expected no plugins registered, got %d", len(m.Plugins()))
	}

	// Drain the first notification.
	n1, ok := m.TakePendingNotification()
	if !ok {
		t.Fatal("expected first queued notification")
	}
	if !strings.Contains(n1.Message, "first") {
		t.Fatalf("expected first message to mention first plugin, got %q", n1.Message)
	}

	// Drain the second notification.
	n2, ok := m.TakePendingNotification()
	if !ok {
		t.Fatal("expected second queued notification")
	}
	if !strings.Contains(n2.Message, "second") {
		t.Fatalf("expected second message to mention second plugin, got %q", n2.Message)
	}

	// Queue should now be empty.
	if _, ok := m.TakePendingNotification(); ok {
		t.Fatal("expected no more notifications after draining queue")
	}
}

func TestLoadNotificationRespectsExistingPendingNotification(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()

	// Simulate a plugin-set notification occupying the single slot.
	m.pendingNotification = &PendingNotification{Message: "plugin said hi", Kind: NotifyKindInfo}

	m.loadPlugin("broken", writePlugin(t, t.TempDir(), "broken.lua", `!!! bad`))

	// The pre-existing notification should be returned first, not the load error.
	n, ok := m.TakePendingNotification()
	if !ok {
		t.Fatal("expected pending notification")
	}
	if n.Message != "plugin said hi" {
		t.Fatalf("expected pre-existing notification first, got %q", n.Message)
	}

	// Now the load error should be drained from the queue.
	n2, ok := m.TakePendingNotification()
	if !ok {
		t.Fatal("expected load error notification after pre-existing one")
	}
	if !strings.Contains(n2.Message, "broken") {
		t.Fatalf("expected load error message, got %q", n2.Message)
	}
	if n2.Kind != NotifyKindError {
		t.Fatalf("expected error kind, got %q", n2.Kind)
	}
}

func TestSuccessfulLoadDoesNotQueueNotification(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()

	good := writePlugin(t, t.TempDir(), "good.lua", `
		local matcha = require("matcha")
	`)
	m.loadPlugin("good", good)

	if _, ok := m.TakePendingNotification(); ok {
		t.Fatal("expected no notification for successful load")
	}
}
