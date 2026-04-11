# Encryption

Matcha supports optional full-disk encryption of all local data using a password you choose. The password is never stored anywhere -- not on disk, not in the OS keyring, not in environment variables. You enter it each time you open matcha.

## How It Works

When encryption is enabled:

1. All local files (config, email cache, email bodies, contacts, drafts, signatures) are encrypted using **AES-256-GCM**.
2. Your password is used to derive an encryption key via **Argon2id**, a memory-hard key derivation function designed to resist brute-force attacks.
3. A small metadata file (`secure.meta`) stores a random salt and an encrypted sentinel phrase. This is used to verify your password is correct -- if decrypting the sentinel produces the expected phrase, you're in.
4. Account passwords are stored inside the encrypted config file instead of the OS keyring, so everything is protected by a single password.
5. The derived key lives only in memory for the duration of your session.

## Enabling Encryption

1. Open **Settings** from the main menu.
2. Select **Encryption: OFF**.
3. Enter a password and confirm it.
4. Press **Enable Encryption**.

All existing data files will be encrypted immediately. On next launch, matcha will prompt for your password before showing anything.

## Unlocking Matcha

When encryption is enabled, matcha shows a lock screen on startup:

```
matcha is locked

> ********

enter: unlock | ctrl+c: quit
```

Enter your password to decrypt and proceed. If the password is wrong, you'll see an error and can try again.

## Disabling Encryption

1. Open **Settings** from the main menu.
2. Select **Encryption: ON**.
3. Confirm with **y** when prompted.

All files will be decrypted back to plain JSON, account passwords will be restored to the OS keyring, and the `secure.meta` file will be removed.

## Technical Details

| Property | Value |
|----------|-------|
| **Cipher** | AES-256-GCM (authenticated encryption) |
| **Key Derivation** | Argon2id (time=3, memory=64MB, threads=4) |
| **Key Size** | 256-bit (32 bytes) |
| **Salt** | 256-bit random, unique per installation |
| **Nonce** | Random per-file, prepended to ciphertext |
| **Password Storage** | Never stored. Derived key held in memory only. |

## What Gets Encrypted

- `~/.config/matcha/config.json` (accounts, settings, passwords)
- `~/.config/matcha/signatures/` (email signatures)
- `~/.cache/matcha/email_cache.json` (email metadata)
- `~/.cache/matcha/contacts.json` (contact autocomplete)
- `~/.cache/matcha/drafts.json` (saved drafts)
- `~/.cache/matcha/folder_cache.json` (folder listings)
- `~/.cache/matcha/folder_emails/` (per-folder email lists)
- `~/.cache/matcha/email_bodies/` (cached email bodies)

The `secure.meta` file itself is **not** encrypted -- it contains only the salt and encrypted sentinel needed to verify your password.

## Important Notes

- **If you forget your password, your data cannot be recovered.** There is no reset mechanism.
- The encryption protects data at rest. Once unlocked, data is decrypted in memory for the session.
- PGP keys and S/MIME certificates referenced by path in your config are not encrypted by matcha (they are external files managed by you).
- OAuth2 tokens are managed separately and are not covered by this encryption.
