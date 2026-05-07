---
title: Threaded View
sidebar_position: 13
---

# Threaded Conversation View

Matcha can group related emails into conversations using the JWZ threading
algorithm (the same approach used by mutt and other classic mail clients).
Replies, forwards, and quoted threads collapse under their root message so an
inbox of 200 individual messages can render as 30 conversations.

## Enabling threaded view

There are three ways to control threading:

### 1. Settings menu (global default)

- Press `Esc` from the inbox to open the main menu.
- Open **Settings** → **General**.
- Toggle **Threaded Conversation View** to ON.

This sets the default for every folder. New folders without an explicit
override inherit this default immediately.

### 2. Configuration file

Edit `~/.config/matcha/config.json` and add:

```json
{
  "enable_threaded": true
}
```

### 3. Keybind (per-folder override)

Press `T` (configurable as `inbox.toggle_threaded` in `keybinds.json`) from any
inbox view to toggle threading **for the current folder only**. The override is
persisted in the folder cache and survives restarts.

A per-folder override always wins over the global default. To return a folder
to the default, toggle it back to match the default value.

## Using threaded view

When threading is enabled the email list shows the root message of each
conversation with a count of replies. The default state is collapsed.

| Key      | Action                                  |
| -------- | --------------------------------------- |
| `T`      | Toggle threaded view for the folder     |
| `enter`  | Open the focused message                |
| `space`  | Expand or collapse the focused thread   |
| `j`/`k`  | Navigate threads or messages within     |

Visual mode (`v`), delete (`d`), archive (`a`), and the other inbox keybinds
behave the same as in flat view — operations applied to a collapsed thread
target the root message; expand the thread first to act on a single reply.

## How threading works

Matcha threads emails entirely on the client. Threading uses:

1. `Message-ID`, `In-Reply-To`, and `References` headers (RFC 5322).
2. A subject-based fallback that strips `Re:`, `Fwd:`, and locale-specific
   prefixes when reply headers are missing.

Threading is recomputed whenever the email cache changes for a folder, so new
mail slots into existing conversations without a manual refresh.

## Per-folder overrides

The setting is split into two layers:

- **Global default** — `Config.EnableThreaded` in `config.json`.
- **Per-folder override** — stored in `folder_cache.json` under
  `threaded_folders`. Only folders the user has explicitly toggled appear here.

If you change the global default in settings, every folder without an override
flips to the new default on the next render. Folders with an override keep
their explicit value until toggled again.
