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
| `matcha.style(text, opts)` | Wrap `text` in lipgloss styling and return an ANSI-styled string (see below) |

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
| `email_body_render` | `(email_table, rendered, raw)` — return a string to replace the rendered body, or `nil` to keep it | About to display an email body. `rendered` is the ANSI-styled display string; `raw` is the original message source (HTML or plain text). Use for recoloring, bold/italic, removing parts, or fully replacing the displayed body with parsed output |

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

## Body rendering

`matcha.on("email_body_render", function(email, rendered, raw) ... end)` runs
after the email body has been converted to its final ANSI-styled form and
before it is placed in the viewport. The callback receives:

- `email`: the same table as `email_viewed`
- `rendered`: the current display string (ANSI-styled, post-HTML→terminal)
- `raw`: the original message body (HTML or plain text) — useful for parsing
  the source instead of the rendered output

Return a new string to replace the rendered body, or `nil` to leave it
unchanged. Multiple registered callbacks chain in registration order; each
subsequent callback sees the previous callback's rendered output, but always
the same raw source.

`matcha.style(text, opts)` wraps `text` in lipgloss styling. `opts` keys (all
optional):

- `color`, `bg`: string color (hex `"#rrggbb"`, named like `"red"`, or ANSI 256 number as string)
- `bold`, `italic`, `underline`, `strikethrough`, `faint`, `blink`, `reverse`: bool

```lua
local matcha = require("matcha")

matcha.on("email_body_render", function(email, rendered, raw)
    -- highlight TODO in red bold (operates on rendered)
    rendered = rendered:gsub("TODO", function(m)
        return matcha.style(m, { color = "#ff0000", bold = true })
    end)
    -- italicize anything in *asterisks*
    rendered = rendered:gsub("%*([^%*]+)%*", function(m)
        return matcha.style(m, { italic = true })
    end)
    -- strip a tracking footer entirely
    rendered = rendered:gsub("%-%-%-%s*Sent via Tracker.*$", "")
    return rendered
end)

-- Parse the raw source and prepend a summary; works regardless of HTML markup.
matcha.on("email_body_render", function(email, rendered, raw)
    local urls = {}
    for url in raw:gmatch("https?://[%w%-_%.~%?=&/%%#:]+") do
        urls[#urls + 1] = url
    end
    local header = matcha.style("URLs: " .. #urls, { bold = true }) .. "\n\n"
    return header .. rendered
end)
```

Caveats:

- The `rendered` string already contains ANSI escape sequences from the
  HTML→terminal conversion. Patterns that straddle existing escapes will not
  match — match plain text spans for predictable behavior, or operate on `raw`.
- Returning a fully replaced string fully takes over the displayed body. To
  build styled output from scratch, compose with `matcha.style` and join with
  newlines.

## Available plugins

The following example plugins ship in `~/.config/matcha/plugins/`:

- `email_age.lua`
- `recipient_counter.lua`

## Files

| File | Description |
|------|-------------|
| `plugin.go` | Plugin manager — Lua VM setup, plugin discovery and loading, notification/status state |
| `hooks.go` | Hook definitions, callback registration, and hook invocation helpers |
| `api.go` | `matcha` Lua module registration (`on`, `log`, `notify`, `set_status`, `set_compose_field`, `bind_key`, `http`, `prompt`, `style`) |
| `http.go` | `matcha.http()` implementation — HTTP client with timeout and body size limits |
| `prompt.go` | `matcha.prompt()` implementation — user input overlay for the composer |
