# Password Command (`pass_cmd`)

Matcha can fetch your account password from an external command rather than the OS keyring. This lets you integrate any CLI-based password manager — [pass](https://www.passwordstore.org/), [gopass](https://github.com/gopasspw/gopass), [age](https://github.com/FiloSottile/age) scripts, or any tool that prints a password to stdout.

This is the same pattern used by [isync (`PassCmd`)](https://isync.sourceforge.io/mbsync.html) and [msmtp (`passwordeval`)](https://marlam.de/msmtp/msmtp.html).

## Configuration

Add `pass_cmd` to an account in `~/.config/matcha/config.json`:

```json
{
  "accounts": [
    {
      "id": "unique-id-1",
      "name": "John Doe",
      "email": "john@example.com",
      "service_provider": "custom",
      "imap_server": "imap.example.com",
      "smtp_server": "smtp.example.com",
      "pass_cmd": "pass show email/john@example.com"
    }
  ]
}
```

Matcha runs the command via `sh -c` at startup and uses its stdout (trailing newlines stripped) as the password. The password is never written to `config.json` or the OS keyring.

## Examples

### pass / gopass

```json
"pass_cmd": "pass show email/john@example.com"
```

```json
"pass_cmd": "gopass show -o email/john@example.com"
```

### age-encrypted file

```json
"pass_cmd": "age --decrypt -i ~/.age/key.txt ~/.secrets/mail.age"
```

### Custom script

```json
"pass_cmd": "/home/john/.local/bin/get-mail-password.sh"
```

The command can be anything that exits `0` and prints the password to stdout.

## Notes

- **Priority**: `pass_cmd` takes precedence over both the OS keyring and any password stored in a secure (encrypted) config. If `pass_cmd` is set, no other source is consulted.
- **Errors**: If the command exits non-zero or cannot be found, matcha logs the error and continues with an empty password, which will cause authentication to fail. Check the command works in a shell before adding it to your config.
- **Encryption compatibility**: `pass_cmd` works alongside [Encryption](/docs/Features/Encryption). The command is stored in the encrypted config, and the resolved password is never written to disk.
