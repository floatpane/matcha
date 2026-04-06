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

## matcha update

Check for and install the latest version of Matcha.

```bash
matcha update
```

Automatically detects your installation method (Homebrew, Snap, Flatpak, WinGet, or binary) and updates accordingly.

## matcha gmail

Manage Gmail OAuth2 authorization.

```bash
matcha gmail auth <email>     # Authorize a Gmail account (opens browser)
matcha gmail token <email>    # Print a fresh access token
matcha gmail revoke <email>   # Revoke and delete stored tokens
```

Requires OAuth2 client credentials in `~/.config/matcha/oauth_client.json`. See the [Gmail setup guide](../setup-guides/gmail.md) for details.

## matcha version

Print the current version.

```bash
matcha --version
matcha -v
matcha version
```
