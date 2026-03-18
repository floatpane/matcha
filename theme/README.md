# theme

The `theme` package defines the color palette system for Matcha's terminal UI. It provides built-in themes and supports user-created custom themes.

## Architecture

Each theme is a set of named colors (accent, danger, warning, link, etc.) used consistently across all UI components. The package:

- Defines 6 built-in themes: Matcha, Rose, Lavender, Ocean, Peach, and Catppuccin Mocha
- Supports custom themes loaded from `~/.config/matcha/themes/*.json`
- Maintains a global `ActiveTheme` variable that all UI components reference
- Uses Lip Gloss color values for terminal-compatible color rendering
