---
title: Split View
sidebar_position: 12
---

# Split View

Matcha includes an optional split pane view that allows you to browse your inbox and read emails side-by-side without having to switch to a separate full-screen email view.

## Enabling Split View

You can enable split view in two ways:

1. **Via the Settings Menu:**
   - Open the main menu by pressing `Esc` from the inbox.
   - Select **Settings**, then **General**.
   - Toggle **View inbox and email side-by-side** to ON.

2. **Via Configuration File:**
   - Open `~/.config/matcha/config.json` in your editor.
   - Add `"enable_split_pane": true` to the root configuration.

## Using Split View

When split pane is enabled, pressing `Enter` on an email in your inbox will open the email in a preview pane on the right side of the screen, instead of switching to a full-screen view.

### Keybindings

When the split preview pane is open, the following keybindings are available:

| Key | Action |
|-----|--------|
| `]` | Switch focus to the preview pane |
| `[` | Switch focus back to the inbox list |
| `j`/`k` | Scroll the content of the currently focused pane (inbox or preview) |
| `Esc` | Close the split preview pane and return focus to the inbox |

## Visual Indicators

The currently focused pane will have a highlighted border, making it clear which side of the split view will receive your keyboard input for scrolling or other actions.
