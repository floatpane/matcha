package plugin

import (
	"io"
	"net/http"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const (
	httpTimeout     = 10 * time.Second
	httpMaxBodySize = 1 << 20 // 1 MB
)

var httpClient = &http.Client{
	Timeout: httpTimeout,
}

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
func (m *Manager) luaHTTP(L *lua.LState) int {
	opts := L.CheckTable(1)

	// URL (required).
	urlVal := opts.RawGetString("url")
	if urlVal == lua.LNil {
		L.Push(lua.LNil)
		L.Push(lua.LString("missing required field: url"))
		return 2
	}
	rawURL := urlVal.String()

	// Scheme validation.
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
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

	req, err := http.NewRequest(method, rawURL, bodyReader)
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
	defer resp.Body.Close()

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
