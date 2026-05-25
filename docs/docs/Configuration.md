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
  "enable_split_pane": true,
  "enable_detailed_dates": true,
  "date_format": "DD/MM/YYYY HH:MM",
  "disable_images": true,
  "hide_tips": true,
  "disable_spellcheck": false,
  "disable_spell_suggestions": false,
  "body_cache_threshold_mb": 100
}
```

`send_as_email` is optional. When set, Matcha uses it for the outgoing `From` header while continuing to authenticate with the account's login address.

`enable_split_pane` enables a side-by-side view where the email list and the selected email are shown on the same screen.

`enable_detailed_dates` shows absolute inbox dates using your configured `date_format` instead of relative labels like "2 hours ago".

`disable_spellcheck` (default `false`) turns off the composer spellcheck entirely — no underline highlights, no dictionary download, no popup. Toggle via Settings → General → Spellcheck.

`disable_spell_suggestions` (default `false`) keeps the misspelled-word underline but suppresses the inline suggestion popup. Useful if you want a quiet check without an autocomplete-style overlay. Toggle via Settings → General → Spell Suggestions.

`body_cache_threshold_mb` sets the maximum size (in megabytes) for the local email body cache. When this limit is reached, least recently accessed cached emails are evicted across all folders to make room for new ones. Defaults to `100` MB if not specified.

## Data Locations

Configuration and persistent data are stored in `~/.config/matcha/`:

| File | Description |
|------|-------------|
| `config.json` | Account settings, preferences |
| `keybinds.json` | Custom keyboard shortcuts (see [Keybinds](/docs/Features/Keybinds)) |
| `signatures/` | Email signatures |
| `pgp/` | PGP keys |
| `plugins/` | Installed Lua plugins |
| `themes/` | Custom theme JSON files |
| `dicts/` | Hunspell spellcheck dictionaries (see [Spellcheck](/docs/Features/Spellcheck)) |
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
