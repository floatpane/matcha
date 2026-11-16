package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestProtocolComboboxCycles(t *testing.T) {
	m := NewLogin(true)

	// Starts focused on the protocol combobox with the default selection.
	if got := m.protocol(); got != "imap" {
		t.Fatalf("initial protocol = %q, want imap", got)
	}

	right := tea.KeyPressMsg{Code: tea.KeyRight}
	want := []string{"jmap", "pop3", "maildir", "imap"} // wraps around
	for _, w := range want {
		model, _ := m.Update(right)
		m = model.(*Login)
		if got := m.protocol(); got != w {
			t.Fatalf("after right, protocol = %q, want %q", got, w)
		}
	}

	// Left cycles backwards, wrapping from imap to maildir.
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = model.(*Login)
	if got := m.protocol(); got != "maildir" {
		t.Fatalf("after left, protocol = %q, want maildir", got)
	}
}

func TestProtocolComboboxIgnoresTyping(t *testing.T) {
	m := NewLogin(true)
	// Focused on the protocol field; typed characters must not edit it.
	model, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = model.(*Login)
	if got := m.protocol(); got != "imap" {
		t.Fatalf("after typing, protocol = %q, want imap (unchanged)", got)
	}
}

func TestValidPort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback int
		want     int
	}{
		{"empty string returns fallback", "", 993, 993},
		{"valid port 993", "993", 143, 993},
		{"valid port 1 (minimum)", "1", 993, 1},
		{"valid port 65535 (maximum)", "65535", 993, 65535},
		{"valid port 587", "587", 25, 587},
		{"valid port 995", "995", 110, 995},
		{"port 0 is invalid, returns fallback", "0", 993, 993},
		{"negative port is invalid", "-1", 993, 993},
		{"port over 65535 is invalid", "70000", 993, 993},
		{"non-numeric returns fallback", "abc", 993, 993},
		{"port with spaces returns fallback", " 993 ", 993, 993},
		{"port 8080", "8080", 993, 8080},
		{"large negative number", "-99999", 993, 993},
		{"very large number", "9999999", 993, 993},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validPort(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("validPort(%q, %d) = %d, want %d", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestValidPortDifferentFallbacks(t *testing.T) {
	if got := validPort("", 143); got != 143 {
		t.Errorf("empty with fallback 143 = %d, want 143", got)
	}
	if got := validPort("", 587); got != 587 {
		t.Errorf("empty with fallback 587 = %d, want 587", got)
	}
	if got := validPort("", 995); got != 995 {
		t.Errorf("empty with fallback 995 = %d, want 995", got)
	}
	if got := validPort("bad", 25); got != 25 {
		t.Errorf("invalid with fallback 25 = %d, want 25", got)
	}
}

func TestSubmitFormPortValidation(t *testing.T) {
	m := NewLogin(true)

	tests := []struct {
		name     string
		portVal  string
		want     int
		fallback int
	}{
		{"valid custom port 143", "143", 143, 993},
		{"invalid port 0 falls back", "0", 993, 993},
		{"invalid negative port falls back", "-1", 993, 993},
		{"invalid overflow port falls back", "70000", 993, 993},
		{"non-numeric port falls back", "abc", 993, 993},
		{"empty port uses default", "", 993, 993},
		{"boundary port 1", "1", 1, 993},
		{"boundary port 65535", "65535", 65535, 993},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.inputs[inputIMAPPort].SetValue(tt.portVal)
			fn := m.submitForm()
			msg := fn().(Credentials)
			if msg.IMAPPort != tt.want {
				t.Errorf("IMAPPort for input %q = %d, want %d", tt.portVal, msg.IMAPPort, tt.want)
			}
		})
	}
}

// Regression: a terminal narrower than 6 columns gave the inputs a negative
// width and bubbles' textinput panicked rendering its placeholder.
func TestLoginTinyWindowNoPanic(t *testing.T) {
	for w := 0; w <= 7; w++ {
		m := NewLogin(false)
		m.Update(tea.WindowSizeMsg{Width: w, Height: 3})
		_ = m.View()
	}
}

// Regression: "oauth2" typed for Gmail kept hiding the password field (and
// was submitted) after switching the provider to a custom server.
func TestLoginStaleOAuthDoesNotHidePassword(t *testing.T) {
	m := NewLogin(true)
	m.inputs[inputProvider].SetValue("gmail")
	m.inputs[inputAuthMethod].SetValue("oauth2")
	m.updateFlags()
	if !m.useOAuth2 {
		t.Fatal("gmail + oauth2 should use OAuth2")
	}

	m.inputs[inputProvider].SetValue("custom")
	m.updateFlags()
	if m.useOAuth2 {
		t.Fatal("custom provider must not use OAuth2")
	}
	if !slices.Contains(m.visibleFields(), inputPassword) {
		t.Fatal("password field hidden for a custom provider")
	}
	if got := m.submitForm()().(Credentials).AuthMethod; got != "password" {
		t.Fatalf("AuthMethod = %q, want password", got)
	}
}

// The form scrolls to keep the focused field visible and never renders
// wider or taller than the window.
func TestLoginViewFitsWindow(t *testing.T) {
	for _, proto := range loginProtocols {
		for _, sz := range [][2]int{{80, 24}, {60, 20}, {40, 12}} {
			m := NewLogin(false)
			m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			m.inputs[inputProtocol].SetValue(proto)
			m.inputs[inputProvider].SetValue("custom")
			m.updateFlags()
			for _, f := range m.visibleFields() {
				m.focusIndex = f
				view := m.View().Content
				name := fmt.Sprintf("%s focus=%d @%dx%d", proto, f, sz[0], sz[1])
				assertViewFits(t, name, view, sz[0], sz[1])
				if f != inputProtocol && !strings.Contains(view, m.inputs[f].View()) {
					t.Errorf("%s: focused field scrolled out of view", name)
				}
			}
		}
	}
}

// OAuth progress (including the long authorization URL) wraps to the window
// and stays on screen while the flow runs.
func TestLoginOAuthStatusWraps(t *testing.T) {
	const url = "https://accounts.google.com/o/oauth2/auth?client_id=1234567890-abcdefghijklmnopqrstuvwxyz.apps.googleusercontent.com&redirect_uri=http%3A%2F%2Flocalhost%3A8080"
	m := NewLogin(false)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m.inputs[inputProvider].SetValue("gmail")
	m.inputs[inputAuthMethod].SetValue("oauth2")
	m.updateFlags()
	m.AddOAuthStatus("If the browser doesn't open, visit this URL:")
	m.AddOAuthStatus("  " + url)
	view := m.View().Content
	assertViewFits(t, "oauth status", view, 60, 20)
	flat := strings.ReplaceAll(ansi.Strip(view), "\n", "")
	if !strings.Contains(flat, url) {
		t.Fatalf("authorization URL not fully shown:\n%s", ansi.Strip(view))
	}
}
