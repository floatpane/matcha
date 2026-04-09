# tui

The `tui` package contains all terminal user interface components built with the [Bubble Tea](https://github.com/charmbracelet/bubbletea) framework. Each file implements a self-contained view or component following the Bubble Tea Model-View-Update (MVU) pattern.

## Architecture

The TUI layer is the interactive frontend of Matcha. Each view implements the `tea.Model` interface (`Init`, `Update`, `View`) and communicates with other views through typed messages. The main application (`main.go`) orchestrates navigation between views by swapping the active model.

### Views

| File | Description |
|------|-------------|
| `inbox.go` | Email inbox list with multi-account tab support. Handles pagination, keyboard navigation, and renders email items with sender, subject, and date. Supports different mailbox types (inbox, sent, trash, archive) and both multi-account and single-account modes. |
| `email_view.go` | Full email display in a scrollable viewport. Shows headers (from, to, subject, date), rendered body content, attachment list, and S/MIME verification status. Manages inline image rendering through out-of-band stdout writes. |
| `composer.go` | Email composition form with fields for To, CC, BCC, Subject, and Body. Features contact autocomplete, file attachment picker, signature insertion, account selection dropdown, and draft auto-saving. Supports reply mode with pre-filled headers and quoted text. |
| `drafts.go` | Draft email list view. Displays saved drafts with subject, recipient, and timestamp. Allows opening drafts in the composer or deleting them. |
| `folder_inbox.go` | Folder navigation sidebar with an email list. Displays IMAP folders in a left panel and the selected folder's emails in the main area. Handles folder selection and email loading per folder. |
| `trash_archive.go` | Combined trash and archive view with tab-based switching between the two. Shares the inbox component structure but targets trash/archive mailboxes. |
| `login.go` | Account login form supporting Gmail, iCloud, and custom IMAP/SMTP providers. Collects credentials, server settings, and optionally S/MIME certificate paths. Validates input before submission. |
| `settings.go` | Settings panel for managing accounts (add/remove), configuring mailing lists, editing signatures, toggling image display, managing tips visibility, and setting up S/MIME certificates. |
| `mailing_list.go` | Editor for creating and modifying mailing list groups (name + comma-separated email addresses). |
| `choice.go` | Main menu / start screen. Presents account selection, navigation to inbox, compose, drafts, marketplace, sent, folders, trash/archive, and settings. |
| `marketplace.go` | Plugin marketplace browser. Fetches the plugin registry from GitHub, displays a scrollable list with install status badges, and installs plugins on selection. Can run standalone (`matcha marketplace`) or from the main menu. |
| `filepicker.go` | File browser for selecting email attachments. Navigates the filesystem with directory listing and file selection. |

### Supporting Files

| File | Description |
|------|-------------|
| `styles.go` | Global Lip Gloss style definitions used across all views (dialog boxes, help text, tips, headings, body text). Also defines the `Status` component for spinner-based loading messages. |
| `theme.go` | `RebuildStyles` function that updates all package-level style variables when the active theme changes, ensuring consistent colors across the UI. |
| `messages.go` | Shared message types for inter-component communication: `ViewEmailMsg`, `SendEmailMsg`, `Credentials`, `ChooseServiceMsg`, `EmailResultMsg`, `ClearStatusMsg`, `GoToMarketplaceMsg`, and the `MailboxKind` enum. |
| `signature.go` | Textarea-based editor for composing and saving email signatures. |

### Test Files

| File | Description |
|------|-------------|
| `inbox_test.go` | Tests for inbox rendering and behavior. |
| `email_view_test.go` | Tests for email view rendering. |
| `composer_test.go` | Tests for email composition logic. |
| `trash_archive_test.go` | Tests for trash/archive view behavior. |
