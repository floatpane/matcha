# fetcher

The `fetcher` package handles all email retrieval operations over IMAP. It connects to mail servers, fetches email headers and bodies, manages attachments, and supports mailbox operations like delete, archive, and move.

## Architecture

This package is the IMAP client layer for Matcha. It:

- Establishes TLS/STARTTLS connections to IMAP servers based on account configuration
- Fetches email lists with pagination and per-account filtering (using `FetchEmail` to match relevant messages)
- Retrieves full email bodies with MIME part traversal (preferring HTML over plain text)
- Handles attachments including inline images (with CID references) and file attachments
- Supports S/MIME decryption (opaque and enveloped) and detached signature verification
- Provides mailbox operations: delete (expunge), archive (move), and folder-to-folder moves
- Exposes both mailbox-specific and convenience functions (e.g., `FetchEmails` defaults to INBOX)
