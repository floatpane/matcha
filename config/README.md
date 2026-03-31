# config

The `config` package handles all persistent application state: user configuration, email/contacts/drafts caching, folder caching, and email signatures. All data is stored as JSON files under `~/.config/matcha/`.

## Architecture

This package acts as the data layer for Matcha. It manages:

- **Account configuration** with multi-account support (Gmail, iCloud, custom IMAP/SMTP)
- **Secure credential storage** via the OS keyring (with automatic migration from plain-text passwords)
- **Local caches** for emails, contacts, drafts, and folder listings to enable fast startup and offline browsing
- **Email signatures** stored as plain text

All cache files use JSON serialization with restrictive file permissions (`0600`/`0700`).

## Files

| File | Description |
|------|-------------|
| `config.go` | Core configuration types (`Account`, `Config`, `MailingList`) and functions for loading, saving, and managing accounts. Handles IMAP/SMTP server resolution per provider, OS keyring integration, and legacy config migration. |
| `cache.go` | Email, contacts, and drafts caching. Provides CRUD operations for `EmailCache`, `ContactsCache` (with search and frequency-based ranking), and `DraftsCache` (with save/delete/get operations). |
| `folder_cache.go` | Caches IMAP folder listings per account and per-folder email metadata. Stores folder names to avoid repeated IMAP `LIST` commands, and caches email headers per folder for fast navigation. |
| `signature.go` | Loads and saves the user's email signature from `~/.config/matcha/signature.txt`. |
| `oauth.go` | OAuth2 integration — token retrieval, authorization flow launcher, and embedded Python helper extraction. |
| `oauth_script.py` | Embedded Gmail OAuth2 helper script (browser-based auth, token refresh, secure storage). |
| `config_test.go` | Unit tests for configuration logic. |

## OAuth2 / XOAUTH2

Accounts with `auth_method: "oauth2"` use Gmail's XOAUTH2 mechanism instead of passwords. The flow works across three layers:

1. **`config/oauth.go`** — Go-side orchestration. Extracts the embedded Python helper to `~/.config/matcha/oauth/`, invokes it to run the browser-based authorization flow (`RunOAuth2Flow`) or to retrieve a fresh access token (`GetOAuth2Token`). The `IsOAuth2()` method on `Account` checks the auth method.

2. **`config/oauth_script.py`** — Embedded Python script that handles the full OAuth2 lifecycle:
   - `auth` — Opens a browser for Google authorization, captures the callback on `localhost:8189`, exchanges the code for tokens, and saves them to `~/.config/matcha/oauth_tokens/`.
   - `token` — Returns a fresh access token, automatically refreshing if expired (with a 5-minute buffer).
   - `revoke` — Revokes tokens with Google and deletes local storage.
   - Client credentials are stored in `~/.config/matcha/oauth_client.json`.

3. **`fetcher/xoauth2.go`** — Implements the XOAUTH2 SASL mechanism (`sasl.Client` interface) for IMAP/SMTP authentication. Formats the initial response as `user=<email>\x01auth=Bearer <token>\x01\x01` per Google's XOAUTH2 protocol spec.
