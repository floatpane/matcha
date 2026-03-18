# screenshots

This directory contains tooling for automated terminal UI screenshot generation using [VHS](https://github.com/charmbracelet/vhs), a terminal recording and screenshot tool.

## Architecture

Screenshots are generated through a two-layer system:

1. **Go helper programs** (`cmd/`) that render realistic mock UI views using real Matcha components with fake data
2. **VHS tape scripts** (`.tape` files) that launch these programs and capture terminal output as images

This pipeline runs in CI via the `.github/workflows/screenshots.yml` workflow to keep documentation screenshots up-to-date automatically.

## Files

### Helper Programs

| File | Description |
|------|-------------|
| `cmd/inbox_view/main.go` | Renders a mock inbox populated with realistic email entries for screenshot capture. |
| `cmd/email_view/main.go` | Renders a mock email view with headers, body content, and inline images for screenshot capture. |

The rest are the tapes themselves
