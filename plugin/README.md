# plugin

Lua-based plugin system for extending Matcha. Plugins are loaded from `~/.config/matcha/plugins/` and run inside a sandboxed Lua VM (no `os`, `io`, or `debug` libraries).

## How it works

The `Manager` creates a Lua VM at startup, registers the `matcha` module, and loads all plugins from the user's plugins directory. Plugins can be either a single `.lua` file or a directory with an `init.lua` entry point.

Plugins interact with Matcha by registering callbacks on hooks:

```lua
local matcha = require("matcha")

matcha.on("email_received", function(email)
    matcha.log("New email from: " .. email.from)
    matcha.notify("New mail!", 3)
end)
```

## Lua API (`matcha` module)

| Function | Description |
|----------|-------------|
| `matcha.on(event, callback)` | Register a callback for a hook event |
| `matcha.log(msg)` | Log a message to stderr |
| `matcha.notify(msg [, seconds])` | Show a temporary notification in the TUI (default 2s) |
| `matcha.set_status(area, text)` | Set a persistent status string for a view area (`"inbox"`, `"composer"`, `"email_view"`) |
| `matcha.set_compose_field(field, value)` | Set a compose field value (`"to"`, `"cc"`, `"bcc"`, `"subject"`, `"body"`) |
| `matcha.bind_key(key, area, description, callback)` | Register a custom keyboard shortcut for a view area (`"inbox"`, `"email_view"`, `"composer"`) |
| `matcha.http(options)` | Make an HTTP request (see below) |
| `matcha.prompt(placeholder, callback)` | Open a text input overlay in the composer (see below) |

## Hook events

| Event | Callback argument | Description |
|-------|-------------------|-------------|
| `startup` | — | Matcha has started |
| `shutdown` | — | Matcha is exiting |
| `email_received` | Lua table with `uid`, `from`, `to`, `subject`, `date`, `is_read`, `account_id`, `folder` | New email arrived |
| `email_viewed` | Same as `email_received` | User opened an email |
| `email_send_before` | Table with `to`, `cc`, `subject`, `account_id` | About to send an email |
| `email_send_after` | Same as `email_send_before` | Email sent successfully |
| `folder_changed` | Folder name (string) | User switched folders |
| `composer_updated` | Table with `body`, `body_len`, `subject`, `to`, `cc`, `bcc` | Composer content changed |

## HTTP requests

`matcha.http(options)` makes an HTTP request and returns `(response, err)`. Options is a table with:

- `url` (string, required) — only `http` and `https` schemes
- `method` (string, optional, default `"GET"`)
- `headers` (table, optional)
- `body` (string, optional)

The response table has `status` (number), `body` (string), and `headers` (table with lowercase keys).

Safety limits: 10s timeout, 1 MB response body cap.

```lua
local res, err = matcha.http({
    url     = "https://api.example.com/webhook",
    method  = "POST",
    headers = { ["Content-Type"] = "application/json" },
    body    = '{"text":"hello"}',
})
if err then
    matcha.log("error: " .. err)
    return
end
matcha.log("status: " .. res.status)
```

## User input prompts

`matcha.prompt(placeholder, callback)` opens a text input overlay in the composer. When the user presses Enter, the callback receives their input string. Pressing Esc cancels without calling the callback.

Only works inside a `bind_key` callback for the `"composer"` area.

```lua
matcha.bind_key("ctrl+r", "composer", "rewrite", function(state)
    matcha.prompt("Enter instruction:", function(input)
        -- input is the user's text
        matcha.log("User typed: " .. input)
    end)
end)
```

## Available plugins

The following example plugins ship in `~/.config/matcha/plugins/`:

- `email_age.lua`
- `recipient_counter.lua`

## Files

| File | Description |
|------|-------------|
| `plugin.go` | Plugin manager — Lua VM setup, plugin discovery and loading, notification/status state |
| `hooks.go` | Hook definitions, callback registration, and hook invocation helpers |
| `api.go` | `matcha` Lua module registration (`on`, `log`, `notify`, `set_status`, `set_compose_field`, `bind_key`, `http`, `prompt`) |
| `http.go` | `matcha.http()` implementation — HTTP client with timeout and body size limits |
| `prompt.go` | `matcha.prompt()` implementation — user input overlay for the composer |
