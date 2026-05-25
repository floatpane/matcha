---
title: Spellcheck
sidebar_position: 15
---

# Spellcheck

Matcha highlights misspelled words in the composer body with a red dotted
underline and shows an inline suggestion popup at the cursor — similar to
the experience you get in VS Code or other modern editors.

## How it works

- A Hunspell `.dic` word list is loaded into memory the first time the
  composer opens.
- The body text is post-processed on every render: words that aren't in
  the loaded dictionary get a red dotted underline (extended SGR
  sub-parameters).
- When the cursor sits at the end of a misspelled word, a bordered
  suggestion popup floats below the cursor with up to five candidates
  ranked by edit distance.

The English dictionary (`en`) is **downloaded automatically** the first
time you open the composer. Nothing else is required to get started.

## Dictionaries

Dictionaries are stored in `~/.config/matcha/dicts/<lang>.dic` and are
sourced from the [wooorm/dictionaries](https://github.com/wooorm/dictionaries)
Hunspell repository.

Manage them with the [`matcha dict`](./CLI.md#matcha-dict) CLI command:

```bash
matcha dict add en-GB    # British English
matcha dict add de       # German
matcha dict add fr       # French
matcha dict add es       # Spanish
matcha dict add ru       # Russian
matcha dict list
matcha dict remove fr
```

Language codes match the upstream directory names — see the
[full list](https://github.com/wooorm/dictionaries/tree/main/dictionaries).

Only one dictionary is active at a time per composer (currently English by
default). Additional dictionaries are stored on disk but not yet selected
automatically; the loader picks `en` first if present.

### Unknown scripts are skipped

If a word contains characters that don't appear in the loaded dictionary
(for example, Cyrillic text against an English-only dictionary, or French
accented characters when only `en` is installed), it is **not** flagged.
The check assumes you're writing in a language the dictionary can't judge
and leaves the text alone.

## Keybindings

The suggestion popup is controlled via the composer keybinds in
`~/.config/matcha/keybinds.json`:

| Action | Default |
|---|---|
| Next suggestion | `ctrl+n` (`composer.spell_next`) |
| Previous suggestion | `ctrl+p` (`composer.spell_prev`) |
| Accept selected | `tab` (`composer.spell_accept`) |
| Dismiss popup | `esc` (`composer.spell_dismiss`) |

Example — rebind to VS Code-style arrow + Enter navigation:

```json
{
  "composer": {
    "spell_next":    "down",
    "spell_prev":    "up",
    "spell_accept":  "enter",
    "spell_dismiss": "esc"
  }
}
```

While the popup is visible, the bound keys are intercepted before the
textarea sees them — so taking over the arrow keys is safe; the popup
closes the moment your cursor leaves the word and the keys go back to
normal navigation.

## Disabling

Both features can be toggled from **Settings → General**, or in
`config.json`:

```json
{
  "disable_spellcheck": true,
  "disable_spell_suggestions": true
}
```

- `disable_spellcheck` — disables underlines, popup, and dictionary
  download entirely.
- `disable_spell_suggestions` — keeps the underline, hides the popup.

Both default to `false` (i.e. spellcheck is on out of the box).

## Terminal support

The red dotted underline uses extended SGR sub-parameters
(`\e[4:4;58:2::255:0:0m`). Terminals that fully support it:

- kitty, Ghostty, WezTerm, foot
- iTerm2, modern xterm

Terminals that ignore the sub-parameters render a plain underline instead
— the misspelled word is still marked, just without the dotted style or
the red colour.
