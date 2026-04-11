# config

The `config` package handles all persistent application state: user configuration, email/contacts/drafts caching, folder caching, email signatures, and optional at-rest encryption. Configuration is stored under `~/.config/matcha/`, cache data under `~/.cache/matcha/`.

## Architecture

This package acts as the data layer for Matcha. It manages:

- **Account configuration** with multi-account support (Gmail, iCloud, custom IMAP/SMTP)
- **Secure credential storage** via the OS keyring (with automatic migration from plain-text passwords), or inside the encrypted config when encryption is enabled
- **Local caches** for emails, email bodies, contacts, drafts, and folder listings to enable fast startup and offline browsing
- **Email body caching** for instant display of previously viewed emails without network round-trips
- **Email signatures** stored as plain text
- **Optional encryption** of all data files using AES-256-GCM with Argon2id key derivation

All files use JSON serialization with restrictive file permissions (`0600`/`0700`). When encryption is enabled, all files (except `secure.meta`) are encrypted before writing to disk.

## Storage Layout

Configuration (`~/.config/matcha/`):

| Path | Description |
|------|-------------|
| `config.json` | Account settings, preferences |
| `signature.txt` | Email signature |
| `secure.meta` | Encryption metadata (salt + sentinel), only present when encryption is enabled |
| `pgp/` | PGP keys |
| `plugins/` | Installed Lua plugins |
| `themes/` | Custom theme JSON files |

Cache (`~/.cache/matcha/`):

| Path | Description |
|------|-------------|
| `email_cache.json` | Email metadata cache |
| `contacts.json` | Contact autocomplete data |
| `drafts.json` | Saved email drafts |
| `folder_cache.json` | Folder listings per account |
| `folder_emails/` | Per-folder email list cache |
| `email_bodies/` | Cached email body content and attachment metadata |

On startup, `MigrateCacheFiles()` moves any cache files from the old location (`~/.config/matcha/`) to `~/.cache/matcha/`.

## Files

| File | Description |
|------|-------------|
| `config.go` | Core configuration types (`Account`, `Config`, `MailingList`) and functions for loading, saving, and managing accounts. Handles IMAP/SMTP server resolution per provider, OS keyring integration, legacy config migration, and cache directory management (`cacheDir()`, `MigrateCacheFiles()`). |
| `cache.go` | Email, contacts, drafts, and email body caching. Provides CRUD operations for `EmailCache`, `ContactsCache` (with search and frequency-based ranking), `DraftsCache` (with save/delete/get operations), and `EmailBodyCache` (per-folder body + attachment metadata caching with pruning). |
| `folder_cache.go` | Caches IMAP folder listings per account and per-folder email metadata. Stores folder names to avoid repeated IMAP `LIST` commands, and caches email headers per folder for fast navigation. |
| `encryption.go` | Optional at-rest encryption using AES-256-GCM with Argon2id key derivation. Provides `SecureReadFile`/`SecureWriteFile` (transparent encryption wrappers used by all other files), `EnableSecureMode`/`DisableSecureMode`, password verification via an encrypted sentinel phrase, and session key management. |
| `signature.go` | Loads and saves the user's email signature from `~/.config/matcha/signature.txt`. |
| `oauth.go` | OAuth2 integration — token retrieval, authorization flow launcher, and embedded Python helper extraction. |
| `oauth_script.py` | Embedded Gmail OAuth2 helper script (browser-based auth, token refresh, secure storage). |
| `config_test.go` | Unit tests for configuration logic. |

## Encryption

When enabled via Settings, all data files are encrypted at rest using a user-chosen password. The password is never stored anywhere.

### How it works

1. **Key derivation**: The password is combined with a random 256-bit salt using Argon2id (time=3, memory=64MB, threads=4) to produce a 256-bit AES key.
2. **Sentinel verification**: A known phrase (`"matcha-verified"`) is encrypted with the derived key and stored in `secure.meta`. On login, if decrypting the sentinel succeeds, the password is correct.
3. **Transparent I/O**: All file read/write operations go through `SecureReadFile`/`SecureWriteFile`. When a session key is set, these encrypt/decrypt automatically. When no key is set (encryption disabled), they pass through to plain `os.ReadFile`/`os.WriteFile`.
4. **Password storage**: When encryption is active, account passwords are stored inside the encrypted `config.json` (via `secureDiskAccount`) instead of the OS keyring. When encryption is disabled, passwords are restored to the keyring.

### `secure.meta` format

```json
{
  "salt": "<base64-encoded-32-byte-salt>",
  "sentinel": "<base64-encoded-AES-GCM-encrypted-sentinel>",
  "argon2_time": 3,
  "argon2_memory": 65536,
  "argon2_threads": 4
}
```

This file is never encrypted itself — its existence signals that encryption is enabled.

## Email Body Cache

Email bodies and attachment metadata are cached per-folder in `~/.cache/matcha/email_bodies/`. When a user views an email:

1. The cache is checked first (`GetCachedEmailBody`).
2. If found, the cached body is returned instantly without a network call.
3. If not found, the body is fetched from the server and saved to cache (`SaveEmailBody`).
4. When folder emails are refreshed, stale body cache entries (for emails no longer on the server) are pruned (`PruneEmailBodyCache`).

Attachment binary data is not cached — only metadata (filename, MIME type, part ID, etc.) is stored. Attachment downloads always go to the server.

## OAuth2 / XOAUTH2

Accounts with `auth_method: "oauth2"` use Gmail's XOAUTH2 mechanism instead of passwords. The flow works across three layers:

1. **`config/oauth.go`** — Go-side orchestration. Extracts the embedded Python helper to `~/.config/matcha/oauth/`, invokes it to run the browser-based authorization flow (`RunOAuth2Flow`) or to retrieve a fresh access token (`GetOAuth2Token`). The `IsOAuth2()` method on `Account` checks the auth method.

2. **`config/oauth_script.py`** — Embedded Python script that handles the full OAuth2 lifecycle:
   - `auth` — Opens a browser for Google authorization, captures the callback on `localhost:8189`, exchanges the code for tokens, and saves them to `~/.config/matcha/oauth_tokens/`.
   - `token` — Returns a fresh access token, automatically refreshing if expired (with a 5-minute buffer).
   - `revoke` — Revokes tokens with Google and deletes local storage.
   - Client credentials are stored in `~/.config/matcha/oauth_client.json`.

3. **`fetcher/xoauth2.go`** — Implements the XOAUTH2 SASL mechanism (`sasl.Client` interface) for IMAP/SMTP authentication. Formats the initial response as `user=<email>\x01auth=Bearer <token>\x01\x01` per Google's XOAUTH2 protocol spec.
