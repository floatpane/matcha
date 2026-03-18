# view

The `view` package handles rendering email content for terminal display. It converts HTML email bodies into styled terminal text with support for inline images, hyperlinks, and quoted reply formatting.

## Architecture

This package bridges raw email content (HTML/Markdown) and terminal output. It:

- Parses HTML email bodies using goquery and converts them to styled terminal text
- Renders hyperlinks using OSC 8 escape sequences (with fallback for unsupported terminals)
- Supports inline image rendering via multiple terminal graphics protocols:
  - **Kitty Graphics Protocol** (Kitty, Ghostty, WezTerm, Wayst, Konsole)
  - **iTerm2 Image Protocol** (iTerm2, Warp)
- Detects quoted reply sections (`>` prefixed lines and `On DATE, EMAIL wrote:` patterns) and renders them in styled quote boxes
- Manages image lifecycle: fetching remote images, resolving CID references, caching, uploading to terminal memory (Kitty IDs), and calculating terminal row placement
- Converts Markdown to HTML via Goldmark before processing
