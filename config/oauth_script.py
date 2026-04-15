#!/usr/bin/env python3
"""
OAuth2 helper for Matcha email client.

Handles the full OAuth2 flow for Gmail and Outlook:
  - Browser-based authorization
  - Localhost callback server for auth code capture
  - Token exchange and refresh
  - Secure token storage in ~/.config/matcha/oauth_tokens/

Usage:
  oauth.py auth   <email> [--provider gmail|outlook] [--client-id ID --client-secret SECRET]
  oauth.py token  <email>
  oauth.py revoke <email>

The 'auth' command initiates the OAuth2 flow, opening a browser.
The 'token' command prints a fresh access token to stdout (refreshing if needed).
The 'revoke' command deletes stored tokens for the given account.
"""

import argparse
import hashlib
import http.server
import json
import os
import secrets
import sys
import threading
import time
import urllib.parse
import urllib.request
import webbrowser

# --- Provider configuration ---

PROVIDERS = {
    "gmail": {
        "name": "Gmail",
        "auth_endpoint": "https://accounts.google.com/o/oauth2/v2/auth",
        "token_endpoint": "https://oauth2.googleapis.com/token",
        "revoke_endpoint": "https://oauth2.googleapis.com/revoke",
        "scopes": ["https://mail.google.com/"],
        "extra_auth_params": {
            "access_type": "offline",
            "prompt": "consent",
        },
        "credentials_help": [
            "To set up Gmail OAuth2:",
            "  1. Go to https://console.cloud.google.com/apis/credentials",
            "  2. Create an OAuth 2.0 Client ID (Desktop application)",
            "  3. Enable the Gmail API",
        ],
    },
    "outlook": {
        "name": "Outlook",
        "auth_endpoint": "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
        "token_endpoint": "https://login.microsoftonline.com/common/oauth2/v2.0/token",
        "revoke_endpoint": None,  # Microsoft does not support token revocation via endpoint
        "scopes": [
            "https://outlook.office365.com/IMAP.AccessAsUser.All",
            "https://outlook.office365.com/SMTP.Send",
            "offline_access",
        ],
        "extra_auth_params": {
            "prompt": "consent",
        },
        "credentials_help": [
            "To set up Outlook OAuth2:",
            "  1. Go to https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps/ApplicationsListBlade",
            "  2. Register a new application (any name, e.g. 'Matcha')",
            "  3. Set a redirect URI: http://localhost:8189 (Web platform)",
            "  4. Under 'Certificates & secrets', create a new client secret",
            "  5. Under 'API permissions', add:",
            "     - Microsoft Graph > Delegated > email",
            "     - Microsoft Graph > Delegated > offline_access",
            "     - Microsoft Graph > Delegated > User.Read",
            "     - Microsoft Graph > Delegated > Mail.ReadWrite",
            "     - Microsoft Graph > Delegated > Mail.Send",
            "     - Microsoft Graph > Delegated > IMAP.AccessAsUser.All",
            "     - Microsoft Graph > Delegated > SMTP.Send",
        ],
    },
}

REDIRECT_PORT = 8189
REDIRECT_URI = f"http://localhost:{REDIRECT_PORT}"


def get_token_dir():
    """Return the token storage directory, creating it if needed."""
    home = os.path.expanduser("~")
    token_dir = os.path.join(home, ".config", "matcha", "oauth_tokens")
    os.makedirs(token_dir, mode=0o700, exist_ok=True)
    return token_dir


def token_file_for(email):
    """Return the token file path for a given email address."""
    safe_name = hashlib.sha256(email.encode()).hexdigest()[:16]
    return os.path.join(get_token_dir(), f"{safe_name}.json")


def load_tokens(email):
    """Load stored tokens for the given email, or return None."""
    path = token_file_for(email)
    if not os.path.exists(path):
        return None
    with open(path, "r") as f:
        return json.load(f)


def save_tokens(email, tokens):
    """Save tokens to disk with restrictive permissions."""
    path = token_file_for(email)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as f:
        json.dump(tokens, f, indent=2)


def client_credentials_file_for(provider):
    """Return the client credentials file path for a given provider."""
    home = os.path.expanduser("~")
    if provider == "gmail":
        # Keep backwards-compatible path for Gmail
        return os.path.join(home, ".config", "matcha", "oauth_client.json")
    return os.path.join(home, ".config", "matcha", f"oauth_client_{provider}.json")


def load_client_credentials(provider):
    """Load OAuth2 client credentials for the given provider."""
    path = client_credentials_file_for(provider)
    if not os.path.exists(path):
        # Also try the generic path as fallback
        home = os.path.expanduser("~")
        generic = os.path.join(home, ".config", "matcha", "oauth_client.json")
        if provider != "gmail" and os.path.exists(generic):
            with open(generic, "r") as f:
                data = json.load(f)
            # Only use generic if it has provider-specific keys
            pid = data.get("provider")
            if pid == provider:
                return data.get("client_id"), data.get("client_secret")
        if not os.path.exists(path):
            return None, None
    with open(path, "r") as f:
        data = json.load(f)
    return data.get("client_id"), data.get("client_secret")


def save_client_credentials(provider, client_id, client_secret):
    """Save OAuth2 client credentials for the given provider."""
    path = client_credentials_file_for(provider)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as f:
        json.dump({"client_id": client_id, "client_secret": client_secret}, f, indent=2)


def exchange_code(code, client_id, client_secret, provider):
    """Exchange an authorization code for tokens."""
    cfg = PROVIDERS[provider]
    data = urllib.parse.urlencode(
        {
            "code": code,
            "client_id": client_id,
            "client_secret": client_secret,
            "redirect_uri": REDIRECT_URI,
            "grant_type": "authorization_code",
        }
    ).encode()

    req = urllib.request.Request(cfg["token_endpoint"], data=data, method="POST")
    req.add_header("Content-Type", "application/x-www-form-urlencoded")

    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read().decode())


def refresh_access_token(refresh_token, client_id, client_secret, provider):
    """Use a refresh token to get a new access token."""
    cfg = PROVIDERS[provider]
    params = {
        "refresh_token": refresh_token,
        "client_id": client_id,
        "client_secret": client_secret,
        "grant_type": "refresh_token",
    }
    # Outlook requires scope on refresh
    if provider == "outlook":
        params["scope"] = " ".join(cfg["scopes"])

    data = urllib.parse.urlencode(params).encode()

    req = urllib.request.Request(cfg["token_endpoint"], data=data, method="POST")
    req.add_header("Content-Type", "application/x-www-form-urlencoded")

    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read().decode())


def revoke_token(token, provider):
    """Revoke an OAuth2 token."""
    cfg = PROVIDERS[provider]
    if cfg["revoke_endpoint"] is None:
        return False

    data = urllib.parse.urlencode({"token": token}).encode()
    req = urllib.request.Request(cfg["revoke_endpoint"], data=data, method="POST")
    req.add_header("Content-Type", "application/x-www-form-urlencoded")

    try:
        with urllib.request.urlopen(req) as resp:
            return resp.status == 200
    except urllib.error.HTTPError:
        return False


class OAuthCallbackHandler(http.server.BaseHTTPRequestHandler):
    """HTTP handler that captures the OAuth2 callback."""

    auth_code = None
    error = None

    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        params = urllib.parse.parse_qs(parsed.query)

        if "code" in params:
            OAuthCallbackHandler.auth_code = params["code"][0]
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.end_headers()
            self.wfile.write(b"""
            <html><body style="font-family: sans-serif; text-align: center; padding-top: 50px;">
            <h2>Authorization successful!</h2>
            <p>You can close this window and return to Matcha.</p>
            </body></html>
            """)
        elif "error" in params:
            OAuthCallbackHandler.error = params["error"][0]
            self.send_response(400)
            self.send_header("Content-Type", "text/html")
            self.end_headers()
            self.wfile.write(
                f"""
            <html><body style="font-family: sans-serif; text-align: center; padding-top: 50px;">
            <h2>Authorization failed</h2>
            <p>Error: {params["error"][0]}</p>
            </body></html>
            """.encode()
            )
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        """Suppress HTTP server logs."""
        pass


def detect_provider(email):
    """Detect the OAuth2 provider from an email address."""
    domain = email.rsplit("@", 1)[-1].lower() if "@" in email else ""
    outlook_domains = {
        "outlook.com",
        "hotmail.com",
        "live.com",
        "msn.com",
        "outlook.co.uk",
        "hotmail.co.uk",
        "live.co.uk",
        "outlook.de",
        "hotmail.de",
        "outlook.fr",
        "hotmail.fr",
        "outlook.it",
        "hotmail.it",
        "outlook.es",
        "hotmail.es",
        "outlook.jp",
        "hotmail.co.jp",
    }
    if domain in ("gmail.com", "googlemail.com"):
        return "gmail"
    if domain in outlook_domains:
        return "outlook"
    return None


def do_auth(email, provider, client_id, client_secret):
    """Run the full OAuth2 authorization flow."""
    cfg = PROVIDERS[provider]
    state = secrets.token_urlsafe(32)

    auth_params = {
        "client_id": client_id,
        "redirect_uri": REDIRECT_URI,
        "response_type": "code",
        "scope": " ".join(cfg["scopes"]),
        "state": state,
        "login_hint": email,
    }
    auth_params.update(cfg["extra_auth_params"])

    auth_url = f"{cfg['auth_endpoint']}?{urllib.parse.urlencode(auth_params)}"

    # Reset handler state
    OAuthCallbackHandler.auth_code = None
    OAuthCallbackHandler.error = None

    # Start local HTTP server for callback
    server = http.server.HTTPServer(("localhost", REDIRECT_PORT), OAuthCallbackHandler)
    server.timeout = 120  # 2 minute timeout

    print(f"Opening browser for {cfg['name']} authorization...", file=sys.stderr)
    print(f"If the browser doesn't open, visit this URL:", file=sys.stderr)
    print(f"  {auth_url}", file=sys.stderr)

    webbrowser.open(auth_url)

    # Wait for the callback
    while OAuthCallbackHandler.auth_code is None and OAuthCallbackHandler.error is None:
        server.handle_request()

    server.server_close()

    if OAuthCallbackHandler.error:
        print(f"Authorization error: {OAuthCallbackHandler.error}", file=sys.stderr)
        sys.exit(1)

    code = OAuthCallbackHandler.auth_code
    print("Authorization code received, exchanging for tokens...", file=sys.stderr)

    # Exchange code for tokens
    token_response = exchange_code(code, client_id, client_secret, provider)

    if "error" in token_response:
        print(f"Token exchange error: {token_response['error']}", file=sys.stderr)
        sys.exit(1)

    # Store tokens with metadata
    tokens = {
        "access_token": token_response["access_token"],
        "refresh_token": token_response.get("refresh_token"),
        "expires_at": int(time.time()) + token_response.get("expires_in", 3600),
        "token_type": token_response.get("token_type", "Bearer"),
        "email": email,
        "provider": provider,
    }

    save_tokens(email, tokens)
    save_client_credentials(provider, client_id, client_secret)

    print("Authorization complete! Tokens saved.", file=sys.stderr)
    # Print the access token to stdout for immediate use
    print(tokens["access_token"])


def do_token(email):
    """Get a fresh access token, refreshing if needed."""
    tokens = load_tokens(email)
    if tokens is None:
        print("No tokens found. Run 'auth' first.", file=sys.stderr)
        sys.exit(1)

    provider = tokens.get("provider", "gmail")

    # Check if token is expired (with 5 minute buffer)
    if time.time() >= tokens.get("expires_at", 0) - 300:
        client_id, client_secret = load_client_credentials(provider)
        if not client_id or not client_secret:
            print("No client credentials found. Run 'auth' first.", file=sys.stderr)
            sys.exit(1)

        refresh_token = tokens.get("refresh_token")
        if not refresh_token:
            print("No refresh token available. Run 'auth' again.", file=sys.stderr)
            sys.exit(1)

        try:
            new_tokens = refresh_access_token(
                refresh_token, client_id, client_secret, provider
            )
        except urllib.error.HTTPError as e:
            print(f"Token refresh failed: {e}", file=sys.stderr)
            sys.exit(1)

        tokens["access_token"] = new_tokens["access_token"]
        tokens["expires_at"] = int(time.time()) + new_tokens.get("expires_in", 3600)
        # Refresh tokens may be rotated
        if "refresh_token" in new_tokens:
            tokens["refresh_token"] = new_tokens["refresh_token"]

        save_tokens(email, tokens)

    print(tokens["access_token"])


def do_revoke(email):
    """Revoke and delete stored tokens."""
    tokens = load_tokens(email)
    if tokens is None:
        print("No tokens found.", file=sys.stderr)
        sys.exit(1)

    provider = tokens.get("provider", "gmail")

    # Try to revoke the refresh token first, then access token
    revoked = False
    if tokens.get("refresh_token"):
        revoked = revoke_token(tokens["refresh_token"], provider)
    if not revoked and tokens.get("access_token"):
        revoked = revoke_token(tokens["access_token"], provider)

    # Delete local token file
    path = token_file_for(email)
    if os.path.exists(path):
        os.remove(path)

    if revoked:
        print("Token revoked and deleted.", file=sys.stderr)
    else:
        print(
            "Local tokens deleted (remote revocation may have failed).", file=sys.stderr
        )


def main():
    parser = argparse.ArgumentParser(description="OAuth2 helper for Matcha")
    subparsers = parser.add_subparsers(dest="command")

    # auth command
    auth_parser = subparsers.add_parser("auth", help="Authorize an email account")
    auth_parser.add_argument("email", help="Email address")
    auth_parser.add_argument(
        "--provider",
        help="OAuth2 provider (gmail or outlook)",
        choices=["gmail", "outlook"],
    )
    auth_parser.add_argument("--client-id", help="OAuth2 client ID")
    auth_parser.add_argument("--client-secret", help="OAuth2 client secret")

    # token command
    token_parser = subparsers.add_parser("token", help="Get a fresh access token")
    token_parser.add_argument("email", help="Email address")

    # revoke command
    revoke_parser = subparsers.add_parser("revoke", help="Revoke stored tokens")
    revoke_parser.add_argument("email", help="Email address")

    args = parser.parse_args()

    if args.command == "auth":
        provider = args.provider
        if not provider:
            provider = detect_provider(args.email)
        if not provider:
            print(
                "Error: Could not detect provider from email address.", file=sys.stderr
            )
            print("Use --provider gmail or --provider outlook", file=sys.stderr)
            sys.exit(1)

        client_id = args.client_id
        client_secret = args.client_secret

        # Fall back to stored credentials
        if not client_id or not client_secret:
            client_id, client_secret = load_client_credentials(provider)

        if not client_id or not client_secret:
            cfg = PROVIDERS[provider]
            print(
                f"Error: OAuth2 client credentials required for {cfg['name']}.",
                file=sys.stderr,
            )
            print("", file=sys.stderr)
            for line in cfg["credentials_help"]:
                print(line, file=sys.stderr)
            print("", file=sys.stderr)
            cred_file = client_credentials_file_for(provider)
            print(f"Create {cred_file} with:", file=sys.stderr)
            print(
                '  {"client_id": "YOUR_ID", "client_secret": "YOUR_SECRET"}',
                file=sys.stderr,
            )
            sys.exit(1)

        do_auth(args.email, provider, client_id, client_secret)

    elif args.command == "token":
        do_token(args.email)

    elif args.command == "revoke":
        do_revoke(args.email)

    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
