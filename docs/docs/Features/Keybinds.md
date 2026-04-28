# Keybinds

Matcha lets you remap every keyboard shortcut. Bindings live in plain JSON at `~/.config/matcha/keybinds.json` and are written automatically the first time you launch the app.

## File location

```
~/.config/matcha/keybinds.json
```

Plain text, not encrypted. Edit with any text editor. Restart matcha to apply changes.

## Default bindings

```json
{
  "global": {
    "quit": "ctrl+c",
    "cancel": "esc",
    "nav_up": "k",
    "nav_down": "j"
  },
  "inbox": {
    "visual_mode": "v",
    "delete": "d",
    "archive": "a",
    "refresh": "r",
    "open": "enter",
    "next_tab": "l",
    "prev_tab": "h"
  },
  "email": {
    "reply": "r",
    "forward": "f",
    "delete": "d",
    "archive": "a",
    "toggle_images": "i",
    "rsvp_accept": "1",
    "rsvp_decline": "2",
    "rsvp_tentative": "3",
    "focus_attachments": "tab"
  },
  "composer": {
    "external_editor": "ctrl+e",
    "next_field": "tab",
    "prev_field": "shift+tab"
  },
  "folder": {
    "next_folder": "tab",
    "prev_folder": "shift+tab",
    "move": "m",
    "focus_preview": "]",
    "focus_inbox": "["
  },
  "drafts": {
    "open": "enter",
    "delete": "d"
  }
}
```

## Areas

| Area       | Where it applies                                         |
| ---------- | -------------------------------------------------------- |
| `global`   | Quit, cancel, vertical navigation — everywhere           |
| `inbox`    | Email list view (visual select, delete, archive, tabs)   |
| `email`    | Single-email view (reply, forward, RSVP, attachments)    |
| `composer` | New email / reply / forward editor                       |
| `folder`   | Folder sidebar + split-pane preview                      |
| `drafts`   | Draft list                                               |

The same key can appear in different areas without conflict — `d` is delete in both `inbox` and `email`, that's intentional. Conflicts only matter within one area.

## Key syntax

Standard [bubbletea](https://charm.land/bubbletea) key strings:

| Form              | Examples                          |
| ----------------- | --------------------------------- |
| Single character  | `a`, `1`, `?`                     |
| Modifier + key    | `ctrl+c`, `ctrl+e`, `shift+tab`   |
| Named key         | `enter`, `esc`, `tab`, `space`    |
| Arrow             | `up`, `down`, `left`, `right`     |

## Conflict warning

If two actions inside the same area share a key, matcha shows a yellow warning at the top of the start menu:

```
⚠ keybind conflict in inbox: "d" used for both "delete" and "archive"
```

The warning stays until you fix the binding. Both actions still fire on the shared key, but only the first one wins.

## What stays hardcoded

A few keys are never read from config — they exist as universal fallbacks:

- Arrow keys (`up`, `down`, `left`, `right`) — always navigate
- `y` / `n` on confirmation prompts
- `enter` inside modal pickers (file picker, account picker, move-to-folder)

This means even an empty or broken `keybinds.json` still leaves the app navigable.

## Reset to defaults

Delete the file:

```bash
rm ~/.config/matcha/keybinds.json
```

Next launch writes a fresh default file.

## Example: Emacs-style nav

```json
{
  "global": {
    "quit": "ctrl+x",
    "cancel": "esc",
    "nav_up": "ctrl+p",
    "nav_down": "ctrl+n"
  }
}
```

## Example: Single-key actions

```json
{
  "inbox": {
    "delete": "x",
    "archive": "e",
    "refresh": "g",
    "open": "enter",
    "visual_mode": "V",
    "next_tab": "n",
    "prev_tab": "p"
  }
}
```
