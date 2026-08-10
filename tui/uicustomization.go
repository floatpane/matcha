package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	overlay "github.com/floatpane/bubble-overlay"
	"github.com/floatpane/matcha/plugin"
	"github.com/floatpane/matcha/theme"
)

// UIProvider is the interface the TUI uses to query plugin-driven UI
// customization state. The plugin.Manager satisfies this interface; tests can
// provide a stub. It is set once at startup via SetUIProvider and read by the
// rendering helpers below.
type UIProvider interface {
	TextOverride(key string) (string, bool)
	IsComponentVisible(name string) bool
	Components() []plugin.CustomComponent
	CustomBanner() string
}

// uiProvider holds the plugin manager reference for UI customization queries.
// It is wired in main.go (or app.NewModel) after the plugin manager is created,
// following the same pattern as BodyTransformer. When nil (no plugins loaded),
// all helpers fall back to default behaviour.
var uiProvider UIProvider

// SetUIProvider wires the plugin manager into the TUI rendering layer so that
// text overrides, visibility toggles, custom components, and banner overrides
// are consulted during rendering.
func SetUIProvider(p UIProvider) {
	uiProvider = p
}

// --- Text override integration -------------------------------------------

// overriddenT checks the plugin text override registry first, then falls back
// to the i18n locale bundle. This is the core integration point: every t()
// call in the TUI layer routes through here once SetUIProvider has been called.
//
// If no provider is set, or no override exists for the key, the standard i18n
// translation is returned, preserving existing behaviour exactly.
func overriddenT(key string) string {
	if uiProvider != nil {
		if val, ok := uiProvider.TextOverride(key); ok {
			return val
		}
	}
	return i18nT(key)
}

// --- Banner override integration -----------------------------------------

// renderLogo returns the styled logo string for the startup/choice screen.
// If a plugin has set a custom banner via matcha.ui.set_banner(), that string
// is rendered with the same accent-colour styling as the default logo.
// Otherwise the default logo is used.
//
// defaultLogo is the ASCII art constant from the calling view (choiceLogo or
// asciiLogo), passed in so this helper can be shared between Choice and Status.
func renderLogo(defaultLogo string) string {
	logoStyle := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent)
	if uiProvider != nil {
		if banner := uiProvider.CustomBanner(); banner != "" {
			return logoStyle.Render(banner)
		}
	}
	return logoStyle.Render(defaultLogo)
}

// --- Visibility helpers ---------------------------------------------------

// isUIVisible returns true if the named component should be drawn. When no
// provider is set, all components default to visible.
func isUIVisible(name string) bool {
	if uiProvider == nil {
		return true
	}
	return uiProvider.IsComponentVisible(name)
}

// --- Custom component rendering -------------------------------------------

// renderCustomComponents composites plugin-injected components onto the main
// content string. Components with PosFloating are placed as overlays using the
// bubble-overlay library (same mechanism as the move/jump overlays). Anchored
// components (PosTopLeft, PosTopRight, PosBottomLeft, PosBottomRight) are
// joined into the content flow.
//
// width and height are the current terminal dimensions, used for overlay
// placement calculations.
func renderCustomComponents(content string, width, height int) string {
	if uiProvider == nil {
		return content
	}
	components := uiProvider.Components()
	if len(components) == 0 {
		return content
	}

	// Separate floating (overlay) from anchored (flow) components.
	var floating []plugin.CustomComponent
	var topLeft, topRight, bottomLeft, bottomRight []plugin.CustomComponent

	for _, c := range components {
		switch c.Position {
		case plugin.PosFloating:
			floating = append(floating, c)
		case plugin.PosTopLeft:
			topLeft = append(topLeft, c)
		case plugin.PosTopRight:
			topRight = append(topRight, c)
		case plugin.PosBottomLeft:
			bottomLeft = append(bottomLeft, c)
		case plugin.PosBottomRight:
			bottomRight = append(bottomRight, c)
		}
	}

	// --- Anchored components: join into content flow ---
	//
	// Top-anchored components are stacked above the main content; bottom-
	// anchored components are stacked below. Left and right at the same
	// vertical anchor are placed side by side with a gap that fills the
	// full terminal width, so a lone "top_right" component is properly
	// right-aligned and a lone "top_left" is left-aligned.

	if len(topLeft) > 0 || len(topRight) > 0 {
		leftBlock := ""
		if len(topLeft) > 0 {
			leftBlock = joinComponentBlocks(topLeft)
		}
		rightBlock := ""
		if len(topRight) > 0 {
			rightBlock = joinComponentBlocks(topRight)
		}
		gap := width - lipgloss.Width(leftBlock) - lipgloss.Width(rightBlock)
		if gap < 0 {
			gap = 0
		}
		topBar := lipgloss.JoinHorizontal(lipgloss.Top,
			leftBlock,
			strings.Repeat(" ", gap),
			rightBlock,
		)
		content = lipgloss.JoinVertical(lipgloss.Left, topBar, content)
	}

	if len(bottomLeft) > 0 || len(bottomRight) > 0 {
		leftBlock := ""
		if len(bottomLeft) > 0 {
			leftBlock = joinComponentBlocks(bottomLeft)
		}
		rightBlock := ""
		if len(bottomRight) > 0 {
			rightBlock = joinComponentBlocks(bottomRight)
		}
		gap := width - lipgloss.Width(leftBlock) - lipgloss.Width(rightBlock)
		if gap < 0 {
			gap = 0
		}
		bottomBar := lipgloss.JoinHorizontal(lipgloss.Bottom,
			leftBlock,
			strings.Repeat(" ", gap),
			rightBlock,
		)
		content = lipgloss.JoinVertical(lipgloss.Left, content, bottomBar)
	}

	// --- Floating components: overlay on top of content ---
	//
	// Each floating component is composited as a centered overlay. When
	// multiple floating components exist, they are stacked: each successive
	// overlay is placed on top of the previous composited result. This
	// matches how the existing move/jump overlays work.
	for _, c := range floating {
		box := componentOverlayStyle.Render(c.Content)
		content = overlay.Center(content, box, width, height)
	}

	return content
}

// joinComponentBlocks concatenates the content of multiple components
// vertically, separated by newlines.
func joinComponentBlocks(components []plugin.CustomComponent) string {
	blocks := make([]string, 0, len(components))
	for _, c := range components {
		blocks = append(blocks, c.Content)
	}
	return strings.Join(blocks, "\n")
}

// componentOverlayStyle gives floating custom components a subtle border so
// they are visually distinct from the underlying content. Plugins that want a
// different look can pre-style their content with matcha.style before passing
// it to add_component.
var componentOverlayStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(theme.ActiveTheme.AccentDark).
	Padding(0, 1)
