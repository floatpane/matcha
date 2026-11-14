package plugin

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// captureStdout runs fn and returns what it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// Lua's print used to write straight to os.Stdout, corrupting the TUI.
func TestLuaPrintGoesToLogNotStdout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewManager()
	defer m.Close()

	var logs bytes.Buffer
	origOut, origFlags := log.Writer(), log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() { log.SetOutput(origOut); log.SetFlags(origFlags) }()

	m.currentPlugin = "greeter"
	stdout := captureStdout(t, func() {
		if err := m.state.DoString(`print("hello", 42)`); err != nil {
			t.Fatal(err)
		}
	})
	if stdout != "" {
		t.Fatalf("print wrote to stdout: %q", stdout)
	}
	if got := logs.String(); !strings.Contains(got, "[plugin greeter] hello\t42") {
		t.Fatalf("log = %q", got)
	}
}

// dofile()/loadfile() without a name read os.Stdin, stealing TUI input.
func TestDofileLoadfileRequireName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewManager()
	defer m.Close()

	if err := m.state.DoString(`dofile()`); err == nil {
		t.Fatal("dofile() without a name should fail")
	}
	if err := m.state.DoString(`local f, err = loadfile(); assert(f == nil)`); err == nil {
		t.Fatal("loadfile() without a name should fail")
	}

	dir := t.TempDir()
	path := dir + "/mod.lua"
	if err := os.WriteFile(path, []byte(`return 1 + 2`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `local v = dofile("` + path + `"); assert(v == 3)
local f = assert(loadfile("` + path + `")); assert(f() == 3)`
	if err := m.state.DoString(script); err != nil {
		t.Fatalf("dofile/loadfile with a name: %v", err)
	}
}

// email_received fires on the UI goroutine (once per fetched email); a hook
// doing matcha.http must not be able to block it for long.
func TestHookHTTPIsBounded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	m := NewManager()
	defer m.Close()
	m.currentPlugin = "webhook"
	if err := m.state.DoString(`local matcha = require("matcha")
failures = 0
matcha.on("email_received", function(email)
  local res, err = matcha.http({url = "` + srv.URL + `", method = "POST", body = "{}"})
  if err then failures = failures + 1 end
end)`); err != nil {
		t.Fatal(err)
	}
	m.currentPlugin = ""

	start := time.Now()
	for i := 0; i < 50; i++ {
		m.CallHook(HookEmailReceived, m.EmailToTable(uint32(i), "a@b", nil, "s", time.Now(), false, "acct", "INBOX"))
	}
	elapsed := time.Since(start)
	if elapsed > hookHTTPBudget+time.Second {
		t.Fatalf("50 hook calls blocked the caller for %v (budget %v)", elapsed, hookHTTPBudget)
	}
	if n := m.state.GetGlobal("failures").String(); n == "0" {
		t.Fatal("expected calls beyond the budget to fail fast")
	}
}

// Key bindings are explicit user actions and keep the full timeout.
func TestKeyBindingHTTPNotBudgeted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	m := NewManager()
	defer m.Close()
	m.httpBudget.windowStart = time.Now()
	m.httpBudget.spent = hookHTTPBudget // exhausted
	m.currentPlugin = "ai"
	if err := m.state.DoString(`local matcha = require("matcha")
ok = false
matcha.bind_key("ctrl+r", "composer", "x", function()
  local res, err = matcha.http({url = "` + srv.URL + `"})
  ok = (err == nil and res.status == 200)
end)`); err != nil {
		t.Fatal(err)
	}
	m.CallKeyBinding(m.Bindings("composer")[0])
	if m.state.GetGlobal("ok").String() != "true" {
		t.Fatal("key binding request should not be limited by the hook budget")
	}
}

// Runtime hook errors become (rate-limited) notifications instead of only
// being logged to stderr on every keystroke.
func TestHookErrorsBecomeRateLimitedNotifications(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewManager()
	defer m.Close()
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	m.currentPlugin = "broken"
	if err := m.state.DoString(`local matcha = require("matcha")
matcha.on("composer_updated", function(state) error("boom") end)`); err != nil {
		t.Fatal(err)
	}
	m.currentPlugin = ""
	for i := 0; i < 20; i++ {
		m.CallComposerHook(HookComposerUpdated, "b", "s", "t", "", "")
	}
	n, ok := m.TakePendingNotification()
	if !ok || n.Kind != NotifyKindError || !strings.Contains(n.Message, "broken") || !strings.Contains(n.Message, "boom") {
		t.Fatalf("notification = %+v, %v", n, ok)
	}
	if extra, ok := m.TakePendingNotification(); ok {
		t.Fatalf("expected one notification per interval, got another: %+v", extra)
	}
}
