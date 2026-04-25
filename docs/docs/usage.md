---
title: Usage
sidebar_position: 3
---

# Usage

## First Launch

On first launch, Matcha will prompt you to configure an email account. You'll need:

- Your email address
- Your password (or app-specific password for Gmail/iCloud)
- Email provider (Gmail, iCloud, or Custom)

## Keyboard Shortcuts

### Main Menu

- `↑/↓` or `j/k` - Navigate menu items
- `Enter` - Select option
- `Esc` - Go back / Exit
- `Ctrl+C` - Quit application

### Inbox View

- `↑/↓` or `j/k` - Navigate emails
- `←/→` or `h/l` - Switch between account tabs
- `Enter` - Open selected email
- `/` - Filter/search emails
- `r` - Refresh inbox
- `[` / `]` - Switch focus between inbox and split pane (when split pane is enabled)
- `d` - Delete selected email
- `a` - Archive selected email
- `v` - Enter visual mode (multi-select)
- `Esc` - Back to main menu

#### Visual Mode (Multi-Select)

Visual mode allows you to select multiple emails and perform batch operations, similar to Vim's visual mode:

- `v` - Enter visual mode (selects current email)
- `↑/↓` or `j/k` - Expand/contract selection
- `d` - Delete all selected emails
- `a` - Archive all selected emails
- `m` - Move all selected emails to a folder
- `v` or `Esc` - Exit visual mode

**Visual indicators:**
- `>` - Cursor position
- `*` - Selected email (not cursor)
- `>*` - Selected email with cursor
- Title shows: "Inbox - VISUAL (N selected)"

**Features:**
- Efficient batch operations using single IMAP commands
- Automatic prevention of cross-account selections
- Works in both inbox and folder views
- Visual mode disabled during search/filtering for safety

### Email View

- `↑/↓` or `j/k` - Scroll email content
- `r` - Reply to email
- `d` - Delete email
- `a` - Archive email
- `Tab` - Focus attachments
- `Esc` - Back to inbox
- `i` - Toggle images

### Attachment View (when focused)

- `↑/↓` or `j/k` - Navigate attachments
- `Enter` - Download and open attachment
- `Tab` or `Esc` - Back to email body

### Composer

- `Tab` / `Shift+Tab` - Navigate fields
- `Enter` -
  - On "From" field: Select account (if multiple)
  - On "Attachment" field: Open file picker
  - On "Send" button: Send email
- `↑/↓` - Navigate contact suggestions (when typing in "To" field)
- `Esc` - Save draft and exit

## CLI Commands

Matcha includes several CLI subcommands that work without launching the TUI.

### Send Email

Send an email directly from the command line:

```bash
matcha send --to user@example.com --subject "Hello" --body "Hi there"
```

This is useful for scripts, automation, and [AI agent integration](./Features/AI_AGENTS.md). See the full [CLI reference](./Features/CLI.md) for all options.

### Update

Check for updates and install the latest version:

```bash
matcha update
```

This command will:

1. Check for the latest release on GitHub
2. Detect your installation method (Homebrew, Snap, or binary)
3. Update using the appropriate method

### Gmail OAuth2

Manage Gmail OAuth2 authorization:

```bash
matcha gmail auth <email>     # Authorize a Gmail account
matcha gmail token <email>    # Print a fresh access token
matcha gmail revoke <email>   # Revoke stored tokens
```
