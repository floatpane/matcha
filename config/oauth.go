// OAuth2 support for Matcha. Uses golang.org/x/oauth2 for the auth code +
// PKCE flow and stores tokens in the OS keyring alongside IMAP passwords.
package config

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// ----- Public API -----

// IsOAuth2 returns true if the account uses OAuth2 authentication.
func (a *Account) IsOAuth2() bool { return a.AuthMethod == "oauth2" }

// GetOAuth2Token returns a valid access token for the given account,
// refreshing transparently if the cached token has expired.
func GetOAuth2Token(email string) (string, error) {
	tok, err := loadToken(email)
	if err != nil {
		return "", fmt.Errorf("oauth2: no stored token for %s (run the OAuth flow first): %w", email, err)
	}

	provider, err := providerForToken(email)
	if err != nil {
		return "", err
	}
	clientID, clientSecret, err := loadClientForAccount(email, provider.Key)
	if err != nil {
		return "", fmt.Errorf("oauth2: missing client credentials for %s: %w", provider.Key, err)
	}

	cfg := provider.oauthConfig(clientID, clientSecret, "") // RedirectURL not needed for refresh
	src := cfg.TokenSource(context.Background(), tok)
	fresh, err := src.Token()
	if err != nil {
		return "", fmt.Errorf("oauth2: token refresh failed: %w", err)
	}
	if fresh.AccessToken != tok.AccessToken {
		// Persist the rotated refresh/access token so we don't re-refresh on every call.
		_ = saveToken(email, fresh)
	}
	return fresh.AccessToken, nil
}

// OAuth2Options tunes RunOAuth2FlowWithOptions. Zero value writes status to os.Stderr.
type OAuth2Options struct {
	// StatusWriter receives progress messages. nil = os.Stderr; pass io.Discard
	// from a TUI to avoid corrupting the alternate-screen grid.
	StatusWriter io.Writer
}

// RunOAuth2Flow performs browser-based authorization for email and writes
// status to os.Stderr. providerKey ("gmail"|"outlook") is auto-detected when
// empty. Non-empty clientID/clientSecret are persisted for future refreshes.
func RunOAuth2Flow(email, providerKey, clientID, clientSecret string) error {
	return RunOAuth2FlowWithOptions(email, providerKey, clientID, clientSecret, OAuth2Options{})
}

// RunOAuth2FlowWithOptions is RunOAuth2Flow with caller-controlled status output.
func RunOAuth2FlowWithOptions(email, providerKey, clientID, clientSecret string, opts OAuth2Options) error {
	statusW := opts.StatusWriter
	if statusW == nil {
		statusW = os.Stderr
	}
	provider, err := resolveProvider(email, providerKey)
	if err != nil {
		return err
	}

	if clientID != "" && clientSecret != "" {
		// Always store per-email so accounts can use distinct OAuth clients.
		if err := saveClientCredentials(email, clientID, clientSecret); err != nil {
			return fmt.Errorf("oauth2: could not store client credentials: %w", err)
		}
		// Seed the provider default on first auth so other accounts can omit
		// the flags. Existing defaults are not overwritten — that's the job
		// of an explicit per-email override.
		if _, _, err := loadClientCredentials(provider.Key); err != nil {
			_ = saveClientCredentials(provider.Key, clientID, clientSecret)
		}
	} else {
		clientID, clientSecret, err = loadClientForAccount(email, provider.Key)
		if err != nil {
			return fmt.Errorf("oauth2: no client credentials configured for %s.\n%s", provider.Name, provider.helpText())
		}
	}

	// Bind a localhost listener on a random free port; we'll set RedirectURL to match.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("oauth2: could not start callback listener: %w", err)
	}
	defer ln.Close()

	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)
	cfg := provider.oauthConfig(clientID, clientSecret, redirectURL)

	state, err := randomURLSafe(24)
	if err != nil {
		return err
	}
	verifier, err := randomURLSafe(64) // PKCE code_verifier
	if err != nil {
		return err
	}
	challenge := pkceS256(verifier)

	authURL := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("login_hint", email),
		oauth2.ApprovalForce, // ensures we always receive a refresh_token on Gmail
	)

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			fmt.Fprintf(w, "Authorization failed: %s\nYou may close this window.", e)
			done <- result{err: fmt.Errorf("oauth2: provider returned error: %s", e)}
			return
		}
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			done <- result{err: errors.New("oauth2: state mismatch (possible CSRF)")}
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			done <- result{err: errors.New("oauth2: callback missing code parameter")}
			return
		}
		fmt.Fprintln(w, "Authorization complete. You can close this tab and return to Matcha.")
		done <- result{code: code}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	fmt.Fprintf(statusW, "Opening browser to authorize %s for %s.\n", provider.Name, email)
	fmt.Fprintf(statusW, "If it doesn't open automatically, visit:\n  %s\n", authURL)
	if err := openBrowser(authURL); err != nil {
		fmt.Fprintf(statusW, "matcha: could not auto-open browser: %v\n", err)
	}

	select {
	case res := <-done:
		if res.err != nil {
			return res.err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tok, err := cfg.Exchange(ctx, res.code,
			oauth2.SetAuthURLParam("code_verifier", verifier),
		)
		if err != nil {
			return fmt.Errorf("oauth2: token exchange failed: %w", err)
		}
		if tok.RefreshToken == "" {
			return errors.New("oauth2: provider did not return a refresh token; revoke prior consent and try again")
		}
		if err := saveToken(email, tok); err != nil {
			return fmt.Errorf("oauth2: could not persist token: %w", err)
		}
		// Persist the email→provider mapping so GetOAuth2Token can resolve the
		// provider for addresses whose domain isn't auto-detectable (Workspace
		// custom domains, etc.). Best-effort: refresh still works via fallback
		// detection for addresses where domain detection succeeds.
		// Persist the resolved provider so custom-domain (Workspace) accounts
		// survive refresh, where domain-based detection would fail.
		_ = keyring.Set(keyringServiceName, email+providerKeySuffix, provider.Key)
		fmt.Fprintln(statusW, "OAuth2 setup complete.")
		return nil
	case <-time.After(5 * time.Minute):
		return errors.New("oauth2: timed out waiting for browser callback")
	}
}

// RevokeOAuth2Token revokes the refresh token at the provider and deletes
// the local copy. Safe to call when no token is stored.
func RevokeOAuth2Token(email string) error {
	tok, err := loadToken(email)
	if err != nil {
		return nil // nothing to revoke
	}
	provider, err := providerForToken(email)
	if err == nil && provider.RevokeEndpoint != "" && tok.RefreshToken != "" {
		form := url.Values{"token": {tok.RefreshToken}}
		req, _ := http.NewRequest("POST", provider.RevokeEndpoint, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if resp, err := http.DefaultClient.Do(req.WithContext(ctx)); err == nil {
			resp.Body.Close()
		}
	}
	return deleteToken(email)
}

// ----- Provider definitions -----

type provider struct {
	Key             string // "gmail" or "outlook"
	Name            string
	AuthEndpoint    string
	TokenEndpoint   string
	RevokeEndpoint  string
	Scopes          []string
	CredentialsHelp []string
}

var providers = map[string]provider{
	"gmail": {
		Key:            "gmail",
		Name:           "Gmail",
		AuthEndpoint:   "https://accounts.google.com/o/oauth2/v2/auth",
		TokenEndpoint:  "https://oauth2.googleapis.com/token",
		RevokeEndpoint: "https://oauth2.googleapis.com/revoke",
		Scopes:         []string{"https://mail.google.com/"},
		CredentialsHelp: []string{
			"To set up Gmail OAuth2:",
			"  1. Go to https://console.cloud.google.com/apis/credentials",
			"  2. Create an OAuth 2.0 Client ID (Desktop application)",
			"  3. Re-run with --client-id and --client-secret",
		},
	},
	"outlook": {
		Key:            "outlook",
		Name:           "Outlook",
		AuthEndpoint:   "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenEndpoint:  "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		RevokeEndpoint: "", // Microsoft uses "logout"; nothing useful to call here
		Scopes: []string{
			"offline_access",
			"https://outlook.office.com/IMAP.AccessAsUser.All",
			"https://outlook.office.com/SMTP.Send",
		},
		CredentialsHelp: []string{
			"To set up Outlook OAuth2:",
			"  1. Register an app at https://portal.azure.com -> App registrations",
			"  2. Add a Public client redirect URI of http://localhost",
			"  3. Re-run with --client-id and --client-secret",
		},
	},
}

func (p provider) oauthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2.Endpoint{AuthURL: p.AuthEndpoint, TokenURL: p.TokenEndpoint},
		RedirectURL:  redirectURL,
		Scopes:       p.Scopes,
	}
}

func (p provider) helpText() string { return strings.Join(p.CredentialsHelp, "\n") }

func detectProvider(email string) (string, bool) {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return "", false
	}
	switch strings.ToLower(email[at+1:]) {
	case "gmail.com", "googlemail.com":
		return "gmail", true
	case "outlook.com", "hotmail.com", "live.com", "msn.com":
		return "outlook", true
	}
	return "", false
}

func resolveProvider(email, key string) (provider, error) {
	if key == "" {
		if k, ok := detectProvider(email); ok {
			key = k
		} else {
			return provider{}, fmt.Errorf("oauth2: cannot auto-detect provider for %s; pass --provider", email)
		}
	}
	p, ok := providers[strings.ToLower(key)]
	if !ok {
		return provider{}, fmt.Errorf("oauth2: unknown provider %q", key)
	}
	return p, nil
}

// providerForToken resolves an email to its OAuth provider. The keyring
// mapping wins so custom-domain accounts work; domain detection is the
// fallback for accounts that predate the mapping.
func providerForToken(email string) (provider, error) {
	if v, err := keyring.Get(keyringServiceName, email+providerKeySuffix); err == nil {
		if p, ok := providers[v]; ok {
			return p, nil
		}
	}
	if k, ok := detectProvider(email); ok {
		return providers[k], nil
	}
	return provider{}, fmt.Errorf("oauth2: cannot determine provider for %s; re-run `matcha oauth auth %s --provider <gmail|outlook> ...`", email, email)
}

// ----- Token & credential storage (keyring with disk migration) -----

const (
	tokenKeySuffix      = ":oauth-token"
	clientCredKeySuffix = ":oauth-client"   // key = "<provider>:oauth-client" or "<email>:oauth-client"
	providerKeySuffix   = ":oauth-provider" // key = "<email>:oauth-provider", value = "gmail"|"outlook"
)

func saveToken(email string, tok *oauth2.Token) error {
	b, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	if err := keyring.Set(keyringServiceName, email+tokenKeySuffix, string(b)); err == nil {
		// Best-effort: clean up any legacy on-disk token now that keyring works.
		_ = os.Remove(legacyTokenPath(email))
		return nil
	}
	// Fall back to disk if the keyring isn't available (e.g. headless server).
	path := legacyTokenPath(email)
	if path == "" {
		return errors.New("oauth2: keyring unavailable and config dir not resolvable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

func loadToken(email string) (*oauth2.Token, error) {
	if v, err := keyring.Get(keyringServiceName, email+tokenKeySuffix); err == nil {
		return decodeToken([]byte(v))
	}
	// Migration: pull from the legacy on-disk location written by the Python script.
	path := legacyTokenPath(email)
	if path == "" {
		return nil, errors.New("token not found")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("token not found")
	}
	tok, err := decodeToken(data)
	if err != nil {
		return nil, err
	}
	// Push it into the keyring and remove the file so we don't read it twice.
	if err := keyring.Set(keyringServiceName, email+tokenKeySuffix, string(data)); err == nil {
		_ = os.Remove(path)
	}
	return tok, nil
}

func deleteToken(email string) error {
	_ = keyring.Delete(keyringServiceName, email+tokenKeySuffix)
	// Remove any per-email client credential override; the provider-level
	// default is intentionally left in place since other accounts may rely on it.
	_ = keyring.Delete(keyringServiceName, email+clientCredKeySuffix)
	_ = keyring.Delete(keyringServiceName, email+providerKeySuffix)
	if path := legacyTokenPath(email); path != "" {
		_ = os.Remove(path)
	}
	return nil
}

// decodeToken accepts both the oauth2.Token shape (`expiry` as RFC3339) and
// the legacy Python shape (`expires_at` as unix timestamp). Combined-struct
// parsing — try-then-fallback would silently drop expiry because the other
// field tags overlap.
func decodeToken(data []byte) (*oauth2.Token, error) {
	var raw struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		TokenType    string    `json:"token_type"`
		Expiry       time.Time `json:"expiry"`     // native oauth2.Token shape
		ExpiresAt    float64   `json:"expires_at"` // legacy Python shape
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("oauth2: malformed token data: %w", err)
	}
	if raw.AccessToken == "" {
		return nil, errors.New("oauth2: token missing access_token")
	}
	tt := raw.TokenType
	if tt == "" {
		tt = "Bearer"
	}
	expiry := raw.Expiry
	if expiry.IsZero() && raw.ExpiresAt > 0 {
		expiry = time.Unix(int64(raw.ExpiresAt), 0)
	}
	return &oauth2.Token{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		TokenType:    tt,
		Expiry:       expiry,
	}, nil
}

// saveClientCredentials stores an OAuth client_id/client_secret pair under
// `key` (a provider name for the default, or an email for an override).
func saveClientCredentials(key, id, secret string) error {
	b, err := json.Marshal(struct {
		ID     string `json:"client_id"`
		Secret string `json:"client_secret"`
	}{id, secret})
	if err != nil {
		return err
	}
	return keyring.Set(keyringServiceName, key+clientCredKeySuffix, string(b))
}

// loadClientForAccount returns OAuth client credentials, preferring a
// per-email override over the provider-level default.
func loadClientForAccount(email, providerKey string) (string, string, error) {
	if id, secret, err := loadClientCredentials(email); err == nil {
		return id, secret, nil
	}
	return loadClientCredentials(providerKey)
}

// loadClientCredentials reads credentials from the keyring under `key`
// (provider name or email). For provider keys we also fall back to the legacy
// on-disk file written by the pre-Go OAuth helper; email keys skip that path
// because the helper only wrote per-provider files.
func loadClientCredentials(key string) (string, string, error) {
	v, err := keyring.Get(keyringServiceName, key+clientCredKeySuffix)
	if err != nil {
		if strings.Contains(key, "@") {
			return "", "", err
		}
		dir, derr := configDir()
		if derr != nil {
			return "", "", err
		}
		path := filepath.Join(dir, "oauth", key+"_client.json")
		data, ferr := os.ReadFile(path)
		if ferr != nil {
			return "", "", err
		}
		v = string(data)
	}
	var creds struct {
		ID     string `json:"client_id"`
		Secret string `json:"client_secret"`
	}
	if err := json.Unmarshal([]byte(v), &creds); err != nil {
		return "", "", fmt.Errorf("oauth2: malformed client credentials: %w", err)
	}
	if creds.ID == "" || creds.Secret == "" {
		return "", "", errors.New("oauth2: empty client credentials")
	}
	return creds.ID, creds.Secret, nil
}

func legacyTokenPath(email string) string {
	dir, err := configDir()
	if err != nil {
		return ""
	}
	// Mirror the Python script's path so existing installs upgrade cleanly.
	return filepath.Join(dir, "oauth_tokens", email+".json")
}

// ----- helpers -----

func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}
