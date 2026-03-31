# backend

The `backend` package defines a unified `Provider` interface for multi-protocol email support and provides three protocol implementations: IMAP, JMAP, and POP3.

## Architecture

This package acts as an abstraction layer, allowing the rest of the application to interact with mail servers through a consistent interface regardless of the underlying protocol. Implementations self-register at init time via `RegisterBackend`, and the factory function `New()` creates the right provider based on the account's `Protocol` field (defaults to `"imap"`).

### Provider interface

The `Provider` interface composes five sub-interfaces:

| Interface | Methods | Purpose |
|-----------|---------|---------|
| `EmailReader` | `FetchEmails`, `FetchEmailBody`, `FetchAttachment` | Retrieve email lists, bodies, and raw attachments |
| `EmailWriter` | `MarkAsRead`, `DeleteEmail`, `ArchiveEmail`, `MoveEmail` | Modify email state and location |
| `EmailSender` | `SendEmail` | Send outgoing mail |
| `FolderManager` | `FetchFolders` | List available mailboxes |
| `Notifier` | `Watch` | Real-time push notifications for mailbox changes |

Backends that don't support an operation return `ErrNotSupported`.

## Protocols

### IMAP (`backend/imap`)

Wraps the existing `fetcher` and `sender` packages behind the `Provider` interface. IMAP IDLE is handled externally in `main.go`, so `Watch()` returns `ErrNotSupported`.

### JMAP (`backend/jmap`)

Native JMAP implementation (RFC 8620 / RFC 8621) using `go-jmap`. Supports OAuth2 and Basic Auth, real-time push via JMAP EventSource, and full mailbox operations including send (via `EmailSubmission`). JMAP string IDs are hashed to `uint32` UIDs for interface compatibility.

### POP3 (`backend/pop3`)

POP3 + SMTP implementation. Inherently limited to a single INBOX folder, no read flags, no move/archive, and no push notifications. Uses the `sender` package for outgoing mail.

## Files

| File | Description |
|------|-------------|
| `backend.go` | Core interfaces and data types (`Provider`, `Email`, `Attachment`, `Folder`, `OutgoingEmail`, `NotifyEvent`, `Capabilities`) |
| `factory.go` | Protocol registry and `New()` factory function |
| `imap/imap.go` | IMAP provider — adapter over `fetcher` and `sender` packages |
| `jmap/jmap.go` | JMAP provider — native implementation with session management and mailbox caching |
| `pop3/pop3.go` | POP3 provider — per-connection model with UIDL-based UID hashing |
