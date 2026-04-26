package httpclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTimeoutConstants(t *testing.T) {
	cases := []struct {
		name string
		got  time.Duration
		min  time.Duration
	}{
		{"PluginCallTimeout", PluginCallTimeout, time.Second},
		{"RegistryFetchTimeout", RegistryFetchTimeout, time.Second},
		{"RemoteImageTimeout", RemoteImageTimeout, time.Second},
		{"InstallTimeout", InstallTimeout, time.Second},
		{"UpdateCheckTimeout", UpdateCheckTimeout, time.Second},
	}
	for _, c := range cases {
		if c.got < c.min {
			t.Errorf("%s = %s, want at least %s", c.name, c.got, c.min)
		}
	}
}

func TestNew_AppliesTimeout(t *testing.T) {
	c := New(7 * time.Second)
	if c.Timeout != 7*time.Second {
		t.Errorf("New(7s).Timeout = %s, want 7s", c.Timeout)
	}
}

func TestNewWithRedirectCap_AppliesTimeoutAndRedirects(t *testing.T) {
	c := NewWithRedirectCap(11*time.Second, 3)
	if c.Timeout != 11*time.Second {
		t.Errorf("Timeout = %s, want 11s", c.Timeout)
	}
	if c.CheckRedirect == nil {
		t.Fatal("CheckRedirect is nil; want a redirect-cap function")
	}

	// Build a stubbed redirect chain and verify the cap fires at the
	// configured maxRedirects.
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	via := []*http.Request{}
	for i := 0; i < 3; i++ {
		if err := c.CheckRedirect(req, via); err != nil {
			t.Fatalf("CheckRedirect rejected %d-redirect chain: %v", i, err)
		}
		via = append(via, req)
	}
	if err := c.CheckRedirect(req, via); err == nil {
		t.Error("CheckRedirect(via len=3) returned nil; want stopped error")
	} else if !strings.Contains(err.Error(), "stopped after 3 redirects") {
		t.Errorf("CheckRedirect error = %q, want 'stopped after 3 redirects' substring", err.Error())
	}
}

// TestNewWithRedirectCap_LiveServer is a defense-in-depth integration check
// that the redirect cap is actually honored by net/http when wired up. It
// uses an in-process server so it stays hermetic.
func TestNewWithRedirectCap_LiveServer(t *testing.T) {
	hops := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer server.Close()

	c := NewWithRedirectCap(2*time.Second, 2)
	resp, err := c.Get(server.URL + "/start")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected redirect-cap error, got nil")
	}
	if !strings.Contains(err.Error(), "stopped after 2 redirects") {
		t.Errorf("redirect error = %v, want substring 'stopped after 2 redirects'", err)
	}
}
