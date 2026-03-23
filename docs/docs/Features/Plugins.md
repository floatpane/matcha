# Plugins

Matcha supports Lua plugins for extending functionality. Plugins can react to events like receiving emails, sending messages, switching folders, and more.

## Getting Started

### Plugin Location

Place your plugins in `~/.config/matcha/plugins/`. Matcha loads them automatically on startup.

A plugin can be either:

- A single `.lua` file (e.g. `my_plugin.lua`)
- A directory with an `init.lua` entry point (e.g. `my_plugin/init.lua`)

```
~/.config/matcha/plugins/
├── hello.lua
├── notify_github.lua
└── my_plugin/
    └── init.lua
```

### Your First Plugin

Create `~/.config/matcha/plugins/hello.lua`:

```lua
local matcha = require("matcha")

matcha.on("startup", function()
  matcha.log("hello plugin loaded")
end)
```

Restart Matcha and check the log output. You should see `hello plugin loaded`.

## API Reference

All plugin functions are accessed through the `matcha` module:

```lua
local matcha = require("matcha")
```

### matcha.on(event, callback)

Register a function to be called when an event occurs.

```lua
matcha.on("email_received", function(email)
  matcha.log("New email from: " .. email.from)
end)
```

### matcha.log(message)

Write a message to Matcha's log output (stderr). Useful for debugging.

```lua
matcha.log("something happened")
```

### matcha.set_status(area, text)

Set a persistent status string displayed in a specific part of the UI. Pass an empty string to clear it.

**Available areas:**

| Area           | Where it appears                          |
| -------------- | ----------------------------------------- |
| `"inbox"`      | Inbox title bar, next to the folder name  |
| `"composer"`   | Composer help bar at the bottom           |
| `"email_view"` | Email viewer help bar at the bottom       |

```lua
matcha.set_status("inbox", "5 unread")      -- shows as "INBOX (5 unread)"
matcha.set_status("composer", "420 chars")  -- shows in composer help bar
matcha.set_status("inbox", "")              -- clears the inbox status
```

### matcha.notify(message [, seconds])

Show a temporary notification in the Matcha UI. The optional second argument sets how long the notification is displayed (default 2 seconds).

```lua
matcha.notify("You have new mail!")       -- shows for 2 seconds
matcha.notify("Important!", 5)            -- shows for 5 seconds
matcha.notify("Quick flash", 0.5)         -- shows for half a second
```

## Events

### startup

Fired once when Matcha starts, after all plugins are loaded.

```lua
matcha.on("startup", function()
  matcha.log("plugin ready")
end)
```

### shutdown

Fired when Matcha exits.

```lua
matcha.on("shutdown", function()
  matcha.log("goodbye")
end)
```

### email_received

Fired for each email when a folder's email list is fetched. Receives an email table.

```lua
matcha.on("email_received", function(email)
  matcha.log(email.from .. ": " .. email.subject)
end)
```

**Email table fields:**

| Field        | Type    | Description                    |
| ------------ | ------- | ------------------------------ |
| `uid`        | number  | Unique email ID                |
| `from`       | string  | Sender address                 |
| `to`         | table   | List of recipient addresses    |
| `subject`    | string  | Email subject line             |
| `date`       | string  | ISO 8601 date string           |
| `is_read`    | boolean | Whether the email has been read |
| `account_id` | string  | ID of the account              |
| `folder`     | string  | Folder name (e.g. "INBOX")     |

### email_viewed

Fired when you open an email to read it. Receives the same email table as `email_received`.

```lua
matcha.on("email_viewed", function(email)
  matcha.log("Reading: " .. email.subject)
end)
```

### email_send_before

Fired just before an email is sent. Receives a send table.

```lua
matcha.on("email_send_before", function(email)
  matcha.log("Sending to: " .. email.to)
end)
```

**Send table fields:**

| Field        | Type   | Description            |
| ------------ | ------ | ---------------------- |
| `to`         | string | Recipient(s)           |
| `cc`         | string | CC recipient(s)        |
| `subject`    | string | Email subject line     |
| `account_id` | string | Sending account ID     |

### email_send_after

Fired after an email is sent successfully. No arguments.

```lua
matcha.on("email_send_after", function()
  matcha.notify("Email sent!")
end)
```

### folder_changed

Fired when you switch to a different folder. Receives the folder name as a string.

```lua
matcha.on("folder_changed", function(folder)
  matcha.log("Now viewing: " .. folder)
end)
```

### composer_updated

Fired on every keystroke while the composer is active. Receives a state table with the current composer content.

```lua
matcha.on("composer_updated", function(state)
  matcha.set_status("composer", state.body_len .. " chars")
end)
```

**State table fields:**

| Field      | Type   | Description                          |
| ---------- | ------ | ------------------------------------ |
| `body`     | string | Current body text                    |
| `body_len` | number | Length of the body in bytes           |
| `subject`  | string | Current subject line                 |
| `to`       | string | Current recipient(s)                 |

## Example Plugins

Example plugins are included in the repository under `examples/plugins/`:

| Plugin               | Description                                  |
| -------------------- | -------------------------------------------- |
| `hello.lua`          | Minimal example that logs startup/shutdown   |
| `notify_github.lua`  | Notifies when GitHub emails arrive           |
| `send_logger.lua`    | Logs outgoing email details                  |
| `folder_announcer.lua` | Shows a notification on folder switch      |
| `unread_counter.lua` | Displays unread count in the inbox title     |
| `char_counter.lua`   | Live character count in the composer         |

To try one, copy it to your plugins directory:

```bash
mkdir -p ~/.config/matcha/plugins
cp examples/plugins/hello.lua ~/.config/matcha/plugins/
```

## Security

Plugins run in a sandboxed Lua 5.1 environment. The following standard libraries are available:

- `base` (print, type, tostring, pairs, ipairs, etc.)
- `string`
- `table`
- `math`
- `package` (for `require`)

The `os`, `io`, and `debug` libraries are **not** available. Plugins cannot access the filesystem or execute system commands.
