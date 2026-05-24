//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/smtp"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/floatpane/matcha/backend"
	_ "github.com/floatpane/matcha/backend/imap"
	"github.com/floatpane/matcha/config"
)

// testEnv resolves the integration test environment. Greenmail exposes the
// following ports by default — we read them from env to allow remapping:
//
//	MATCHA_TEST_IMAP_HOST       default: 127.0.0.1
//	MATCHA_TEST_IMAP_PORT       default: 3993 (implicit TLS)
//	MATCHA_TEST_SMTP_PORT       default: 3465 (implicit TLS, used by matcha sender)
//	MATCHA_TEST_SMTP_PLAIN_PORT default: 3025 (plain SMTP, used by deliverViaSMTP)
//	MATCHA_TEST_API_PORT        default: 8080 (Greenmail REST API)
type testEnv struct {
	host          string
	imapPort      int
	smtpPort      int
	smtpPlainPort int
	apiPort       int
}

func loadEnv(t *testing.T) testEnv {
	t.Helper()
	env := testEnv{
		host:          getenv("MATCHA_TEST_IMAP_HOST", "127.0.0.1"),
		imapPort:      getenvInt(t, "MATCHA_TEST_IMAP_PORT", 3993),
		smtpPort:      getenvInt(t, "MATCHA_TEST_SMTP_PORT", 3465),
		smtpPlainPort: getenvInt(t, "MATCHA_TEST_SMTP_PLAIN_PORT", 3025),
		apiPort:       getenvInt(t, "MATCHA_TEST_API_PORT", 8080),
	}
	return env
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(t *testing.T, key string, fallback int) int {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("invalid %s: %v", key, err)
	}
	return n
}

func waitForGreenmail(t *testing.T, env testEnv) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	url := fmt.Sprintf("http://%s:%d/api/configuration", env.host, env.apiPort)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("greenmail not ready after 60s at %s", url)
}

func resetGreenmail(t *testing.T, env testEnv) {
	t.Helper()
	url := fmt.Sprintf("http://%s:%d/api/mail/purge", env.host, env.apiPort)
	req, _ := http.NewRequest(http.MethodPost, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reset greenmail: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		t.Fatalf("reset greenmail: status %d", resp.StatusCode)
	}
}

// deliverViaSMTP injects a message into the IMAP store by speaking SMTP to
// Greenmail directly. Greenmail's REST API only supports reading and purging;
// SMTP is the only documented way to inject mail.
func deliverViaSMTP(t *testing.T, env testEnv, from, to, subject, body string) {
	t.Helper()
	addr := fmt.Sprintf("%s:%d", env.host, env.smtpPlainPort)

	hdr := textproto.MIMEHeader{}
	hdr.Set("From", from)
	hdr.Set("To", to)
	hdr.Set("Subject", subject)
	hdr.Set("Date", time.Now().UTC().Format(time.RFC1123Z))
	hdr.Set("MIME-Version", "1.0")
	hdr.Set("Content-Type", "text/plain; charset=UTF-8")

	var msg strings.Builder
	for k, vs := range hdr {
		for _, v := range vs {
			fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
		}
	}
	msg.WriteString("\r\n")
	msg.WriteString(body)

	if err := smtp.SendMail(addr, nil, from, []string{to}, []byte(msg.String())); err != nil {
		t.Fatalf("deliver via smtp: %v", err)
	}
	// Greenmail delivers asynchronously; wait briefly so the next IMAP read
	// sees the message.
	time.Sleep(300 * time.Millisecond)
}

func newTestAccount(env testEnv, user, pass string) *config.Account {
	return &config.Account{
		ID:              "test-account",
		Name:            "Test User",
		Email:           user,
		Password:        pass,
		ServiceProvider: "custom",
		IMAPServer:      env.host,
		IMAPPort:        env.imapPort,
		SMTPServer:      env.host,
		SMTPPort:        env.smtpPort,
		Insecure:        true,
		Protocol:        "imap",
		SC:              &config.SessionCache{},
	}
}

func TestIntegration_FetchInbox(t *testing.T) {
	env := loadEnv(t)
	waitForGreenmail(t, env)
	resetGreenmail(t, env)

	const user = "alice@example.com"
	const pass = "secret"

	deliverViaSMTP(t, env, "bob@example.com", user, "Hello Alice", "first message")
	deliverViaSMTP(t, env, "carol@example.com", user, "Invoice Q4", "please pay")

	acct := newTestAccount(env, user, pass)
	provider, err := backend.New(acct)
	if err != nil {
		t.Fatalf("backend.New: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	emails, err := provider.FetchEmails(ctx, "INBOX", 50, 0)
	if err != nil {
		t.Fatalf("FetchEmails: %v", err)
	}
	if len(emails) != 2 {
		t.Fatalf("FetchEmails returned %d, want 2", len(emails))
	}

	subjects := map[string]bool{}
	for _, e := range emails {
		subjects[e.Subject] = true
	}
	for _, want := range []string{"Hello Alice", "Invoice Q4"} {
		if !subjects[want] {
			t.Errorf("missing subject %q in %v", want, subjects)
		}
	}
}

func TestIntegration_SearchSubject(t *testing.T) {
	env := loadEnv(t)
	waitForGreenmail(t, env)
	resetGreenmail(t, env)

	const user = "alice@example.com"
	const pass = "secret"

	deliverViaSMTP(t, env, "bob@example.com", user, "Invoice Q4", "")
	deliverViaSMTP(t, env, "bob@example.com", user, "Random update", "")
	deliverViaSMTP(t, env, "bob@example.com", user, "Invoice Q1", "")

	acct := newTestAccount(env, user, pass)
	provider, err := backend.New(acct)
	if err != nil {
		t.Fatalf("backend.New: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	results, err := provider.Search(ctx, "INBOX", backend.SearchQuery{Subject: "Invoice"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Search returned %d, want 2", len(results))
	}
	for _, r := range results {
		if !strings.Contains(r.Subject, "Invoice") {
			t.Errorf("unexpected subject %q in search results", r.Subject)
		}
	}
}

func TestIntegration_MarkAsRead(t *testing.T) {
	env := loadEnv(t)
	waitForGreenmail(t, env)
	resetGreenmail(t, env)

	const user = "alice@example.com"
	const pass = "secret"

	deliverViaSMTP(t, env, "bob@example.com", user, "Toggle me", "")

	acct := newTestAccount(env, user, pass)
	provider, err := backend.New(acct)
	if err != nil {
		t.Fatalf("backend.New: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	emails, err := provider.FetchEmails(ctx, "INBOX", 10, 0)
	if err != nil || len(emails) != 1 {
		t.Fatalf("FetchEmails: err=%v len=%d", err, len(emails))
	}
	uid := emails[0].UID
	if emails[0].IsRead {
		t.Fatal("email unexpectedly marked read before MarkAsRead")
	}

	if err := provider.MarkAsRead(ctx, "INBOX", uid); err != nil {
		t.Fatalf("MarkAsRead: %v", err)
	}

	emails, err = provider.FetchEmails(ctx, "INBOX", 10, 0)
	if err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	if !emails[0].IsRead {
		t.Error("email not marked read after MarkAsRead")
	}
}
