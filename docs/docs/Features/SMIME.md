# S/MIME Email Security

Matcha supports S/MIME (Secure/Multipurpose Internet Mail Extensions) for signing and encrypting your emails. S/MIME provides end-to-end security, ensuring your messages are authentic and private.

## Features

- **🔏 Digital Signing**: Cryptographically sign outgoing emails so recipients can verify they came from you.
- **🔐 Encryption**: Encrypt emails so only the intended recipients can read them.
- **✅ Signature Verification**: Automatically verify S/MIME signatures on incoming emails.
- **📬 Encrypted Email Decryption**: Decrypt incoming S/MIME-encrypted emails using your private key.
- **⚙️ Per-Account Configuration**: Configure separate certificates and keys for each email account.
- **🔄 Sign by Default**: Optionally enable automatic signing for all outgoing emails.
- **📎 Recipient Certificates**: Store recipient public certificates for encryption.

## Setting Up S/MIME

### 1. Obtain a Certificate

You can either get a certificate from a trusted Certificate Authority (CA) or create a self-signed certificate for testing and personal use.

### 2. Configure in Matcha

Open **Settings** and select an account to configure S/MIME. You will need to provide:

| Field | Description |
|-------|-------------|
| **Certificate (PEM) Path** | Path to your public certificate file (e.g. `~/.certs/cert.pem`) |
| **Private Key (PEM) Path** | Path to your private key file (e.g. `~/.certs/private.pem`) |
| **Sign by Default** | Toggle to automatically sign all outgoing emails |

Your configuration is stored per-account in `~/.config/matcha/config.json`:

```json
{
  "accounts": [
    {
      "email": "you@example.com",
      "smime_cert": "/home/you/.certs/cert.pem",
      "smime_key": "/home/you/.certs/private.pem",
      "smime_sign_by_default": true
    }
  ]
}
```

### 3. Sending Signed Emails

When **Sign by Default** is enabled, all outgoing emails are automatically signed with your certificate. Recipients with S/MIME-capable email clients will see a verification indicator confirming the email came from you and hasn't been tampered with.

### 4. Sending Encrypted Emails

To encrypt an email, toggle the **Encrypt Email (S/MIME)** checkbox in the composer. For encryption to work, you need the recipient's public certificate stored in:

```
~/.config/matcha/certs/<recipient-email>.pem
```

For example, to encrypt an email to `alice@example.com`, place her public certificate at:

```
~/.config/matcha/certs/alice@example.com.pem
```

Matcha automatically includes your own certificate when encrypting, so you can still read the email in your Sent folder.

## Creating a Self-Signed Certificate

If you don't have a certificate from a CA, you can create a self-signed one using OpenSSL. This is useful for personal use or testing.

### Generate the Certificate and Key

```bash
# Create a directory for your certificates
mkdir -p ~/.certs

# Generate a private key and self-signed certificate in one step
openssl req -x509 -newkey rsa:4096 -keyout ~/.certs/private.pem -out ~/.certs/cert.pem \
  -days 365 -nodes -subj "/CN=Your Name/emailAddress=you@example.com"
```

| Flag | Description |
|------|-------------|
| `-x509` | Generate a self-signed certificate instead of a certificate request |
| `-newkey rsa:4096` | Create a new 4096-bit RSA key |
| `-keyout` | Path to write the private key |
| `-out` | Path to write the certificate |
| `-days 365` | Certificate validity period |
| `-nodes` | Do not encrypt the private key with a passphrase |
| `-subj` | Certificate subject (replace with your name and email) |

### Protect the Private Key

```bash
chmod 600 ~/.certs/private.pem
```

### Trusting Your Self-Signed Certificate

Recipients won't automatically trust a self-signed certificate. To avoid signature warnings, you (and your recipients) need to add the certificate to the system trust store.

#### macOS

```bash
# Add the certificate to the System keychain
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain ~/.certs/cert.pem
```

To trust it only for your user instead of system-wide:

```bash
security add-trusted-cert -r trustRoot \
  -k ~/Library/Keychains/login.keychain-db ~/.certs/cert.pem
```

#### Linux (Debian/Ubuntu)

```bash
sudo cp ~/.certs/cert.pem /usr/local/share/ca-certificates/my-smime.crt
sudo update-ca-certificates
```

#### Linux (Fedora/RHEL)

```bash
sudo cp ~/.certs/cert.pem /etc/pki/ca-trust/source/anchors/my-smime.pem
sudo update-ca-trust
```

### Verify the Certificate

```bash
# View certificate details
openssl x509 -in ~/.certs/cert.pem -text -noout

# Verify the certificate is valid
openssl verify ~/.certs/cert.pem
```

## Supported Key Formats

Matcha supports the following private key formats:

- **PKCS#8** (recommended) — `BEGIN PRIVATE KEY`
- **PKCS#1 RSA** — `BEGIN RSA PRIVATE KEY`
- **EC** — `BEGIN EC PRIVATE KEY` (for decryption of incoming emails)

All certificates and keys must be in **PEM format**.
