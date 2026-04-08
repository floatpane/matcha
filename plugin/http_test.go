package plugin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// newTestManager creates a Manager with a fresh Lua VM for testing.
func newTestManager() *Manager {
	return NewManager()
}

func TestHTTPGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("X-Test", "hello")
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	m := newTestManager()
	defer m.Close()

	err := m.state.DoString(`
		local matcha = require("matcha")
		res, err = matcha.http({ url = "` + srv.URL + `" })
	`)
	if err != nil {
		t.Fatal(err)
	}

	errVal := m.state.GetGlobal("err")
	if errVal != lua.LNil {
		t.Fatalf("expected nil error, got %v", errVal)
	}

	res := m.state.GetGlobal("res")
	tbl, ok := res.(*lua.LTable)
	if !ok {
		t.Fatalf("expected table, got %T", res)
	}

	if status := tbl.RawGetString("status"); status.(lua.LNumber) != 200 {
		t.Errorf("expected status 200, got %v", status)
	}
	if body := tbl.RawGetString("body"); body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", body.String())
	}

	headers := tbl.RawGetString("headers").(*lua.LTable)
	if v := headers.RawGetString("x-test"); v.String() != "hello" {
		t.Errorf("expected header x-test='hello', got %q", v.String())
	}
}

func TestHTTPPostWithBodyAndHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		w.Write(body)
	}))
	defer srv.Close()

	m := newTestManager()
	defer m.Close()

	err := m.state.DoString(`
		local matcha = require("matcha")
		res, err = matcha.http({
			url = "` + srv.URL + `",
			method = "post",
			headers = { ["Content-Type"] = "application/json" },
			body = '{"key":"value"}',
		})
	`)
	if err != nil {
		t.Fatal(err)
	}

	errVal := m.state.GetGlobal("err")
	if errVal != lua.LNil {
		t.Fatalf("expected nil error, got %v", errVal)
	}

	res := m.state.GetGlobal("res")
	tbl := res.(*lua.LTable)
	if body := tbl.RawGetString("body"); body.String() != `{"key":"value"}` {
		t.Errorf("expected echoed body, got %q", body.String())
	}
}

func TestHTTPMissingURL(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	err := m.state.DoString(`
		local matcha = require("matcha")
		res, err = matcha.http({})
	`)
	if err != nil {
		t.Fatal(err)
	}

	resVal := m.state.GetGlobal("res")
	if resVal != lua.LNil {
		t.Errorf("expected nil result, got %v", resVal)
	}

	errVal := m.state.GetGlobal("err")
	if errVal == lua.LNil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(errVal.String(), "url") {
		t.Errorf("expected error about url, got %q", errVal.String())
	}
}

func TestHTTPInvalidScheme(t *testing.T) {
	m := newTestManager()
	defer m.Close()

	err := m.state.DoString(`
		local matcha = require("matcha")
		res, err = matcha.http({ url = "file:///etc/passwd" })
	`)
	if err != nil {
		t.Fatal(err)
	}

	resVal := m.state.GetGlobal("res")
	if resVal != lua.LNil {
		t.Errorf("expected nil result, got %v", resVal)
	}

	errVal := m.state.GetGlobal("err")
	if errVal == lua.LNil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(errVal.String(), "scheme") {
		t.Errorf("expected error about scheme, got %q", errVal.String())
	}
}

func TestHTTPBodyTruncation(t *testing.T) {
	// Server returns more than 1 MB.
	bigBody := strings.Repeat("x", httpMaxBodySize+1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(bigBody))
	}))
	defer srv.Close()

	m := newTestManager()
	defer m.Close()

	err := m.state.DoString(`
		local matcha = require("matcha")
		res, err = matcha.http({ url = "` + srv.URL + `" })
		body_len = #res.body
	`)
	if err != nil {
		t.Fatal(err)
	}

	bodyLen := m.state.GetGlobal("body_len")
	n, ok := bodyLen.(lua.LNumber)
	if !ok {
		t.Fatalf("expected number, got %T", bodyLen)
	}
	if int(n) > httpMaxBodySize {
		t.Errorf("expected body to be capped at %d, got %d", httpMaxBodySize, int(n))
	}
}
