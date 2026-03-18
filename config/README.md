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
| `config_test.go` | Unit tests for configuration logic. |
