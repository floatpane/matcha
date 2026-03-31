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
| `composer_updated` | Table with `body`, `body_len`, `subject`, `to` | Composer content changed |

## Available plugins

The following example plugins ship in `~/.config/matcha/plugins/`:

- `email_age.lua`
- `recipient_counter.lua`

## Files

| File | Description |
|------|-------------|
| `plugin.go` | Plugin manager — Lua VM setup, plugin discovery and loading, notification/status state |
| `hooks.go` | Hook definitions, callback registration, and hook invocation helpers |
| `api.go` | `matcha` Lua module registration (`on`, `log`, `notify`, `set_status`) |
