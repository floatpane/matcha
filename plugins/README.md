# Matcha Plugin Marketplace

This directory contains the official Matcha plugin collection and the plugin registry that powers the marketplace.

## Installing Plugins

### From the TUI Marketplace

Browse and install plugins interactively:

```bash
matcha marketplace
```

Use `j/k` or arrow keys to navigate, `Enter` to install, and `q` to quit. You can also access the marketplace from Matcha's main menu.

### From a URL

```bash
matcha install https://raw.githubusercontent.com/floatpane/matcha/master/plugins/hello.lua
```

### From a Local File

```bash
matcha install path/to/my_plugin.lua
```

### Using curl

```bash
curl -sL https://raw.githubusercontent.com/floatpane/matcha/master/plugins/hello.lua \
  -o ~/.config/matcha/plugins/hello.lua
```

Plugins are installed to `~/.config/matcha/plugins/` and loaded automatically on next startup.

## Configuring Plugins

Open a plugin file in your editor to configure it:

```bash
matcha config hello          # opens ~/.config/matcha/plugins/hello.lua in $EDITOR
matcha config                # opens ~/.config/matcha/config.json in $EDITOR
```

## Submitting Your Plugin

Anyone can add their plugin to the marketplace by submitting a PR to this repository.

### Steps

1. **Write your plugin** as a `.lua` file following the [Plugin API docs](https://docs.matcha.floatpane.com/Features/Plugins).

2. **Add an entry to `registry.json`** in this directory:

   ```json
   {
     "name": "my_plugin",
     "title": "My Plugin",
     "description": "A short description of what your plugin does.",
     "file": "my_plugin.lua",
     "url": "https://raw.githubusercontent.com/YOUR_USER/YOUR_REPO/main/my_plugin.lua"
   }
   ```

3. **Submit a pull request** to [floatpane/matcha](https://github.com/floatpane/matcha).

### Registry Fields

| Field         | Required | Description                                                                 |
|---------------|----------|-----------------------------------------------------------------------------|
| `name`        | yes      | Unique identifier (lowercase, underscores). Must match the filename without `.lua`. |
| `title`       | yes      | Human-readable name shown in the marketplace.                               |
| `description` | yes      | One or two sentences describing what the plugin does.                       |
| `file`        | yes      | The `.lua` filename that gets saved to the user's plugins directory.        |
| `url`         | no       | Direct download URL for the plugin file. If omitted, defaults to this repo's `plugins/` directory. |

### Guidelines

- Keep plugins focused — one plugin, one purpose.
- Include a comment header in your `.lua` file with a description:
  ```lua
  -- my_plugin.lua
  -- A short description of what this plugin does.
  ```
- Test your plugin with the latest version of Matcha before submitting.
- Do not include external dependencies — plugins run in a sandboxed Lua environment.
- The `url` field should point to a raw file URL that stays stable (use a tagged release or `main` branch).

## Browsing Online

Visit the [Plugin Marketplace](https://docs.matcha.floatpane.com/marketplace) on the Matcha documentation site to browse all available plugins with one-line install commands.
