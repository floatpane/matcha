package plugin

import (
	"testing"
)

func TestTextOverride(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// No override initially
	if _, ok := m.TextOverride("folder_inbox.folders_title"); ok {
		t.Fatal("expected no override before SetTextOverride")
	}

	// Set override
	m.SetTextOverride("folder_inbox.folders_title", "My Folders")
	val, ok := m.TextOverride("folder_inbox.folders_title")
	if !ok {
		t.Fatal("expected override to be present after SetTextOverride")
	}
	if val != "My Folders" {
		t.Fatalf("expected %q, got %q", "My Folders", val)
	}

	// Clear override
	m.ClearTextOverride("folder_inbox.folders_title")
	if _, ok := m.TextOverride("folder_inbox.folders_title"); ok {
		t.Fatal("expected override to be cleared")
	}
}

func TestComponentVisibility(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// Default: all components visible
	if !m.IsComponentVisible("sidebar") {
		t.Fatal("expected sidebar visible by default")
	}
	if !m.IsComponentVisible("status_bar") {
		t.Fatal("expected status_bar visible by default")
	}
	if !m.IsComponentVisible("header") {
		t.Fatal("expected header visible by default")
	}

	// Hide sidebar
	if !m.SetComponentVisible("sidebar", false) {
		t.Fatal("SetComponentVisible should return true for valid name")
	}
	if m.IsComponentVisible("sidebar") {
		t.Fatal("expected sidebar hidden after SetComponentVisible(false)")
	}

	// Show sidebar again
	m.SetComponentVisible("sidebar", true)
	if !m.IsComponentVisible("sidebar") {
		t.Fatal("expected sidebar visible after SetComponentVisible(true)")
	}

	// Invalid component name
	if m.SetComponentVisible("nonexistent", false) {
		t.Fatal("SetComponentVisible should return false for invalid name")
	}
	// Unknown components default to visible
	if !m.IsComponentVisible("nonexistent") {
		t.Fatal("unknown components should default to visible")
	}
}

func TestAddComponent(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// Add a component
	if !m.AddComponent("clock", "12:34", "top_right") {
		t.Fatal("AddComponent should return true for valid position")
	}

	comps := m.Components()
	if len(comps) != 1 {
		t.Fatalf("expected 1 component, got %d", len(comps))
	}
	if comps[0].ID != "clock" {
		t.Fatalf("expected ID %q, got %q", "clock", comps[0].ID)
	}
	if comps[0].Position != PosTopRight {
		t.Fatalf("expected position %q, got %q", PosTopRight, comps[0].Position)
	}
	if comps[0].Content != "12:34" {
		t.Fatalf("expected content %q, got %q", "12:34", comps[0].Content)
	}

	// Replace component with same ID
	m.AddComponent("clock", "15:00", "floating")
	comps = m.Components()
	if len(comps) != 1 {
		t.Fatalf("expected 1 component after replace, got %d", len(comps))
	}
	if comps[0].Content != "15:00" {
		t.Fatalf("expected replaced content %q, got %q", "15:00", comps[0].Content)
	}
	if comps[0].Position != PosFloating {
		t.Fatalf("expected replaced position %q, got %q", PosFloating, comps[0].Position)
	}

	// Add second component
	m.AddComponent("weather", "Sunny", "bottom_left")
	comps = m.Components()
	if len(comps) != 2 {
		t.Fatalf("expected 2 components, got %d", len(comps))
	}

	// Remove component
	m.RemoveComponent("clock")
	comps = m.Components()
	if len(comps) != 1 {
		t.Fatalf("expected 1 component after remove, got %d", len(comps))
	}
	if comps[0].ID != "weather" {
		t.Fatalf("expected remaining ID %q, got %q", "weather", comps[0].ID)
	}

	// Invalid position
	if m.AddComponent("bad", "content", "invalid_position") {
		t.Fatal("AddComponent should return false for invalid position")
	}
}

func TestCustomBanner(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// No banner by default
	if banner := m.CustomBanner(); banner != "" {
		t.Fatalf("expected empty banner by default, got %q", banner)
	}

	// Set banner
	m.SetBanner("  Custom Banner!")
	if banner := m.CustomBanner(); banner != "  Custom Banner!" {
		t.Fatalf("expected %q, got %q", "  Custom Banner!", banner)
	}

	// Clear banner
	m.SetBanner("")
	if banner := m.CustomBanner(); banner != "" {
		t.Fatalf("expected empty banner after clear, got %q", banner)
	}
}

func TestComponentsSnapshot(t *testing.T) {
	m := NewManager()
	defer m.Close()

	m.AddComponent("a", "content_a", "floating")
	m.AddComponent("b", "content_b", "top_left")

	// Take snapshot
	snapshot := m.Components()

	// Modify after snapshot
	m.RemoveComponent("a")

	// Snapshot should be unaffected (it's a copy)
	if len(snapshot) != 2 {
		t.Fatalf("expected snapshot to have 2 components, got %d", len(snapshot))
	}

	// Live query should reflect removal
	live := m.Components()
	if len(live) != 1 {
		t.Fatalf("expected live to have 1 component, got %d", len(live))
	}
}
