package tui

import (
	"testing"
)

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
