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

### matcha.set_compose_field(field, value)

Set a compose field value from a plugin. Only works when the composer is active (e.g. inside a `composer_updated` callback). The change is applied after the hook returns.

**Available fields:**

| Field       | Description              |
| ----------- | ------------------------ |
| `"to"`      | Recipient(s)             |
| `"cc"`      | CC recipient(s)          |
| `"bcc"`     | BCC recipient(s)         |
| `"subject"` | Subject line             |
| `"body"`    | Email body               |

```lua
-- Auto-add a BCC on every new email
matcha.on("composer_updated", function(state)
  if state.bcc == "" then
    matcha.set_compose_field("bcc", "archive@example.com")
  end
end)
```

### matcha.bind_key(key, area, description, callback)

Register a custom keyboard shortcut. The shortcut is scoped to a specific view area and shows up in the help bar. The callback receives a context table when the key is pressed.

**Parameters:**

| Parameter     | Type     | Description                                                    |
| ------------- | -------- | -------------------------------------------------------------- |
| `key`         | string   | Key string (e.g. `"ctrl+k"`, `"g"`, `"ctrl+shift+a"`)         |
| `area`        | string   | View area: `"inbox"`, `"email_view"`, or `"composer"`          |
| `description` | string   | Short text shown in the help bar                               |
| `callback`    | function | Called when the key is pressed; receives a context table        |

**Context tables by area:**

- **inbox / email_view**: Same email table as `email_viewed` (`uid`, `from`, `to`, `subject`, `date`, `is_read`, `account_id`, `folder`)
- **composer**: Same state table as `composer_updated` (`body`, `body_len`, `subject`, `to`, `cc`, `bcc`)

```lua
-- Add a shortcut to show email subject in inbox
matcha.bind_key("ctrl+i", "inbox", "info", function(email)
  if email then
    matcha.notify("Subject: " .. email.subject, 3)
  end
end)

-- Add a shortcut to insert a greeting in the composer
matcha.bind_key("ctrl+g", "composer", "greeting", function(state)
  matcha.set_compose_field("body", "Hi there,\n\n" .. state.body)
end)
```

### matcha.http(options)

Make an HTTP request. Takes a single options table and returns two values: a response table on success, or `nil` plus an error string on failure.

**Options table:**

| Field     | Type   | Required | Description                              |
| --------- | ------ | -------- | ---------------------------------------- |
| `url`     | string | yes      | Request URL (http or https only)         |
| `method`  | string | no       | HTTP method (default `"GET"`)            |
| `headers` | table  | no       | Request headers as key-value pairs       |
| `body`    | string | no       | Request body                             |

**Response table:**

| Field     | Type   | Description                                  |
| --------- | ------ | -------------------------------------------- |
| `status`  | number | HTTP status code (e.g. 200)                  |
| `body`    | string | Response body (capped at 1 MB)               |
| `headers` | table  | Response headers (lowercase keys)            |

**Limits:** Requests time out after 10 seconds. Response bodies are capped at 1 MB. Only `http://` and `https://` URLs are allowed.

```lua
-- GET request
local res, err = matcha.http({ url = "https://api.example.com/status" })
if err then
  matcha.log("error: " .. err)
  return
end
matcha.log("status: " .. res.status)

-- POST request with headers and body
local res, err = matcha.http({
  url     = "https://hooks.slack.com/services/xxx",
  method  = "POST",
  headers = { ["Content-Type"] = "application/json" },
  body    = '{"text":"New email received!"}',
})
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
| `cc`       | string | Current CC recipient(s)              |
| `bcc`      | string | Current BCC recipient(s)             |

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
| `webhook_notify.lua` | Posts to a webhook when emails arrive        |
| `weather_status.lua` | Shows current weather in the inbox status bar |

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

Plugins can make HTTP requests via `matcha.http()`, with built-in safety limits: 10-second timeout, 1 MB response cap, and only `http`/`https` schemes.
