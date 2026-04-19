# Background Daemon

Matcha includes an optional background daemon that keeps email connections alive, syncs mail, and sends desktop notifications — even when the TUI is closed.

## Features

- **Always-On IMAP IDLE**: Maintains persistent connections to detect new mail instantly.
- **Periodic Sync**: Fetches new emails every 5 minutes for all accounts.
- **Desktop Notifications**: Sends notifications when new mail arrives and the TUI is not running.
- **Instant TUI Startup**: When the TUI connects to a running daemon, email data is immediately available.
- **Automatic Fallback**: If the daemon is not running, the TUI works exactly as before (direct mode).

## Commands

```bash
matcha daemon start    # Start the daemon in the background
matcha daemon stop     # Stop the running daemon
matcha daemon status   # Show daemon status (PID, uptime, accounts)
matcha daemon run      # Run the daemon in the foreground (for systemd/launchd)
```

## How It Works

The daemon runs as a separate background process. It communicates with the TUI over a Unix domain socket using a JSON-based protocol.

```
┌─────────────┐     Unix Socket      ┌──────────────────┐
│  TUI Client  │◄──── JSON-RPC ─────►│     Daemon        │
│  (matcha)    │     bidirectional    │  (matcha daemon)  │
└─────────────┘                      └──────────────────┘
```

When you open the TUI:
1. It tries to connect to the daemon socket.
2. If the daemon is running, the TUI subscribes to folder updates and receives real-time push events.
3. If the daemon is not running, the TUI falls back to direct mode — identical to previous behavior.

## Status

```bash
$ matcha daemon status
Daemon running (PID 12345)
Uptime: 2h 15m
Accounts: 2
  - alice@gmail.com
  - bob@outlook.com
```

## Running as a System Service

### systemd (Linux)

Create `~/.config/systemd/user/matcha-daemon.service`:

```ini
[Unit]
Description=Matcha Email Daemon
After=network-online.target

[Service]
ExecStart=/usr/bin/matcha daemon run
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

```bash
systemctl --user enable matcha-daemon
systemctl --user start matcha-daemon
```

### launchd (macOS)

Create `~/Library/LaunchAgents/com.matcha.daemon.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.matcha.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/matcha</string>
        <string>daemon</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/com.matcha.daemon.plist
```

## File Paths

| File | Platform | Purpose |
|------|----------|---------|
| `$XDG_RUNTIME_DIR/matcha/daemon.sock` | Linux | Unix domain socket |
| `$XDG_RUNTIME_DIR/matcha/daemon.pid` | Linux | PID file |
| `~/Library/Caches/matcha/daemon.sock` | macOS | Unix domain socket |
| `~/Library/Caches/matcha/daemon.pid` | macOS | PID file |

## Architecture

The daemon is split across three packages:

- **`daemonrpc/`** — Shared protocol definitions (request/response types, event types, transport layer). Used by both daemon and client.
- **`daemon/`** — The daemon process itself: lifecycle management, RPC handlers, IDLE watchers, periodic sync, PID file management, signal handling.
- **`daemonclient/`** — Client library with a `Service` interface that abstracts daemon mode vs direct mode. The TUI uses this interface transparently.
