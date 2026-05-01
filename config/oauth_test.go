package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

func TestPKCES256_RFC7636Vector(t *testing.T) {
	// RFC 7636 Appendix B test vector.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := pkceS256(verifier); got != want {
		t.Errorf("pkceS256 = %q, want %q", got, want)
	}
}

func TestRandomURLSafe_LengthAndCharset(t *testing.T) {
	s, err := randomURLSafe(32)
	if err != nil {
		t.Fatal(err)
	}
	// base64url(32 bytes) with no padding = 43 chars.
	if len(s) != 43 {
		t.Errorf("length = %d, want 43", len(s))
	}
	for _, r := range s {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			t.Errorf("non-url-safe character %q in %q", r, s)
		}
	}
	// Two calls should not collide.
	s2, _ := randomURLSafe(32)
	if s == s2 {
		t.Error("randomURLSafe returned identical values across calls")
	}
}

func TestDetectProvider(t *testing.T) {
	cases := []struct {
		email  string
		want   string
		wantOK bool
	}{
		{"alice@gmail.com", "gmail", true},
		{"ALICE@GMAIL.COM", "gmail", true},
		{"alice@googlemail.com", "gmail", true},
		{"bob@outlook.com", "outlook", true},
		{"bob@hotmail.com", "outlook", true},
		{"bob@live.com", "outlook", true},
		{"bob@msn.com", "outlook", true},
		{"someone@example.com", "", false},
		{"no-at-sign", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := detectProvider(c.email)
		if got != c.want || ok != c.wantOK {
			t.Errorf("detectProvider(%q) = (%q, %v), want (%q, %v)", c.email, got, ok, c.want, c.wantOK)
		}
	}
}

func TestResolveProvider(t *testing.T) {
	if p, err := resolveProvider("alice@gmail.com", ""); err != nil || p.Key != "gmail" {
		t.Errorf("auto-detect gmail: %v / %+v", err, p)
	}
	if p, err := resolveProvider("alice@example.com", "outlook"); err != nil || p.Key != "outlook" {
		t.Errorf("explicit outlook: %v / %+v", err, p)
	}
	if p, err := resolveProvider("alice@example.com", "OUTLOOK"); err != nil || p.Key != "outlook" {
		t.Errorf("uppercase provider should be normalized: %v / %+v", err, p)
	}
	if _, err := resolveProvider("alice@example.com", ""); err == nil {
		t.Error("expected error when provider cannot be auto-detected")
	}
	if _, err := resolveProvider("alice@example.com", "yahoo"); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestDecodeToken_NativeShape(t *testing.T) {
	original := &oauth2.Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour).UTC().Truncate(time.Second),
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeToken(data)
	if err != nil {
		t.Fatalf("decodeToken: %v", err)
	}
	if got.AccessToken != original.AccessToken || got.RefreshToken != original.RefreshToken {
		t.Errorf("token round-trip mismatch: got %+v want %+v", got, original)
	}
}

func TestDecodeToken_LegacyPythonShape(t *testing.T) {
	expiry := time.Now().Add(2 * time.Hour).Unix()
	legacy := []byte(`{
		"access_token": "legacy-access",
		"refresh_token": "legacy-refresh",
		"token_type": "",
		"expires_at": ` + itoa(expiry) + `
	}`)
	got, err := decodeToken(legacy)
	if err != nil {
		t.Fatalf("decodeToken legacy: %v", err)
	}
	if got.AccessToken != "legacy-access" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "legacy-access")
	}
	if got.RefreshToken != "legacy-refresh" {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, "legacy-refresh")
	}
	if got.TokenType != "Bearer" {
		t.Errorf("default TokenType = %q, want %q", got.TokenType, "Bearer")
	}
	if got.Expiry.Unix() != expiry {
		t.Errorf("Expiry = %d, want %d", got.Expiry.Unix(), expiry)
	}
}

func TestDecodeToken_Malformed(t *testing.T) {
	if _, err := decodeToken([]byte("not json")); err == nil {
		t.Error("expected error for non-JSON input")
	}
	if _, err := decodeToken([]byte(`{"refresh_token":"x"}`)); err == nil {
		t.Error("expected error when access_token is missing")
	}
}

// setHomeDir redirects os.UserHomeDir to a temp directory on every supported
// OS. Linux/macOS read $HOME; Windows reads %USERPROFILE%. Setting both is
// harmless on the platform that doesn't honor it.
func setHomeDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestSaveAndLoadToken_KeyringRoundTrip(t *testing.T) {
	keyring.MockInit()
	setHomeDir(t, t.TempDir())

	email := "user@gmail.com"
	tok := &oauth2.Token{
		AccessToken:  "AT",
		RefreshToken: "RT",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour).UTC().Truncate(time.Second),
	}
	if err := saveToken(email, tok); err != nil {
		t.Fatalf("saveToken: %v", err)
	}
	got, err := loadToken(email)
	if err != nil {
		t.Fatalf("loadToken: %v", err)
	}
	if got.AccessToken != tok.AccessToken || got.RefreshToken != tok.RefreshToken {
		t.Errorf("loadToken returned %+v, want %+v", got, tok)
	}
	if err := deleteToken(email); err != nil {
		t.Fatalf("deleteToken: %v", err)
	}
	if _, err := loadToken(email); err == nil {
		t.Error("loadToken should fail after deleteToken")
	}
}

func TestLoadToken_MigratesFromDisk(t *testing.T) {
	keyring.MockInit()
	home := t.TempDir()
	setHomeDir(t, home)

	email := "migrate@gmail.com"
	dir := filepath.Join(home, ".config", "matcha", "oauth_tokens")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	legacyJSON := []byte(`{"access_token":"legacy-AT","refresh_token":"legacy-RT","expires_at":` + itoa(time.Now().Add(time.Hour).Unix()) + `}`)
	path := filepath.Join(dir, email+".json")
	if err := os.WriteFile(path, legacyJSON, 0600); err != nil {
		t.Fatal(err)
	}

	got, err := loadToken(email)
	if err != nil {
		t.Fatalf("loadToken: %v", err)
	}
	if got.AccessToken != "legacy-AT" || got.RefreshToken != "legacy-RT" {
		t.Errorf("migrated token = %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("legacy file should be removed after migration, stat err = %v", err)
	}
	// Subsequent load must come from the keyring.
	got2, err := loadToken(email)
	if err != nil {
		t.Fatalf("second loadToken: %v", err)
	}
	if got2.AccessToken != "legacy-AT" {
		t.Errorf("post-migration load returned %+v", got2)
	}
}

func TestSaveAndLoadClientCredentials(t *testing.T) {
	keyring.MockInit()
	setHomeDir(t, t.TempDir())

	if err := saveClientCredentials("gmail", "client-id-1", "client-secret-1"); err != nil {
		t.Fatalf("saveClientCredentials: %v", err)
	}
	id, secret, err := loadClientCredentials("gmail")
	if err != nil {
		t.Fatalf("loadClientCredentials: %v", err)
	}
	if id != "client-id-1" || secret != "client-secret-1" {
		t.Errorf("got (%q, %q)", id, secret)
	}
	if _, _, err := loadClientCredentials("missing-provider"); err == nil {
		t.Error("expected error for missing provider")
	}
}

func TestLoadClientCredentials_MigratesFromDisk(t *testing.T) {
	keyring.MockInit()
	home := t.TempDir()
	setHomeDir(t, home)

	dir := filepath.Join(home, ".config", "matcha", "oauth")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"client_id":"on-disk-id","client_secret":"on-disk-secret"}`)
	if err := os.WriteFile(filepath.Join(dir, "gmail_client.json"), legacy, 0600); err != nil {
		t.Fatal(err)
	}

	id, secret, err := loadClientCredentials("gmail")
	if err != nil {
		t.Fatalf("loadClientCredentials from disk: %v", err)
	}
	if id != "on-disk-id" || secret != "on-disk-secret" {
		t.Errorf("got (%q, %q)", id, secret)
	}
}

func TestLegacyTokenPath_Shape(t *testing.T) {
	home := t.TempDir()
	setHomeDir(t, home)
	got := legacyTokenPath("user@gmail.com")
	want := filepath.Join(home, ".config", "matcha", "oauth_tokens", "user@gmail.com.json")
	if got != want {
		t.Errorf("legacyTokenPath = %q, want %q", got, want)
	}
}

func TestProviderHelpText(t *testing.T) {
	p := providers["gmail"]
	help := p.helpText()
	if !strings.Contains(help, "console.cloud.google.com") {
		t.Errorf("gmail help text missing setup URL: %q", help)
	}
}

func TestLoadClientForAccount_PerEmailOverridesProviderDefault(t *testing.T) {
	keyring.MockInit()
	setHomeDir(t, t.TempDir())

	// Provider-level default (used by accounts that don't override).
	if err := saveClientCredentials("gmail", "default-id", "default-secret"); err != nil {
		t.Fatal(err)
	}
	// Per-email override (e.g. a Workspace account using a different Cloud Project).
	if err := saveClientCredentials("work@corp.example", "work-id", "work-secret"); err != nil {
		t.Fatal(err)
	}

	// An account without an override should fall back to the provider default.
	id, secret, err := loadClientForAccount("personal@gmail.com", "gmail")
	if err != nil {
		t.Fatalf("loadClientForAccount fallback: %v", err)
	}
	if id != "default-id" || secret != "default-secret" {
		t.Errorf("fallback returned (%q, %q), want default-id/default-secret", id, secret)
	}

	// An account with an override should get its own credentials.
	id, secret, err = loadClientForAccount("work@corp.example", "gmail")
	if err != nil {
		t.Fatalf("loadClientForAccount override: %v", err)
	}
	if id != "work-id" || secret != "work-secret" {
		t.Errorf("override returned (%q, %q), want work-id/work-secret", id, secret)
	}
}

func TestDeleteToken_RemovesPerEmailClientCreds(t *testing.T) {
	keyring.MockInit()
	setHomeDir(t, t.TempDir())

	// Set both a provider-level default and a per-email override.
	if err := saveClientCredentials("gmail", "default-id", "default-secret"); err != nil {
		t.Fatal(err)
	}
	if err := saveClientCredentials("user@gmail.com", "override-id", "override-secret"); err != nil {
		t.Fatal(err)
	}

	if err := deleteToken("user@gmail.com"); err != nil {
		t.Fatalf("deleteToken: %v", err)
	}

	// Per-email override should be gone.
	if _, _, err := loadClientCredentials("user@gmail.com"); err == nil {
		t.Error("expected per-email override to be deleted")
	}
	// Provider-level default must remain — other accounts may still rely on it.
	id, secret, err := loadClientCredentials("gmail")
	if err != nil {
		t.Fatalf("provider default should survive: %v", err)
	}
	if id != "default-id" || secret != "default-secret" {
		t.Errorf("provider default mutated to (%q, %q)", id, secret)
	}
}

func TestProviderForToken_KeyringMappingForCustomDomain(t *testing.T) {
	keyring.MockInit()
	setHomeDir(t, t.TempDir())

	// Workspace-style domain — not auto-detectable; should fail before mapping.
	if _, err := providerForToken("user@corp.example"); err == nil {
		t.Error("expected error before mapping is stored")
	}

	if err := keyring.Set(keyringServiceName, "user@corp.example"+providerKeySuffix, "gmail"); err != nil {
		t.Fatal(err)
	}
	p, err := providerForToken("user@corp.example")
	if err != nil {
		t.Fatalf("after mapping stored: %v", err)
	}
	if p.Key != "gmail" {
		t.Errorf("got provider %q, want gmail", p.Key)
	}
}

// TestProviderForToken_KeyringPrecedence verifies the keyring mapping is
// consulted *before* domain detection by using an @gmail.com address that
// would otherwise auto-detect to gmail, and storing "outlook" in the keyring.
// If the keyring entry isn't actually consulted first, the test catches it.
func TestProviderForToken_KeyringPrecedence(t *testing.T) {
	keyring.MockInit()
	setHomeDir(t, t.TempDir())

	if err := keyring.Set(keyringServiceName, "user@gmail.com"+providerKeySuffix, "outlook"); err != nil {
		t.Fatal(err)
	}
	p, err := providerForToken("user@gmail.com")
	if err != nil {
		t.Fatalf("providerForToken: %v", err)
	}
	if p.Key != "outlook" {
		t.Errorf("got provider %q, want outlook (keyring should win over domain detection)", p.Key)
	}
}

func TestProviderForToken_FallsBackToDomainDetection(t *testing.T) {
	keyring.MockInit()
	setHomeDir(t, t.TempDir())

	// No keyring mapping — should still resolve via domain heuristic.
	p, err := providerForToken("legacy.user@gmail.com")
	if err != nil {
		t.Fatalf("domain fallback failed: %v", err)
	}
	if p.Key != "gmail" {
		t.Errorf("got provider %q, want gmail", p.Key)
	}
}

func TestRevokeOAuth2Token_NoStoredTokenIsNoop(t *testing.T) {
	keyring.MockInit()
	setHomeDir(t, t.TempDir())
	// Should not error when no token is stored.
	if err := RevokeOAuth2Token("nobody@gmail.com"); err != nil {
		t.Errorf("RevokeOAuth2Token on empty state should be a no-op, got %v", err)
	}
}

// itoa is a tiny helper to keep the test file dependency-free.
func itoa(n int64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
