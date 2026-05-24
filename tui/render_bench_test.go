package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

func BenchmarkLogPanelView(b *testing.B) {
	logger := &snapshotLogger{}
	for i := 0; i < 10; i++ {
		logger.Write([]byte("benchmark log line " + strings.Repeat("x", 40) + "\n"))
	}
	panel := NewLogPanel(logger)
	panel.SetSize(80, 12)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = panel.View()
	}
}

func BenchmarkSearchOverlayView(b *testing.B) {
	overlay := NewSearchOverlay(80, 24)
	emails := make([]fetcher.Email, 10)
	for i := range emails {
		emails[i] = fetcher.Email{
			UID:     uint32(i),
			From:    "sender@example.com",
			Subject: "Benchmark email subject",
			Date:    time.Now(),
		}
	}
	overlay.results = emails
	overlay.done = true

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = overlay.View()
	}
}

func BenchmarkInboxConstruction(b *testing.B) {
	accounts := []config.Account{{ID: "a", Email: "a@example.com"}}
	emails := make([]fetcher.Email, 500)
	for i := range emails {
		emails[i] = fetcher.Email{
			UID:       uint32(i),
			From:      "bench@example.com",
			Subject:   "Subject line " + strings.Repeat("y", 20),
			Date:      time.Now().Add(-time.Duration(i) * time.Minute),
			AccountID: "a",
		}
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewInbox(emails, accounts)
	}
}
