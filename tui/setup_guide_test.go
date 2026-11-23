package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var spaceKey = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}

// Regression: Bubble Tea v2 reports the space bar as "space", so every
// "space" hint of the setup guide did nothing.
func TestSetupGuideSpaceKey(t *testing.T) {
	m := NewSetupGuide(false, false, nil, nil)
	m.Update(spaceKey)
	if m.step != setupStepFeatures {
		t.Fatalf("space on welcome: step = %d, want %d", m.step, setupStepFeatures)
	}

	m.step = setupStepShowcase
	m.showcaseTourStep = 1
	m.Update(spaceKey) // tooltip: "space next"
	if m.showcaseTourStep != 2 {
		t.Fatalf("tour step = %d, want 2", m.showcaseTourStep)
	}
	m.Update(spaceKey) // tooltip: "space skip"
	if m.showcaseTourStep != 4 {
		t.Fatalf("tour step = %d, want 4", m.showcaseTourStep)
	}
}

// The showcase inbox rows must end in the same column (selected or not) and
// no step may render wider or taller than the window.
func TestSetupGuideFitsWindow(t *testing.T) {
	for _, sz := range [][2]int{{200, 60}, {120, 40}, {80, 24}, {60, 20}, {40, 12}} {
		w, h := sz[0], sz[1]
		for step := setupStepWelcome; step <= setupStepDone; step++ {
			for _, mode := range []int{0, 1} {
				m := NewSetupGuide(true, false, nil, nil)
				m.Update(tea.WindowSizeMsg{Width: w, Height: h})
				m.step = step
				m.showcaseMode = mode
				assertViewFits(t, fmt.Sprintf("step %d mode %d @%dx%d", step, mode, w, h), m.View().Content, w, h)
			}
		}
	}

	m := NewSetupGuide(false, false, nil, nil)
	pane := m.viewTourInboxPane(55, 24)
	var widths []int
	for _, l := range strings.Split(pane, "\n") {
		if strings.Contains(l, "·") { // inbox rows
			widths = append(widths, lipgloss.Width(l))
		}
	}
	for _, w := range widths {
		if w != 55 {
			t.Fatalf("inbox row widths = %v, want all 55", widths)
		}
	}
}
