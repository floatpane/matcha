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

### matcha.prompt(placeholder, callback)

Open a text input overlay in the composer. When the user presses Enter, the callback is called with their input string. If the user presses Esc, the prompt is cancelled and the callback is not called.

This function only works inside a `bind_key` callback for the `"composer"` area.

```lua
matcha.bind_key("ctrl+r", "composer", "rewrite", function(state)
  matcha.prompt("Enter instruction:", function(input)
    matcha.log("User typed: " .. input)
    -- Use matcha.http() + matcha.set_compose_field() to process and update the body
  end)
end)
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

## Marketplace

Matcha includes a built-in plugin marketplace with 35+ community plugins. You can browse and install plugins from the terminal or from the [online marketplace](/marketplace).

### Browse Plugins

Open the interactive TUI marketplace:

```bash
matcha marketplace
```

Use `j/k` or arrow keys to navigate, `Enter` to install a plugin, and `q` to quit. You can also access it from Matcha's main menu.

### Install a Plugin

Install from the marketplace or directly by URL:

```bash
matcha install https://raw.githubusercontent.com/floatpane/matcha/master/plugins/hello.lua
```

Install from a local file:

```bash
matcha install path/to/my_plugin.lua
```

Plugins are saved to `~/.config/matcha/plugins/` and loaded on next startup.

### Configure a Plugin

Open an installed plugin in your editor to change its settings:

```bash
matcha config hello          # opens ~/.config/matcha/plugins/hello.lua
matcha config                # opens ~/.config/matcha/config.json
```

### Submit Your Plugin

Anyone can add their plugin to the Matcha marketplace by submitting a pull request to the [matcha repository](https://github.com/floatpane/matcha).

1. Write your plugin as a `.lua` file following the API documented on this page.

2. Add an entry to [`plugins/registry.json`](https://github.com/floatpane/matcha/blob/master/plugins/registry.json):

   ```json
   {
     "name": "my_plugin",
     "title": "My Plugin",
     "description": "A short description of what your plugin does.",
     "file": "my_plugin.lua",
     "url": "https://raw.githubusercontent.com/YOUR_USER/YOUR_REPO/main/my_plugin.lua"
   }
   ```

   The `url` field points to where your plugin file is hosted. If you include the `.lua` file directly in the Matcha repo, you can omit `url` and it will default to the `plugins/` directory.

3. Submit your pull request. Once merged, your plugin will appear in the TUI marketplace, the CLI, and the [online marketplace](/marketplace).

**Guidelines:**
- Keep plugins focused — one plugin, one purpose.
- Include a comment header in your `.lua` file with a description.
- Test your plugin with the latest version of Matcha before submitting.
- Plugins run in a sandboxed environment — no external dependencies are available.

## Example Plugins

The repository includes 35+ example plugins. Here are a few to get started:

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
| `ai_rewrite.lua`   | AI-powered email rewriting in the composer     |

Browse the full list in the [Plugin Marketplace](/marketplace) or run `matcha marketplace`.

## Security

Plugins run in a sandboxed Lua 5.1 environment. The following standard libraries are available:

- `base` (print, type, tostring, pairs, ipairs, etc.)
- `string`
- `table`
- `math`
- `package` (for `require`)

The `os`, `io`, and `debug` libraries are **not** available. Plugins cannot access the filesystem or execute system commands.

Plugins can make HTTP requests via `matcha.http()`, with built-in safety limits: 10-second timeout, 1 MB response cap, and only `http`/`https` schemes.
