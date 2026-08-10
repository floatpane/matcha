package tui

import (
	"strings"
	"testing"

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

	// The top-right component should appear on the first line.
	// With an 80-char terminal and a 5-char "12:34" block, there should be
	// (80 - 5) = 75 spaces of padding before the content on that line.
	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}

	topLine := lines[0]
	if !strings.HasSuffix(strings.TrimRight(topLine, " "), "12:34") {
		t.Fatalf("expected first line to end with %q (right-aligned), got %q", "12:34", topLine)
	}
}
