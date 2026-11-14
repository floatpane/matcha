package plugin

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/floatpane/matcha/internal/httpclient"
)

const httpMaxBodySize = 1 << 20 // 1 MB

var httpClient = httpclient.New(httpclient.PluginCallTimeout)

// Hooks run synchronously on the UI goroutine, and some fire once per fetched
// email (email_received), so matcha.http calls made from hooks share a time
// budget: at most hookHTTPBudget of request time per hookHTTPWindow. Once it
// is spent, calls from hooks fail immediately instead of freezing the UI.
// Calls from key bindings and prompts (explicit user actions) are exempt.
const (
	hookHTTPBudget = 2 * time.Second
	hookHTTPWindow = 10 * time.Second
)

type httpBudget struct {
	windowStart time.Time
	spent       time.Duration
}

// remaining returns the budget left in the current window.
func (b *httpBudget) remaining(now time.Time) time.Duration {
	if now.Sub(b.windowStart) >= hookHTTPWindow {
		b.windowStart = now
		b.spent = 0
	}
	return hookHTTPBudget - b.spent
}

func (b *httpBudget) charge(d time.Duration) { b.spent += d }

// luaHTTP implements matcha.http(options) — make an HTTP request.
//
// options is a table with fields:
//   - url     (string, required)
//   - method  (string, optional, default "GET")
//   - headers (table, optional)
//   - body    (string, optional)
//
// Returns (response_table, nil) on success or (nil, error_string) on failure.
// response_table has fields: status (number), body (string), headers (table).
func (m *Manager) luaHTTP(L *lua.LState) int { //nolint:gocritic
	opts := L.CheckTable(1)

	// URL (required).
	urlVal := opts.RawGetString("url")
	if urlVal == lua.LNil {
		L.Push(lua.LNil)
		L.Push(lua.LString("missing required field: url"))
		return 2
	}
	rawURL := urlVal.String()

	// URL format validation.
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("invalid URL: " + err.Error()))
		return 2
	}

	// Scheme validation.
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		L.Push(lua.LNil)
		L.Push(lua.LString("unsupported URL scheme: only http and https are allowed"))
		return 2
	}

	// Method (optional, default GET).
	method := "GET"
	if v := opts.RawGetString("method"); v != lua.LNil {
		method = strings.ToUpper(v.String())
	}

	// Body (optional).
	var bodyReader io.Reader
	if v := opts.RawGetString("body"); v != lua.LNil {
		bodyReader = strings.NewReader(v.String())
	}

	timeout := httpclient.PluginCallTimeout
	inHook := m.hookDepth > 0
	if inHook {
		left := m.httpBudget.remaining(time.Now())
		if left <= 0 {
			L.Push(lua.LNil)
			L.Push(lua.LString("matcha.http: time budget for hooks exhausted (hooks run on the UI thread; use a key binding for slow requests)"))
			return 2
		}
		timeout = min(timeout, left)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if inHook {
		start := time.Now()
		defer func() { m.httpBudget.charge(time.Since(start)) }()
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Headers (optional).
	if v := opts.RawGetString("headers"); v != lua.LNil {
		if tbl, ok := v.(*lua.LTable); ok {
			tbl.ForEach(func(k, v lua.LValue) {
				req.Header.Set(k.String(), v.String())
			})
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(io.LimitReader(resp.Body, httpMaxBodySize))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Build response table.
	result := L.NewTable()
	result.RawSetString("status", lua.LNumber(resp.StatusCode))
	result.RawSetString("body", lua.LString(string(body)))

	headers := L.NewTable()
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headers.RawSetString(strings.ToLower(k), lua.LString(vals[0]))
		}
	}
	result.RawSetString("headers", headers)

	L.Push(result)
	L.Push(lua.LNil)
	return 2
}
