package view

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"charm.land/lipgloss/v2"
)

// clearAllTerminalEnv clears all environment variables that could indicate terminal capabilities
func clearAllTerminalEnv() {
	// Clear hyperlink support indicators
	os.Unsetenv("VTE_VERSION")
	os.Unsetenv("KITTY_WINDOW_ID")
	os.Unsetenv("GHOSTTY_RESOURCES_DIR")
	os.Unsetenv("WEZTERM_EXECUTABLE")
	os.Unsetenv("WEZTERM_CONFIG_FILE")
	os.Unsetenv("ITERM_SESSION_ID")
	os.Unsetenv("ITERM_PROFILE")
	os.Unsetenv("WARP_IS_LOCAL_SHELL_SESSION")
	os.Unsetenv("WARP_COMBINED_PROMPT_COMMAND_FINISHED")
	os.Unsetenv("KONSOLE_DBUS_SESSION")
	os.Unsetenv("KONSOLE_VERSION")

	// Set basic terminal that doesn't support anything special
	os.Setenv("TERM", "xterm")
	os.Setenv("TERM_PROGRAM", "basic")
}

func TestDecodeQuotedPrintable(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple case",
			input:    "Hello=2C world=21",
			expected: "Hello, world!",
		},
		{
			name:     "With soft line break",
			input:    "This is a long line that gets wrapped=\r\n and continues here.",
			expected: "This is a long line that gets wrapped and continues here.",
		},
		{
			name:     "No encoding",
			input:    "Just a plain string.",
			expected: "Just a plain string.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := decodeQuotedPrintable(tc.input)
			if err != nil {
				t.Fatalf("decodeQuotedPrintable() failed: %v", err)
			}
			if decoded != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, decoded)
			}
		})
	}
}

func TestDebugImageProtocolUsesLogger(t *testing.T) {
	t.Setenv("DEBUG_IMAGE_PROTOCOL", "1")
	t.Setenv("DEBUG_IMAGE_PROTOCOL_LOG", "")
	t.Setenv("DEBUG_KITTY_IMAGES", "")
	t.Setenv("DEBUG_KITTY_LOG", "")

	var logBuf bytes.Buffer
	originalLogOutput := log.Writer()
	originalLogFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(originalLogOutput)
		log.SetFlags(originalLogFlags)
	})

	debugImageProtocol("hello %s", "world")

	want := "[img-protocol] hello world\n"
	if got := logBuf.String(); got != want {
		t.Fatalf("debugImageProtocol log output = %q, want %q", got, want)
	}
}

func TestMarkdownToHTML(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Heading",
			input:    "# Hello",
			expected: "<h1>Hello</h1>",
		},
		{
			name:     "Bold",
			input:    "**bold text**",
			expected: "<p><strong>bold text</strong></p>",
		},
		{
			name:     "Link",
			input:    "[link](http://example.com)",
			expected: `<p><a href="http://example.com">link</a></p>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			html := markdownToHTML([]byte(tc.input))
			// Trim newlines for consistent comparison
			if strings.TrimSpace(string(html)) != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, html)
			}
		})
	}
}

func TestGhosttySupported(t *testing.T) {
	// Save original environment variables
	origTerm := os.Getenv("TERM")
	origTermProgram := os.Getenv("TERM_PROGRAM")
	origGhosttyResources := os.Getenv("GHOSTTY_RESOURCES_DIR")

	// Restore environment variables after test
	defer func() {
		os.Setenv("TERM", origTerm)
		os.Setenv("TERM_PROGRAM", origTermProgram)
		os.Setenv("GHOSTTY_RESOURCES_DIR", origGhosttyResources)
	}()

	testCases := []struct {
		name                string
		term                string
		termProgram         string
		ghosttyResourcesDir string
		expected            bool
	}{
		{
			name:                "No Ghostty environment variables",
			term:                "xterm",
			termProgram:         "",
			ghosttyResourcesDir: "",
			expected:            false,
		},
		{
			name:                "TERM contains ghostty",
			term:                "xterm-ghostty",
			termProgram:         "",
			ghosttyResourcesDir: "",
			expected:            true,
		},
		{
			name:                "TERM_PROGRAM is ghostty",
			term:                "xterm",
			termProgram:         "ghostty",
			ghosttyResourcesDir: "",
			expected:            true,
		},
		{
			name:                "GHOSTTY_RESOURCES_DIR is set",
			term:                "xterm",
			termProgram:         "",
			ghosttyResourcesDir: "/usr/share/ghostty",
			expected:            true,
		},
		{
			name:                "Multiple Ghostty indicators",
			term:                "ghostty",
			termProgram:         "ghostty",
			ghosttyResourcesDir: "/usr/share/ghostty",
			expected:            true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("TERM", tc.term)
			os.Setenv("TERM_PROGRAM", tc.termProgram)
			os.Setenv("GHOSTTY_RESOURCES_DIR", tc.ghosttyResourcesDir)

			result := ghosttySupported()
			if result != tc.expected {
				t.Errorf("Expected %t, got %t", tc.expected, result)
			}
		})
	}
}

func TestZellijDetection(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected bool
	}{
		{"ZELLIJ set", map[string]string{"ZELLIJ": "1"}, true},
		{"ZELLIJ_SESSION_NAME set", map[string]string{"ZELLIJ_SESSION_NAME": "test"}, true},
		{"No Zellij", map[string]string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env
			origZellij := os.Getenv("ZELLIJ")
			origZellijSession := os.Getenv("ZELLIJ_SESSION_NAME")
			defer func() {
				if origZellij != "" {
					os.Setenv("ZELLIJ", origZellij)
				} else {
					os.Unsetenv("ZELLIJ")
				}
				if origZellijSession != "" {
					os.Setenv("ZELLIJ_SESSION_NAME", origZellijSession)
				} else {
					os.Unsetenv("ZELLIJ_SESSION_NAME")
				}
			}()

			// Clear first
			os.Unsetenv("ZELLIJ")
			os.Unsetenv("ZELLIJ_SESSION_NAME")

			// Set test env
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			if got := zellijSupported(); got != tt.expected {
				t.Errorf("zellijSupported() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSixelDetection(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected bool
	}{
		{"Zellij", map[string]string{"ZELLIJ": "1"}, true},
		{"MLterm", map[string]string{"TERM": "mlterm"}, true},
		{"foot", map[string]string{"TERM": "foot"}, true},
		{"xterm with SIXEL", map[string]string{"TERM": "xterm", "SIXEL": "1"}, true},
		{"plain xterm", map[string]string{"TERM": "xterm"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env
			origZellij := os.Getenv("ZELLIJ")
			origZellijSession := os.Getenv("ZELLIJ_SESSION_NAME")
			origTerm := os.Getenv("TERM")
			origSixel := os.Getenv("SIXEL")
			defer func() {
				if origZellij != "" {
					os.Setenv("ZELLIJ", origZellij)
				} else {
					os.Unsetenv("ZELLIJ")
				}
				if origZellijSession != "" {
					os.Setenv("ZELLIJ_SESSION_NAME", origZellijSession)
				} else {
					os.Unsetenv("ZELLIJ_SESSION_NAME")
				}
				if origTerm != "" {
					os.Setenv("TERM", origTerm)
				} else {
					os.Unsetenv("TERM")
				}
				if origSixel != "" {
					os.Setenv("SIXEL", origSixel)
				} else {
					os.Unsetenv("SIXEL")
				}
			}()

			// Clear all env first
			os.Unsetenv("ZELLIJ")
			os.Unsetenv("ZELLIJ_SESSION_NAME")
			os.Unsetenv("TERM")
			os.Unsetenv("SIXEL")

			// Set test env
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			if got := sixelSupported(); got != tt.expected {
				t.Errorf("sixelSupported() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestImageProtocolSupported(t *testing.T) {
	// Save original environment variables
	origTerm := os.Getenv("TERM")
	origKittyWindow := os.Getenv("KITTY_WINDOW_ID")
	origTermProgram := os.Getenv("TERM_PROGRAM")
	origGhosttyResources := os.Getenv("GHOSTTY_RESOURCES_DIR")
	origItermlSession := os.Getenv("ITERM_SESSION_ID")
	origWeztermExec := os.Getenv("WEZTERM_EXECUTABLE")
	origWarpLocal := os.Getenv("WARP_IS_LOCAL_SHELL_SESSION")
	origKonsoleDBus := os.Getenv("KONSOLE_DBUS_SESSION")

	// Restore environment variables after test
	defer func() {
		os.Setenv("TERM", origTerm)
		os.Setenv("KITTY_WINDOW_ID", origKittyWindow)
		os.Setenv("TERM_PROGRAM", origTermProgram)
		os.Setenv("GHOSTTY_RESOURCES_DIR", origGhosttyResources)
		os.Setenv("ITERM_SESSION_ID", origItermlSession)
		os.Setenv("WEZTERM_EXECUTABLE", origWeztermExec)
		os.Setenv("WARP_IS_LOCAL_SHELL_SESSION", origWarpLocal)
		os.Setenv("KONSOLE_DBUS_SESSION", origKonsoleDBus)
	}()

	testCases := []struct {
		name        string
		setupEnv    func()
		clearAllEnv func()
		expected    bool
	}{
		{
			name: "No supported terminals",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("TERM_PROGRAM", "basic")
			},
			clearAllEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WEZTERM_EXECUTABLE")
				os.Unsetenv("WARP_IS_LOCAL_SHELL_SESSION")
				os.Unsetenv("KONSOLE_DBUS_SESSION")
			},
			expected: false,
		},
		{
			name: "Kitty supported via TERM",
			setupEnv: func() {
				os.Setenv("TERM", "xterm-kitty")
			},
			clearAllEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WEZTERM_EXECUTABLE")
				os.Unsetenv("WARP_IS_LOCAL_SHELL_SESSION")
				os.Unsetenv("KONSOLE_DBUS_SESSION")
			},
			expected: true,
		},
		{
			name: "Kitty supported via KITTY_WINDOW_ID",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("KITTY_WINDOW_ID", "1")
			},
			clearAllEnv: func() {
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WEZTERM_EXECUTABLE")
				os.Unsetenv("WARP_IS_LOCAL_SHELL_SESSION")
				os.Unsetenv("KONSOLE_DBUS_SESSION")
			},
			expected: true,
		},
		{
			name: "Ghostty supported via TERM_PROGRAM",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("TERM_PROGRAM", "ghostty")
			},
			clearAllEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WEZTERM_EXECUTABLE")
				os.Unsetenv("WARP_IS_LOCAL_SHELL_SESSION")
				os.Unsetenv("KONSOLE_DBUS_SESSION")
			},
			expected: true,
		},
		{
			name: "iTerm2 supported via TERM_PROGRAM",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("TERM_PROGRAM", "iterm.app")
			},
			clearAllEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WEZTERM_EXECUTABLE")
				os.Unsetenv("WARP_IS_LOCAL_SHELL_SESSION")
				os.Unsetenv("KONSOLE_DBUS_SESSION")
			},
			expected: true,
		},
		{
			name: "WezTerm supported via WEZTERM_EXECUTABLE",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("WEZTERM_EXECUTABLE", "/usr/bin/wezterm")
			},
			clearAllEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WARP_IS_LOCAL_SHELL_SESSION")
				os.Unsetenv("KONSOLE_DBUS_SESSION")
			},
			expected: true,
		},
		{
			name: "Warp supported via WARP_IS_LOCAL_SHELL_SESSION",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("WARP_IS_LOCAL_SHELL_SESSION", "1")
			},
			clearAllEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WEZTERM_EXECUTABLE")
				os.Unsetenv("KONSOLE_DBUS_SESSION")
			},
			expected: true,
		},
		{
			name: "Konsole supported via KONSOLE_DBUS_SESSION",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("KONSOLE_DBUS_SESSION", "/Sessions/1")
			},
			clearAllEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WEZTERM_EXECUTABLE")
				os.Unsetenv("WARP_IS_LOCAL_SHELL_SESSION")
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.clearAllEnv()
			tc.setupEnv()

			result := imageProtocolSupported()
			if result != tc.expected {
				t.Errorf("Expected %t, got %t", tc.expected, result)
			}
		})
	}
}

func TestHyperlinkSupported(t *testing.T) {
	// Save original environment variables
	origTerm := os.Getenv("TERM")
	origTermProgram := os.Getenv("TERM_PROGRAM")
	origVTEVersion := os.Getenv("VTE_VERSION")
	origKittyWindow := os.Getenv("KITTY_WINDOW_ID")
	origGhosttyResources := os.Getenv("GHOSTTY_RESOURCES_DIR")
	origWeztermExec := os.Getenv("WEZTERM_EXECUTABLE")

	// Restore environment variables after test
	defer func() {
		os.Setenv("TERM", origTerm)
		os.Setenv("TERM_PROGRAM", origTermProgram)
		os.Setenv("VTE_VERSION", origVTEVersion)
		os.Setenv("KITTY_WINDOW_ID", origKittyWindow)
		os.Setenv("GHOSTTY_RESOURCES_DIR", origGhosttyResources)
		os.Setenv("WEZTERM_EXECUTABLE", origWeztermExec)
	}()

	testCases := []struct {
		name        string
		setupEnv    func()
		clearAllEnv func()
		expected    bool
	}{
		{
			name: "No hyperlink support",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("TERM_PROGRAM", "basic")
			},
			clearAllEnv: func() {
				os.Unsetenv("VTE_VERSION")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("WEZTERM_EXECUTABLE")
			},
			expected: false,
		},
		{
			name: "Kitty hyperlink support via TERM",
			setupEnv: func() {
				os.Setenv("TERM", "xterm-kitty")
			},
			clearAllEnv: func() {
				os.Unsetenv("VTE_VERSION")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("WEZTERM_EXECUTABLE")
			},
			expected: true,
		},
		{
			name: "VTE-based terminal hyperlink support",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("VTE_VERSION", "0.60.3")
			},
			clearAllEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("WEZTERM_EXECUTABLE")
			},
			expected: true,
		},
		{
			name: "iTerm2 hyperlink support",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("TERM_PROGRAM", "iterm.app")
			},
			clearAllEnv: func() {
				os.Unsetenv("VTE_VERSION")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("WEZTERM_EXECUTABLE")
			},
			expected: true,
		},
		{
			name: "WezTerm hyperlink support",
			setupEnv: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("WEZTERM_EXECUTABLE", "/usr/bin/wezterm")
			},
			clearAllEnv: func() {
				os.Unsetenv("VTE_VERSION")
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.clearAllEnv()
			tc.setupEnv()

			result := hyperlinkSupported()
			if result != tc.expected {
				t.Errorf("Expected %t, got %t", tc.expected, result)
			}
		})
	}
}

func TestProcessBodyWithHyperlinkSupport(t *testing.T) {
	// Save original environment variables
	origTerm := os.Getenv("TERM")
	origTermProgram := os.Getenv("TERM_PROGRAM")
	origVTEVersion := os.Getenv("VTE_VERSION")
	origKittyWindow := os.Getenv("KITTY_WINDOW_ID")

	// Restore environment variables after test
	defer func() {
		os.Setenv("TERM", origTerm)
		os.Setenv("TERM_PROGRAM", origTermProgram)
		os.Setenv("VTE_VERSION", origVTEVersion)
		os.Setenv("KITTY_WINDOW_ID", origKittyWindow)
	}()

	h1Style := lipgloss.NewStyle().SetString("H1")
	h2Style := lipgloss.NewStyle().SetString("H2")
	bodyStyle := lipgloss.NewStyle().SetString("BODY")

	testCases := []struct {
		name                string
		setupHyperlinks     func()
		input               string
		expectedContains    string
		expectedNotContains string
	}{
		{
			name: "Link with hyperlink support",
			setupHyperlinks: func() {
				os.Setenv("TERM", "xterm-kitty")
				os.Unsetenv("VTE_VERSION")
				os.Unsetenv("KITTY_WINDOW_ID")
			},
			input:               `<a href="http://example.com">Click here</a>`,
			expectedContains:    "Click here",
			expectedNotContains: "<http://example.com>",
		},
		{
			name: "Link without hyperlink support",
			setupHyperlinks: func() {
				clearAllTerminalEnv()
			},
			input:            `<a href="http://example.com">Click here</a>`,
			expectedContains: "Click here <http://example.com>",
		},
		{
			name: "Image link with hyperlink support",
			setupHyperlinks: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("VTE_VERSION", "0.60.3")
				os.Unsetenv("KITTY_WINDOW_ID")
			},
			input:               `<img src="http://example.com/img.png" alt="alt text">`,
			expectedContains:    "[Click here to view image: alt text]",
			expectedNotContains: "<http://example.com/img.png>",
		},
		{
			name: "Image link without hyperlink support",
			setupHyperlinks: func() {
				clearAllTerminalEnv()
			},
			input:            `<img src="http://example.com/img.png" alt="alt text">`,
			expectedContains: "[Image: alt text, http://example.com/img.png]",
		},
	}

	// Regex to strip out ANSI SGR escape codes (e.g. \x1b[38;2;...m)
	ansiEscapeRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupHyperlinks()

			processed, _, err := ProcessBody(tc.input, "", h1Style, h2Style, bodyStyle, false)
			if err != nil {
				t.Fatalf("ProcessBody() failed: %v", err)
			}

			cleanProcessed := ansiEscapeRegex.ReplaceAllString(processed, "")

			if !strings.Contains(cleanProcessed, tc.expectedContains) {
				t.Errorf("Processed body does not contain expected text.\nGot: %q\nWant to contain: %q", cleanProcessed, tc.expectedContains)
			}

			if tc.expectedNotContains != "" && strings.Contains(cleanProcessed, tc.expectedNotContains) {
				t.Errorf("Processed body contains unexpected text.\nGot: %q\nShould not contain: %q", cleanProcessed, tc.expectedNotContains)
			}
		})
	}
}

func TestProcessBodyWithImageProtocol(t *testing.T) {
	// Save original environment variables
	origTerm := os.Getenv("TERM")
	origTermProgram := os.Getenv("TERM_PROGRAM")
	origKittyWindow := os.Getenv("KITTY_WINDOW_ID")
	origGhosttyResources := os.Getenv("GHOSTTY_RESOURCES_DIR")
	origItermlSession := os.Getenv("ITERM_SESSION_ID")
	origWeztermExec := os.Getenv("WEZTERM_EXECUTABLE")

	// Restore environment variables after test
	defer func() {
		os.Setenv("TERM", origTerm)
		os.Setenv("TERM_PROGRAM", origTermProgram)
		os.Setenv("KITTY_WINDOW_ID", origKittyWindow)
		os.Setenv("GHOSTTY_RESOURCES_DIR", origGhosttyResources)
		os.Setenv("ITERM_SESSION_ID", origItermlSession)
		os.Setenv("WEZTERM_EXECUTABLE", origWeztermExec)
	}()

	h1Style := lipgloss.NewStyle().SetString("H1")
	h2Style := lipgloss.NewStyle().SetString("H2")
	bodyStyle := lipgloss.NewStyle().SetString("BODY")

	// Create a simple base64 PNG image (1x1 pixel white PNG)
	testBase64PNG := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="

	testCases := []struct {
		name                string
		setupImageProtocol  func()
		clearAllImageEnv    func()
		input               string
		expectedContains    string
		expectedNotContains string
		expectPlacements    bool
	}{
		{
			name: "Data URI image with Kitty support returns placement",
			setupImageProtocol: func() {
				os.Setenv("TERM", "xterm-kitty")
			},
			clearAllImageEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WEZTERM_EXECUTABLE")
			},
			input:               `<img src="data:image/png;base64,` + testBase64PNG + `" alt="test image">`,
			expectedNotContains: "[Image: test image,",
			expectPlacements:    true,
		},
		{
			name: "Data URI image with iTerm2 support returns placement",
			setupImageProtocol: func() {
				os.Setenv("TERM", "xterm")
				os.Setenv("TERM_PROGRAM", "iterm.app")
			},
			clearAllImageEnv: func() {
				os.Unsetenv("KITTY_WINDOW_ID")
				os.Unsetenv("GHOSTTY_RESOURCES_DIR")
				os.Unsetenv("ITERM_SESSION_ID")
				os.Unsetenv("WEZTERM_EXECUTABLE")
			},
			input:               `<img src="data:image/png;base64,` + testBase64PNG + `" alt="test image">`,
			expectedNotContains: "[Image: test image,",
			expectPlacements:    true,
		},
		{
			name: "Data URI image without protocol support",
			setupImageProtocol: func() {
				clearAllTerminalEnv()
			},
			clearAllImageEnv: func() {
				// This is handled by clearAllTerminalEnv now
			},
			input:            `<img src="data:image/png;base64,` + testBase64PNG + `" alt="test image">`,
			expectedContains: "[Image: test image,",
		},
		{
			name: "Remote image with WezTerm support (has hyperlink support)",
			setupImageProtocol: func() {
				clearAllTerminalEnv()
				os.Setenv("WEZTERM_EXECUTABLE", "/usr/bin/wezterm")
			},
			clearAllImageEnv: func() {
				// This is handled by clearAllTerminalEnv now
			},
			input:            `<img src="http://example.com/img.png" alt="remote image">`,
			expectedContains: "[Click here to view image: remote image]", // Remote images won't render without actual fetch, but hyperlinks work
		},
		{
			name: "Remote image without protocol support",
			setupImageProtocol: func() {
				clearAllTerminalEnv()
			},
			clearAllImageEnv: func() {
				// This is handled by clearAllTerminalEnv now
			},
			input:            `<img src="http://example.com/img.png" alt="remote image">`,
			expectedContains: "[Image: remote image,",
		},
	}

	ansiEscapeRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.clearAllImageEnv()
			tc.setupImageProtocol()

			processed, placements, err := ProcessBody(tc.input, "", h1Style, h2Style, bodyStyle, false)
			if err != nil {
				t.Fatalf("ProcessBody() failed: %v", err)
			}

			if tc.expectPlacements {
				if len(placements) == 0 {
					t.Errorf("Expected image placements but got none")
				} else {
					if placements[0].Base64 == "" {
						t.Errorf("Expected non-empty Base64 in placement")
					}
					if placements[0].Rows < 1 {
						t.Errorf("Expected Rows >= 1, got %d", placements[0].Rows)
					}
				}
			}

			cleanProcessed := ansiEscapeRegex.ReplaceAllString(processed, "")

			if tc.expectedContains != "" && !strings.Contains(cleanProcessed, tc.expectedContains) {
				t.Errorf("Processed body does not contain expected text.\nGot: %q\nWant to contain: %q", cleanProcessed, tc.expectedContains)
			}

			if tc.expectedNotContains != "" && strings.Contains(cleanProcessed, tc.expectedNotContains) {
				t.Errorf("Processed body contains unexpected text.\nGot: %q\nShould not contain: %q", cleanProcessed, tc.expectedNotContains)
			}
		})
	}
}

func TestProcessBody(t *testing.T) {
	h1Style := lipgloss.NewStyle().SetString("H1")
	h2Style := lipgloss.NewStyle().SetString("H2")
	bodyStyle := lipgloss.NewStyle().SetString("BODY")

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple HTML",
			input:    "<p>Hello, world!</p>",
			expected: "Hello, world!",
		},
		{
			name:     "With headers HTML",
			input:    "<h1>Header 1</h1>",
			expected: "Header 1",
		},
		{
			name:     "With headers Markdown",
			input:    "# Header 1",
			expected: "Header 1",
		},
		{
			name:     "Plain text",
			input:    "Just plain text without any markup",
			expected: "Just plain text without any markup",
		},
	}

	ansiEscapeRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			processed, _, err := ProcessBody(tc.input, "", h1Style, h2Style, bodyStyle, false)
			if err != nil {
				t.Fatalf("ProcessBody() failed: %v", err)
			}

			cleanProcessed := ansiEscapeRegex.ReplaceAllString(processed, "")

			if !strings.Contains(cleanProcessed, tc.expected) {
				t.Errorf("Processed body does not contain expected text.\nGot: %q\nWant to contain: %q", cleanProcessed, tc.expected)
			}
		})
	}
}

// datadogShapeHTML is the indented attribute-heavy table shape commonly
// produced by Datadog Daily Digest, marketing tools, and any sender that
// uses HTML <table> for layout. md4c's html_block rule rejects this shape
// (leading whitespace, attribute-laden opening tag), so the markdown
// pre-pass passes the literal text through, and htmlconv then renders the
// raw "<table cellpadding=..." tag as visible body text.
const datadogShapeHTML = `    <table cellpadding="0" cellspacing="0" border="0" width="710" style="border:1px solid #E7E7E7;">
      <tr>
        <td style="background-color: #632ca6; color: white;">
          <h1>The Daily Digest</h1>
        </td>
      </tr>
    </table>`

// TestProcessBody_LegacyPathManglesIndentedHTML pins the bug this PR fixes.
// With an empty MIME type, the renderer falls through to the legacy
// markdown→HTML pre-pass, which is what every body went through before this
// change. For Datadog-shape input the output literally contains the opening
// "<table cellpadding=..." text, which is what users see leaked into the
// inbox viewer. This test will pass on master too — it documents the bug,
// not the fix.
func TestProcessBody_LegacyPathManglesIndentedHTML(t *testing.T) {
	ansiEscapeRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	processed, _, err := ProcessBody(datadogShapeHTML, "", lipgloss.NewStyle(), lipgloss.NewStyle(), lipgloss.NewStyle(), false)
	if err != nil {
		t.Fatalf("ProcessBody(legacy) failed: %v", err)
	}
	clean := ansiEscapeRegex.ReplaceAllString(processed, "")
	if !strings.Contains(clean, "<table") {
		t.Errorf("legacy path should leak literal '<table' tag for indented attribute-heavy HTML — if this assertion stops firing, md4c's html_block handling has improved and this PR's premise needs re-evaluation. Got:\n%s", clean)
	}
}

// TestProcessBody_HTMLMIMETypeSkipsMarkdownPrepass is the fix counterpart to
// the legacy-mangling test above. Same input, but tagged "text/html", goes
// straight to htmlconv without the broken markdown pre-pass.
func TestProcessBody_HTMLMIMETypeSkipsMarkdownPrepass(t *testing.T) {
	ansiEscapeRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	bodyStyle := lipgloss.NewStyle()
	h1Style := lipgloss.NewStyle()
	h2Style := lipgloss.NewStyle()

	// Same input as TestProcessBody_LegacyPathManglesIndentedHTML — the
	// differential is purely the MIME-type argument.
	processed, _, err := ProcessBody(datadogShapeHTML, BodyMIMETypeHTML, h1Style, h2Style, bodyStyle, false)
	if err != nil {
		t.Fatalf("ProcessBody(text/html) failed: %v", err)
	}
	clean := ansiEscapeRegex.ReplaceAllString(processed, "")
	if strings.Contains(clean, "<table") {
		t.Errorf("text/html body should not leak literal '<table' tag. Got:\n%s", clean)
	}
	if !strings.Contains(clean, "The Daily Digest") {
		t.Errorf("expected text content 'The Daily Digest' in output. Got:\n%s", clean)
	}

	// Sanity: a body labeled as plain text falls through markdownToHTML and
	// preserves markdown semantics (heading rendering through the pipeline).
	mdBody := "# Heading One\n\nSome **bold** text."
	plainProcessed, _, err := ProcessBody(mdBody, BodyMIMETypePlain, h1Style, h2Style, bodyStyle, false)
	if err != nil {
		t.Fatalf("ProcessBody(text/plain) failed: %v", err)
	}
	plainClean := ansiEscapeRegex.ReplaceAllString(plainProcessed, "")
	if !strings.Contains(plainClean, "Heading One") {
		t.Errorf("text/plain body should still render markdown. Got:\n%s", plainClean)
	}
}

func TestRemoteImageCache_EvictsOldestWhenFull(t *testing.T) {
	// Start with a clean cache so prior tests don't interfere.
	remoteImageCache.Purge()
	// cleaning up the current test's cache
	defer remoteImageCache.Purge()

	// overfilling the cache beyond its configured capacity.
	overfillBy := 5
	totalInserts := remoteImageCacheSize + overfillBy
	for i := range totalInserts {
		url := fmt.Sprintf("https://example.com/img%d.png", i)
		remoteImageCache.Add(url, "fake-base64-data")
	}

	// cache should not be overfilled beyond it's capped size
	if got := remoteImageCache.Len(); got != remoteImageCacheSize {
		t.Errorf("expected cache size %d, got %d", remoteImageCacheSize, got)
	}

	// old entries should be evicted
	for i := range overfillBy {
		evictedURL := fmt.Sprintf("https://example.com/img%d.png", i)
		if _, ok := remoteImageCache.Get(evictedURL); ok {
			t.Errorf("expected %q to be evicted, but it's still in cache", evictedURL)
		}
	}

	// The most recent entries should still be present.
	for i := overfillBy; i < totalInserts; i++ {
		keptURL := fmt.Sprintf("https://example.com/img%d.png", i)
		if _, ok := remoteImageCache.Get(keptURL); !ok {
			t.Errorf("expected %q to still be in cache", keptURL)
		}
	}
}

func TestAllocImageID_NoRace(t *testing.T) {
	// Reset the counter so IDs start from a known value.
	atomic.StoreUint32(&nextImageID, 1000)

	const goroutines = 100
	const idsPerGoroutine = 100

	results := make(chan uint32, goroutines*idsPerGoroutine)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range idsPerGoroutine {
				results <- allocImageID()
			}
		}()
	}

	// Close channel once all writers are done.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all IDs and verify uniqueness.
	seen := make(map[uint32]bool, goroutines*idsPerGoroutine)
	for id := range results {
		if seen[id] {
			t.Fatalf("duplicate image ID allocated: %d (race condition detected)", id)
		}
		seen[id] = true
	}

	expected := uint32(goroutines * idsPerGoroutine)
	if uint32(len(seen)) != expected {
		t.Errorf("expected %d unique IDs, got %d", expected, len(seen))
	}
}
