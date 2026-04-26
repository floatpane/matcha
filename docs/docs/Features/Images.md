# Image Protocol Compatibility

> [!TIP] This feature is optional. To disable images, go to Settings > Image Display: OFF, you can also always turn it on for 1 email by pressing `i`

Matcha supports modern terminal image protocols for displaying images directly in your terminal. This allows for rich email viewing with inline images, including those embedded in HTML emails (CID references), remote images, and base64-encoded data URIs.

## Supported Protocols

### 🖼️ Kitty Graphics Protocol

Full support is provided for terminals implementing the Kitty Graphics Protocol.

**Supported Terminals:**
- [Kitty](https://sw.kovidgoyal.net/kitty/)
- [Ghostty](https://ghostty.org/)
- [WezTerm](https://wezfurlong.org/wezterm/)
- [Wayst](https://github.com/91861/wayst)
- [Konsole](https://konsole.kde.org/)

### 🖼️ Sixel

Support for the Sixel graphics format, a bitmap protocol widely supported across terminal emulators and multiplexers.

**Supported Terminals:**
- [Foot](https://codeberg.org/dnkl/foot)
- [mlterm](https://github.com/arakiken/mlterm)
- [Zellij](https://zellij.dev/) (multiplexer — works on top of any terminal)
- Any terminal with `TERM` containing `xterm` and `SIXEL=1` environment variable set

### 🖼️ iTerm2 Inline Images

Native support is included for the iTerm2 inline image protocol.

**Supported Terminals:**
- [iTerm2](https://iterm2.com/)
- [Warp](https://www.warp.dev/)

## Features

- **Inline Email Images**: Display images embedded in HTML emails.
- **Remote Image Fetching**: Automatically fetches and displays remote images from URLs.
- **Data URI Support**: Renders base64-encoded inline images.
- **Smart Fallback**: Gracefully falls back to clickable links when images aren't supported.

## Debugging

If images aren't displaying correctly, you can enable debug logging to troubleshoot:

```bash
export DEBUG_IMAGE_PROTOCOL=1
export DEBUG_IMAGE_PROTOCOL_LOG=/tmp/matcha-images.log
matcha
```

Check the log file at `/tmp/matcha-images.log` for detailed information about image rendering.
