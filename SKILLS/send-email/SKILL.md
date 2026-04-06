---
name: send-email
description: >
  Use when the user wants to send an email, compose a message, or email someone.
  Triggers: send email, email someone, compose email, write email, mail to, message to,
  send a message, draft and send. Works with multiple accounts and recipients.
---

# Send Email Skill

Send emails from the terminal using matcha's configured accounts.

## How It Works

Matcha has a `send` CLI subcommand that sends emails non-interactively:

```
matcha send --to <recipients> --subject <subject> --body <body> [flags]
```

## Available Flags

| Flag | Description |
|------|-------------|
| `--to` | Recipient(s), comma-separated **(required)** |
| `--subject` | Email subject **(required)** |
| `--body` | Email body (Markdown supported). Use `"-"` to read from stdin |
| `--from` | Sender account email (defaults to first configured account) |
| `--cc` | CC recipient(s), comma-separated |
| `--bcc` | BCC recipient(s), comma-separated |
| `--attach` | Attachment file path (can be repeated for multiple files) |
| `--signature` | Append default signature (default: true). Use `--signature=false` to disable |
| `--sign-smime` | Sign with S/MIME (uses account default if not set) |
| `--encrypt-smime` | Encrypt with S/MIME |
| `--sign-pgp` | Sign with PGP (uses account default if not set) |

## Instructions

1. **Always ask the user** for the recipient (`--to`) and subject (`--subject`) if not provided.
2. **Ask for the body** or compose it based on the user's intent. The body supports Markdown.
3. **Account selection**: If the user specifies which account to send from, use `--from <email>`. Otherwise omit it to use the default account.
4. **Attachments**: If the user mentions files to attach, use `--attach <path>` for each one. This flag can be repeated.
5. **Stdin body**: For long or multi-line bodies, pipe content into the command with `--body -`.
6. **Show the command** to the user before running it so they can confirm.

## Examples

Simple email:
```bash
matcha send --to alice@example.com --subject "Meeting tomorrow" --body "Hi Alice, can we meet at 2pm?"
```

From a specific account:
```bash
matcha send --from work@company.com --to client@example.com --subject "Invoice" --body "Please find the invoice attached." --attach ~/Documents/invoice.pdf
```

Multiple recipients with CC:
```bash
matcha send --to alice@example.com,bob@example.com --cc manager@example.com --subject "Project update" --body "The project is on track."
```

Long body from stdin:
```bash
cat ~/notes/report.md | matcha send --to team@example.com --subject "Weekly Report" --body -
```

Multiple attachments:
```bash
matcha send --to alice@example.com --subject "Files" --body "Here are the files." --attach report.pdf --attach data.csv
```

No signature:
```bash
matcha send --to alice@example.com --subject "Quick note" --body "Thanks!" --signature=false
```

## Account Configuration

Accounts are configured in `~/.config/matcha/config.json`. The `--from` flag matches against both the login email and fetch email fields. If omitted, the first configured account is used.

To list configured accounts, the user can check their matcha settings in the TUI.
