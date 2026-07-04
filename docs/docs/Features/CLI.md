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

## matcha apply

Apply a patch you received by email — the output of `git format-patch` /
`git send-email` — to a local working tree. This is matcha's "git-mail"
workflow: review a patch in your inbox, then apply it without leaving the
terminal.

```bash
matcha apply [patch-file] [flags]
```

The patch is read from `patch-file`, or from **stdin** when no file is given
(or when the file is `-`). Matcha **never runs git** — it parses the email and
writes the file changes directly, confined to the target directory.

### Flags

| Flag | Description |
|------|-------------|
| `--repo` | Working tree to apply into (default: current directory) |
| `--check` | Validate only — report what would change, write nothing (dry run) |
| `--reverse` | Unapply the patch instead of applying it |
| `--series` | Treat the input as an mbox and apply the whole patch series in order |
| `-h` | Show help |

### Examples

**Apply a saved patch to a project:**

```bash
matcha apply fix.patch --repo ~/src/myproject
```

**Pipe a patch straight from git:**

```bash
git format-patch -1 --stdout | matcha apply --repo .
```

**Dry-run before committing to it:**

```bash
matcha apply --check fix.patch --repo ~/src/myproject
```

**Apply a whole series from an mbox:**

```bash
matcha apply series.mbox --series --repo ~/src/myproject
```

**Undo a patch you applied:**

```bash
matcha apply --reverse fix.patch --repo ~/src/myproject
```

### How it behaves

- **Transactional (per patch).** Every hunk is matched in memory before
  anything is written. If a hunk does not apply, the file is left untouched and
  the command exits non-zero — you never get a half-applied file.
- **Offset-tolerant, context-exact.** A patch still applies when surrounding
  edits have shifted the target lines, but the context itself must match —
  matcha will not silently patch the wrong place.
- **Path-confined.** Patches whose paths try to escape `--repo` (via `../` or
  an absolute path) are rejected before any file is touched.
- **Series caveat.** `--series` is transactional per patch, not across the
  whole series: if patch 3 of 5 conflicts, patches 1–2 are already written.
  Use `--check --series` first to validate the whole set.

Matcha does **not** create a git commit, move `HEAD`, or touch the index — it
only edits files. Commit the result yourself once you are happy with it.

> Under the hood, `matcha apply` uses the standalone
> [`go-mailpatch`](https://github.com/floatpane/go-mailpatch) (parsing) and
> [`go-patchapply`](https://github.com/floatpane/go-patchapply) (applying)
> libraries, extracted from matcha.

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Patch applied (or, with `--check`, would apply) cleanly |
| `1` | Error (parse failure, hunk conflict, unsafe path, missing/existing file) |

## matcha send-patch

Generate a patch from a local git repository and send it via email —
matcha's replacement for `git send-email`. It runs `git format-patch --stdout`
to produce the patch, then sends the resulting RFC 5322 message through your
configured SMTP account.

```bash
matcha send-patch [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--to` | Recipient email address **(required)** |
| `--repo` | Path to the git repository (default: current directory) |
| `--range` | Git commit range (default: `HEAD~1..HEAD`; e.g. `origin/main..HEAD`) |
| `--cc` | CC recipient(s), comma-separated |
| `--subject` | Override the patch subject. Defaults to the commit subject |
| `--version` | Patch series version (e.g. `2` for `[PATCH v2]`) |
| `--from` | Sender account email. Defaults to first configured account |
| `-h` | Show help |

### Examples

**Send the latest commit as a patch:**

```bash
matcha send-patch --to reviewer@example.com
```

**Send a range of commits from a specific repo:**

```bash
matcha send-patch --to reviewer@example.com --repo ~/src/myproject \
  --range origin/main..HEAD
```

**Send a v2 patch series with CC:**

```bash
matcha send-patch --to list@example.org --cc maintainer@example.org \
  --version 2 --range HEAD~3..HEAD
```

**Override the subject:**

```bash
matcha send-patch --to reviewer@example.com --subject "Fix critical bug" \
  --range HEAD~1..HEAD
```

**Send from a specific account:**

```bash
matcha send-patch --from work@company.com --to reviewer@example.com
```

### How it behaves

- **Patch generation.** Matcha runs `git format-patch --stdout` on the
  specified `--range` inside `--repo`. The output is a standard RFC 5322 email
  containing the commit message and unified diff.
- **From rewriting.** The `From:` header is rewritten to your configured
  account's sending identity so the SMTP server accepts it. The original git
  author is preserved in the patch body's `From:` line, as `git format-patch`
  always includes it.
- **Recipient collection.** Recipients are merged from the patch's own `To:`
  and `Cc:` headers, the `--cc` flag, and the `--to` flag (deduplicated).
- **Sent folder.** The sent message is appended to your Sent folder
  automatically, except for Gmail (which auto-appends sent messages).

> Under the hood, `matcha send-patch` uses the standalone
> [`go-patchapply`](https://github.com/floatpane/go-patchapply) library to
> generate the patch and matcha's built-in SMTP sender to deliver it. See the
> [patch email guide](../for-developers/patch-email.md) for the full
> architecture.

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Patch sent successfully |
| `1` | Error (missing flags, git failure, config error, send failure) |

## matcha marketplace

Open the interactive plugin marketplace in the terminal. Fetches the plugin registry from GitHub and displays a browsable list of available plugins.

```bash
matcha marketplace
```

Use `j/k` or arrow keys to navigate, `Enter` to install a plugin, and `q` to quit. Installed plugins are marked with an `[installed]` badge.

You can also access the marketplace from Matcha's main menu, or browse the [online marketplace](https://docs.matcha.email/marketplace).

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

## matcha dict

Manage spellcheck dictionaries. Dictionaries are downloaded from the
[wooorm/dictionaries](https://github.com/wooorm/dictionaries) Hunspell
repository and stored in `~/.config/matcha/dicts/<lang>.dic`.

```bash
matcha dict add <language-code>      # download and install a dictionary
matcha dict remove <language-code>   # delete an installed dictionary
matcha dict list                     # show installed dictionaries
```

The English dictionary (`en`) is downloaded automatically the first time
you open the composer — `matcha dict add` is only needed for additional
languages.

### Examples

```bash
matcha dict add en-GB     # British English
matcha dict add de        # German
matcha dict add fr        # French
matcha dict add es        # Spanish
matcha dict add ru        # Russian
matcha dict list
matcha dict remove fr
```

Language codes match the directory names under
[`dictionaries/`](https://github.com/wooorm/dictionaries/tree/main/dictionaries)
in the upstream repository.

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
