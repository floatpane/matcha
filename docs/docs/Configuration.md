---
title: Configuration
sidebar_position: 5
---

# Configuration

Configuration is stored in `~/.config/matcha/config.json`.

## Example Configuration

> Passwords have been removed since [v0.19.0](https://github.com/floatpane/matcha/releases/tag/v0.19.0)

```json
{
  "accounts": [
    {
      "id": "unique-id-1",
      "name": "John Doe",
      "email": "john@gmail.com",
      "service_provider": "gmail",
      "fetch_email": "john@gmail.com",
      "send_as_email": "john@alias.example",
      "smime_cert": "/home/jane/.certs/jane_smime_cert.pem",
      "smime_key": "/home/jane/.certs/jane_smime_private.pem"
    },
    {
      "id": "unique-id-2",
      "name": "Work Email",
      "email": "john@company.com",
      "service_provider": "custom",
      "fetch_email": "john@company.com",
      "imap_server": "imap.company.com",
      "imap_port": 993,
      "smtp_server": "smtp.company.com",
      "smtp_port": 587
    }
  ],
  "mailing_lists": [
    {
      "name": "Team",
      "addresses": ["alice@example.com", "bob@example.com"]
    }
  ],
  "theme": "Matcha",
  "disable_images": true,
  "hide_tips": true
}
```

`send_as_email` is optional. When set, Matcha uses it for the outgoing `From` header while continuing to authenticate with the account's login address.

## Data Locations

Configuration and persistent data are stored in `~/.config/matcha/`:

| File | Description |
|------|-------------|
| `config.json` | Account settings, preferences |
| `signatures/` | Email signatures |
| `pgp/` | PGP keys |
| `plugins/` | Installed Lua plugins |
| `themes/` | Custom theme JSON files |
| `secure.meta` | Encryption metadata (only when encryption is enabled) |

Cache data is stored in `~/.cache/matcha/`:

| File | Description |
|------|-------------|
| `email_cache.json` | Email metadata cache |
| `contacts.json` | Contact autocomplete data |
| `drafts.json` | Saved email drafts |
| `folder_cache.json` | Folder listings per account |
| `folder_emails/` | Per-folder email list cache |
| `email_bodies/` | Cached email body content |

Cache files are automatically refreshed from the server on each app launch and manual refresh. If an email is removed from the server, its cache entry is cleaned up on the next refresh.

## Encryption

All data files can optionally be encrypted with a password. See [Encryption](/docs/Features/Encryption) for details.

When encryption is enabled, account passwords are stored inside the encrypted `config.json` instead of the OS keyring.
