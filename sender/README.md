# sender

The `sender` package handles email composition and delivery over SMTP. It constructs properly formatted multipart MIME messages and sends them through the configured SMTP server.

## Architecture

This package is the SMTP client layer for Matcha. It:

- Builds multipart MIME messages with plain text, HTML, inline images, and file attachments
- Supports S/MIME detached signing and envelope encryption using PKCS#7
- Handles SMTP authentication with both PLAIN and LOGIN mechanisms (fallback for servers like Mailo)
- Supports both implicit TLS (port 465) and STARTTLS (other ports)
- Generates unique Message-IDs and handles reply threading via `In-Reply-To` and `References` headers
