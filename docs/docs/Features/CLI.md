---
title: CLI
sidebar_position: 10
---

# CLI Commands

Matcha provides several subcommands for non-interactive use. These work without launching the TUI and are ideal for scripts, cron jobs, and AI agent integration.

## matcha send

Send an email directly from the command line.

```bash
matcha send --to <recipients> --subject <subject> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--to` | Recipient(s), comma-separated **(required)** |
| `--subject` | Email subject **(required)** |
| `--body` | Email body (Markdown supported). Use `"-"` to read from stdin |
| `--from` | Sender account email. Defaults to first configured account |
| `--cc` | CC recipient(s), comma-separated |
| `--bcc` | BCC recipient(s), comma-separated |
| `--attach` | Attachment file path. Can be repeated for multiple files |
| `--signature` | Append default signature (default: `true`). Use `--signature=false` to disable |
| `--sign-smime` | Sign with S/MIME. Uses account default if not set |
| `--encrypt-smime` | Encrypt with S/MIME |
| `--sign-pgp` | Sign with PGP. Uses account default if not set |

### Examples

**Simple email:**

```bash
matcha send --to alice@example.com --subject "Meeting tomorrow" --body "Can we meet at 2pm?"
```

**Send from a specific account:**

```bash
matcha send --from work@company.com --to client@example.com --subject "Invoice" \
  --body "Please find the invoice attached." --attach ~/Documents/invoice.pdf
```

**Multiple recipients with CC:**

```bash
matcha send --to alice@example.com,bob@example.com --cc manager@example.com \
  --subject "Project update" --body "The project is on track."
```

**Read body from stdin (useful for piping):**

```bash
cat ~/notes/report.md | matcha send --to team@example.com --subject "Weekly Report" --body -
```

**Multiple attachments:**

```bash
matcha send --to alice@example.com --subject "Files" --body "Here are the files." \
  --attach report.pdf --attach data.csv
```

**Without signature:**

```bash
matcha send --to alice@example.com --subject "Quick note" --body "Thanks!" --signature=false
```

### Account Selection

The `--from` flag matches against both the login email and fetch email of your configured accounts. If omitted, the first configured account is used.

```bash
# Use your work account
matcha send --from work@company.com --to someone@example.com --subject "Hi" --body "Hello"
```

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Email sent successfully |
| `1` | Error (missing flags, bad config, send failure) |

## matcha marketplace

Open the interactive plugin marketplace in the terminal. Fetches the plugin registry from GitHub and displays a browsable list of available plugins.

```bash
matcha marketplace
```

Use `j/k` or arrow keys to navigate, `Enter` to install a plugin, and `q` to quit. Installed plugins are marked with an `[installed]` badge.

You can also access the marketplace from Matcha's main menu, or browse the [online marketplace](https://docs.matcha.floatpane.com/marketplace).

## matcha install

Install a plugin from a URL or a local file.

```bash
matcha install <url_or_file>
```

### Examples

**Install from the official plugin repository:**

```bash
matcha install https://raw.githubusercontent.com/floatpane/matcha/master/plugins/hello.lua
```

**Install from a third-party URL:**

```bash
matcha install https://raw.githubusercontent.com/someone/repo/main/my_plugin.lua
```

**Install from a local file:**

```bash
matcha install ~/Downloads/custom_plugin.lua
```

Plugins are saved to `~/.config/matcha/plugins/` and loaded automatically on next startup. The file must have a `.lua` extension.

## matcha contacts export

Export your contacts cache to JSON or CSV format.

```bash
matcha contacts export [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-f` | Output format: `json` or `csv` (default: `json`) |
| `-o` | Output file path. If omitted, prints to stdout |
| `--no-header` | Omit CSV header row (CSV format only) |
| `-h` | Show help |

### Examples

**Export as JSON to stdout:**

```bash
matcha contacts export
```

**Export as CSV to stdout:**

```bash
matcha contacts export -f csv
```

**Export to a file:**

```bash
matcha contacts export -o ~/contacts.json
matcha contacts export -f csv -o ~/contacts.csv
```

**Export CSV without headers:**

```bash
matcha contacts export -f csv --no-header
```

If encryption is enabled, you will be prompted for your password before the contacts can be read.

### Output Format

**JSON** exports an array of contact objects with `name`, `email`, `last_used`, and `use_count` fields.

**CSV** exports a header row (`name,email,last_used,use_count`) followed by one row per contact. Use `--no-header` to omit the header row.

## matcha config

Open a configuration file in your `$EDITOR` (falls back to `vi`).

```bash
matcha config [plugin_name]
```

### Examples

**Open the main config file:**

```bash
matcha config
```

Opens `~/.config/matcha/config.json`.

**Open a plugin for configuration:**

```bash
matcha config ai_rewrite
```

Opens `~/.config/matcha/plugins/ai_rewrite.lua` so you can edit settings like API keys or model names.

## matcha update

Check for and install the latest version of Matcha.

```bash
matcha update
```

Automatically detects your installation method (Homebrew, Snap, Flatpak, WinGet, or binary) and updates accordingly.

## matcha oauth

Manage OAuth2 authorization for Gmail and Outlook.

```bash
matcha oauth auth <email>                        # Authorize an account (opens browser, auto-detects provider)
matcha oauth auth <email> --provider outlook     # Specify provider explicitly
matcha oauth token <email>                       # Print a fresh access token
matcha oauth revoke <email>                      # Revoke and delete stored tokens
```

`matcha gmail` is kept as an alias for backwards compatibility.

Client credentials are stored per provider:
- Gmail: `~/.config/matcha/oauth_client.json` — see the [Gmail setup guide](../setup-guides/gmail.md)
- Outlook: `~/.config/matcha/oauth_client_outlook.json` — see the [Outlook setup guide](../setup-guides/outlook.md)

## matcha version

Print the current version.

```bash
matcha --version
matcha -v
matcha version
```
