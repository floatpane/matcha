# PGP Email Security

Matcha supports PGP (Pretty Good Privacy) for signing and encrypting your emails. PGP is widely used in open-source communities, journalism, and privacy-focused environments.

## Features

- **Digital Signing**: Cryptographically sign outgoing emails so recipients can verify they came from you.
- **Encryption**: Encrypt emails so only the intended recipients can read them.
- **Signature Verification**: Automatically verify PGP signatures on incoming emails.
- **Encrypted Email Decryption**: Decrypt incoming PGP-encrypted emails using your private key.
- **Per-Account Configuration**: Configure separate keys for each email account.
- **Sign by Default**: Optionally enable automatic signing for all outgoing emails.
- **Encrypt by Default**: Optionally encrypt all outgoing emails when recipient keys are available.
- **YubiKey Support**: Sign emails using a YubiKey or other OpenPGP smartcard.

## Setting Up PGP (File-Based Keys)

### 1. Generate a PGP Key Pair

If you don't already have a PGP key, generate one with GnuPG:

```bash
gpg --full-generate-key
```

Follow the prompts to select:
- Key type: **RSA and RSA** (default)
- Key size: **4096** bits (recommended)
- Expiration: your preference
- Name and email address

### 2. Export Your Keys

Export your public and private keys to files:

```bash
# Create a directory for your keys
mkdir -p ~/.config/matcha/pgp
chmod 700 ~/.config/matcha/pgp

# Export your public key
gpg --export --armor your@email.com > ~/.config/matcha/pgp/public.asc

# Export your private key
gpg --export-secret-keys --armor your@email.com > ~/.config/matcha/pgp/private.asc

# Protect the private key
chmod 600 ~/.config/matcha/pgp/private.asc
```

### 3. Configure in Matcha

Open **Settings**, select an account, and configure the PGP section:

| Field | Description |
|-------|-------------|
| **Public Key Path** | Path to your public key file (e.g. `~/.config/matcha/pgp/public.asc`) |
| **Private Key Path** | Path to your private key file (e.g. `~/.config/matcha/pgp/private.asc`) |
| **Key Source** | `File` for file-based keys, `YubiKey` for hardware keys |
| **Sign by Default** | Toggle to automatically sign all outgoing emails |
| **Encrypt by Default** | Toggle to encrypt when recipient keys are available |

Your configuration is stored per-account in `~/.config/matcha/config.json`:

```json
{
  "accounts": [
    {
      "email": "you@example.com",
      "pgp_public_key": "/home/you/.config/matcha/pgp/public.asc",
      "pgp_private_key": "/home/you/.config/matcha/pgp/private.asc",
      "pgp_sign_by_default": true,
      "pgp_encrypt_by_default": false
    }
  ]
}
```

### 4. Sending Signed Emails

When **Sign by Default** is enabled, all outgoing emails are automatically signed with your PGP key. Recipients with PGP-capable email clients will see a verification indicator.

### 5. Sending Encrypted Emails

To encrypt an email, toggle the **Encrypt Email (PGP)** checkbox in the composer. For encryption to work, you need the recipient's public key stored in:

```
~/.config/matcha/pgp/<recipient-email>.asc
```

For example, to encrypt an email to `alice@example.com`, place her public key at:

```
~/.config/matcha/pgp/alice@example.com.asc
```

You can obtain someone's public key from:
- A keyserver: `gpg --recv-keys <key-id> && gpg --export --armor alice@example.com > ~/.config/matcha/pgp/alice@example.com.asc`
- Their website or email signature
- Direct exchange

Matcha automatically includes your own public key when encrypting, so you can still read the email in your Sent folder.

### Supported Key Formats

Matcha supports both common OpenPGP key formats:

| Format | Extension | Description |
|--------|-----------|-------------|
| ASCII-armored | `.asc` | Text-based format, starts with `-----BEGIN PGP PUBLIC KEY BLOCK-----` |
| Binary | `.gpg` | Compact binary format |

## Setting Up PGP with YubiKey

Matcha supports signing emails directly on a YubiKey or other OpenPGP-compatible smartcard. The private key never leaves the hardware device.

### Prerequisites

1. **A YubiKey with an OpenPGP key**: Your YubiKey must have a signing key loaded. You can either generate a key on-device or import an existing one using `gpg --edit-key`.

2. **PC/SC daemon**: The `pcscd` service must be running. This is the middleware that communicates with smartcards.

3. **CCID driver**: Required for USB smartcard communication.

#### Install on Arch Linux

```bash
sudo pacman -S pcsclite ccid
sudo systemctl enable --now pcscd.socket
```

#### Install on Debian/Ubuntu

```bash
sudo apt install pcscd libccid
sudo systemctl enable --now pcscd.socket
```

#### Install on Fedora

```bash
sudo dnf install pcsc-lite ccid
sudo systemctl enable --now pcscd.socket
```

#### Install on macOS

PC/SC is built into macOS. No additional installation needed.

### Configure in Matcha

1. Open **Settings** and select your account.
2. In the PGP section, set **Key Source** to `YubiKey`.
3. Enter your YubiKey PIN (stored securely in your OS keyring, never written to disk).
4. Enable **Sign by Default** if desired.

Your configuration:

```json
{
  "accounts": [
    {
      "email": "you@example.com",
      "pgp_key_source": "yubikey",
      "pgp_sign_by_default": true
    }
  ]
}
```

Note: The YubiKey PIN is stored in your OS keyring (e.g. GNOME Keyring, KDE Wallet, macOS Keychain) and is never saved to `config.json`.

### Moving Your GPG Key to a YubiKey

If you have an existing GPG key and want to move it to your YubiKey:

```bash
# Find your key ID
gpg --list-secret-keys --keyid-format long

# Edit the key
gpg --edit-key <KEY-ID>

# In the GPG prompt, move the signing subkey to the card
gpg> keytocard

# Select slot 1 (Signature key)
# Save and quit
gpg> save
```

### Generating a Key Directly on YubiKey

```bash
# Edit the card
gpg --card-edit

# Enter admin mode
gpg/card> admin

# Generate keys on-device (private key never leaves the YubiKey)
gpg/card> generate
```

## Status Indicators

When viewing an email, Matcha shows the PGP status in the header:

| Badge | Meaning |
|-------|---------|
| `[PGP: Verified]` | The PGP signature was verified successfully |
| `[PGP: Unverified]` | A PGP signature is present but could not be verified |
| `[PGP: Encrypted]` | The email was PGP-encrypted and decrypted successfully |

## PGP vs S/MIME

Matcha supports both PGP and S/MIME. They are mutually exclusive per message: you cannot sign the same email with both. Choose based on your needs:

| | PGP | S/MIME |
|---|-----|--------|
| **Key management** | Manual (keyservers, direct exchange) | Certificate Authorities |
| **Trust model** | Web of Trust / TOFU | Hierarchical (CA-based) |
| **Popular with** | Open-source, privacy communities | Enterprise, corporate |
| **Hardware keys** | YubiKey, smartcards | Smart cards, USB tokens |
| **Setup effort** | Lower (self-managed keys) | Higher (need CA-issued certificate) |

## Troubleshooting

### "Failed to connect to PC/SC daemon"

The `pcscd` service is not running.

```bash
# Start pcscd
sudo systemctl enable --now pcscd.socket

# Verify it's running
systemctl status pcscd.socket
```

### "No OpenPGP smartcard found"

The YubiKey is not detected. Check:

1. **Is the YubiKey plugged in?**
   ```bash
   lsusb | grep -i yubi
   ```

2. **Is the CCID driver installed?**
   ```bash
   # Arch Linux
   sudo pacman -S ccid

   # Debian/Ubuntu
   sudo apt install libccid
   ```

3. **Can pcscd see the card readers?**
   ```bash
   systemctl status pcscd
   ```
   Look for `LIBUSB_ERROR_ACCESS` or `LIBUSB_ERROR_BUSY` in the logs.

### "LIBUSB_ERROR_ACCESS" in pcscd logs

The `pcscd` user doesn't have permission to access the USB device. Add a udev rule:

```bash
# Create udev rule (adjust idProduct if needed)
echo 'ACTION=="add", SUBSYSTEM=="usb", ATTR{idVendor}=="1050", MODE="0666"' | \
  sudo tee /etc/udev/rules.d/70-yubikey.rules

# Reload rules and replug YubiKey
sudo udevadm control --reload-rules
sudo udevadm trigger
```

Then restart pcscd:

```bash
sudo systemctl restart pcscd.socket pcscd.service
```

### "LIBUSB_ERROR_BUSY" in pcscd logs

Another process (usually GnuPG's `scdaemon`) has an exclusive lock on the YubiKey. Configure `scdaemon` to share access through `pcscd`:

```bash
# Add to ~/.gnupg/scdaemon.conf
echo -e "disable-ccid\npcsc-shared" >> ~/.gnupg/scdaemon.conf

# Restart scdaemon
gpgconf --kill scdaemon
```

This tells `scdaemon` to use `pcscd` as its backend instead of grabbing the USB device directly, allowing both GnuPG and Matcha to share the YubiKey.

### "PIN verification failed"

- The default YubiKey PIN is `123456` (change it with `gpg --card-edit` then `passwd`).
- After 3 wrong PIN attempts, the PIN is locked. Reset it with the Admin PIN (default `12345678`) using `gpg --card-edit` then `admin` then `passwd`.

### "No PGP keys found in keyring"

Your exported key file may be empty or corrupted. Verify it:

```bash
# Check the public key
gpg --show-keys ~/.config/matcha/pgp/public.asc

# Check the private key
gpg --show-keys ~/.config/matcha/pgp/private.asc
```

### Signature shows as "Unverified"

This happens when the sender's public key is not available to verify the signature. To verify signatures from a contact, store their public key at:

```
~/.config/matcha/pgp/<sender-email>.asc
```
