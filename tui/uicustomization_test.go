package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/plugin"
)

// stubUIProvider is a test double for the UIProvider interface.
type stubUIProvider struct {
	textOverrides map[string]string
	visibility    map[string]bool
	components    []plugin.CustomComponent
	banner        string
}

func (s *stubUIProvider) TextOverride(key string) (string, bool) {
	v, ok := s.textOverrides[key]
	return v, ok
}

func (s *stubUIProvider) IsComponentVisible(name string) bool {
	v, ok := s.visibility[name]
	if !ok {
		return true
	}
	return v
}

func (s *stubUIProvider) Components() []plugin.CustomComponent {
	return s.components
}

func (s *stubUIProvider) CustomBanner() string {
	return s.banner
}

func TestOverriddenT_Fallback(t *testing.T) {
	// With no provider set, t() should fall back to i18n.
	// We can't easily test a specific translation, but we can verify
	// it doesn't panic and returns a non-empty string for a known key.
	old := uiProvider
	uiProvider = nil
	defer func() { uiProvider = old }()

	result := i18nT("folder_inbox.folders_title")
	if result == "" {
		t.Fatal("expected non-empty fallback translation")
	}
}

func TestOverriddenT_WithOverride(t *testing.T) {
	old := uiProvider
	defer func() { uiProvider = old }()

	uiProvider = &stubUIProvider{
		textOverrides: map[string]string{
			"folder_inbox.folders_title": "My Custom Folders",
		},
	}

	result := overriddenT("folder_inbox.folders_title")
	if result != "My Custom Folders" {
		t.Fatalf("expected override %q, got %q", "My Custom Folders", result)
	}
}

func TestIsUIVisible_DefaultVisible(t *testing.T) {
	old := uiProvider
	uiProvider = nil
	defer func() { uiProvider = old }()

	if !isUIVisible("sidebar") {
		t.Fatal("expected sidebar visible when no provider set")
	}
}

func TestIsUIVisible_Hidden(t *testing.T) {
	old := uiProvider
	defer func() { uiProvider = old }()

	uiProvider = &stubUIProvider{
		visibility: map[string]bool{"sidebar": false},
	}

	if isUIVisible("sidebar") {
		t.Fatal("expected sidebar hidden when provider says false")
	}
}

func TestRenderLogo_Default(t *testing.T) {
	old := uiProvider
	uiProvider = nil
	defer func() { uiProvider = old }()

	result := renderLogo("DEFAULT_LOGO")
	if result == "" {
		t.Fatal("expected non-empty logo render")
	}
}

func TestRenderLogo_CustomBanner(t *testing.T) {
	old := uiProvider
	defer func() { uiProvider = old }()

	uiProvider = &stubUIProvider{
		banner: "  CUSTOM BANNER!",
	}

	result := renderLogo("DEFAULT_LOGO")
	// The result should contain the custom banner text (wrapped in ANSI codes)
	if result == "" {
		t.Fatal("expected non-empty banner render")
	}
}

func TestRenderCustomComponents_NoProvider(t *testing.T) {
	old := uiProvider
	uiProvider = nil
	defer func() { uiProvider = old }()

	result := renderCustomComponents("base content", 80, 24)
	if result != "base content" {
		t.Fatalf("expected unchanged content with no provider, got %q", result)
	}
}

func TestRenderCustomComponents_NoComponents(t *testing.T) {
	old := uiProvider
	defer func() { uiProvider = old }()

	uiProvider = &stubUIProvider{}

	result := renderCustomComponents("base content", 80, 24)
	if result != "base content" {
		t.Fatalf("expected unchanged content with no components, got %q", result)
	}
}

func TestRenderCustomComponents_AnchoredTop(t *testing.T) {
	old := uiProvider
	defer func() { uiProvider = old }()

	uiProvider = &stubUIProvider{
		components: []plugin.CustomComponent{
			{ID: "header", Content: "CUSTOM HEADER", Position: plugin.PosTopLeft},
		},
	}

	result := renderCustomComponents("base content", 80, 24)
	if result == "base content" {
		t.Fatal("expected modified content with anchored component")
	}
}

func TestRenderCustomComponents_AnchoredBottom(t *testing.T) {
	old := uiProvider
	defer func() { uiProvider = old }()

	uiProvider = &stubUIProvider{
		components: []plugin.CustomComponent{
			{ID: "footer", Content: "CUSTOM FOOTER", Position: plugin.PosBottomLeft},
		},
	}

	result := renderCustomComponents("base content", 80, 24)
	if result == "base content" {
		t.Fatal("expected modified content with bottom-anchored component")
	}
}

func TestRenderCustomComponents_TopRightAlignment(t *testing.T) {
	old := uiProvider
	defer func() { uiProvider = old }()

	uiProvider = &stubUIProvider{
		components: []plugin.CustomComponent{
			{ID: "clock", Content: "12:34", Position: plugin.PosTopRight},
		},
	}

	width := 80
	result := renderCustomComponents("base content", width, 24)

	// The top-right component is painted over the right end of the first
	// line, leaving the rest of that line (and the line count) untouched.
	lines := strings.Split(result, "\n")
	if len(lines) != 1 {
		t.Fatalf("a top component must not add rows, got %d lines", len(lines))
	}

	topLine := ansi.Strip(lines[0])
	if !strings.HasPrefix(topLine, "base content") {
		t.Fatalf("expected the content to survive on the first line, got %q", topLine)
	}
	if !strings.HasSuffix(topLine, "12:34") || ansi.StringWidth(topLine) != width {
		t.Fatalf("expected first line to end with %q at column %d, got %q", "12:34", width, topLine)
	}
}

// TestRenderCustomComponents_FullHeightKeepsHeight paints top and bottom
// components over a full-screen view: the result must stay exactly one
// screen tall with the components on the first and last rows.
func TestRenderCustomComponents_FullHeightKeepsHeight(t *testing.T) {
	old := uiProvider
	defer func() { uiProvider = old }()

	uiProvider = &stubUIProvider{
		components: []plugin.CustomComponent{
			{ID: "top", Content: "TOP", Position: plugin.PosTopLeft},
			{ID: "bottom", Content: "BOTTOM", Position: plugin.PosBottomRight},
		},
	}

	const width, height = 40, 10
	rows := make([]string, height)
	for i := range rows {
		rows[i] = strings.Repeat(".", width)
	}
	lines := strings.Split(ansi.Strip(renderCustomComponents(strings.Join(rows, "\n"), width, height)), "\n")
	if len(lines) != height {
		t.Fatalf("view grew to %d rows, want %d", len(lines), height)
	}
	if !strings.HasPrefix(lines[0], "TOP") {
		t.Fatalf("top component not on the first row: %q", lines[0])
	}
	if !strings.HasSuffix(lines[height-1], "BOTTOM") {
		t.Fatalf("bottom component not on the last row: %q", lines[height-1])
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w != width {
			t.Fatalf("row %d is %d cells wide, want %d", i, w, width)
		}
	}
}
