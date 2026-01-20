<div align="center">

---

<img src = "assets/logo.png" width=200 height=200>

```
             __       __
    ____ ___  ____ _/ /______/ /_  ____ _
    / __ '__ \/ __ '/ __/ ___/ __ \/ __ '/
  / / / / / / /_/ / /_/ /__/ / / / /_/ /
/_/ /_/ /_/\__,_/\__/\___/_/ /_/\__,_/
```

---

[![Go CI](https://github.com/floatpane/matcha/actions/workflows/ci.yml/badge.svg)](https://github.com/floatpane/matcha/actions/workflows/ci.yml)
[![Go Release](https://github.com/floatpane/matcha/actions/workflows/release.yml/badge.svg)](https://github.com/floatpane/matcha/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/floatpane/matcha)](https://golang.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/floatpane/matcha)](https://goreportcard.com/report/github.com/floatpane/matcha)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/floatpane/matcha)](https://github.com/floatpane/matcha/releases)
[![GitHub All Releases](https://img.shields.io/github/downloads/floatpane/matcha/total)](https://github.com/floatpane/matcha/releases)
[![GitHub license](https://img.shields.io/github/license/floatpane/matcha)](https://github.com/floatpane/matcha/blob/master/LICENSE)

**A powerful, feature-rich email client for your terminal.** Built with Go and the Bubble Tea TUI framework, Matcha brings a beautiful, modern email experience to the command line with support for rich content, multiple accounts, and advanced terminal features.


</div>

![Demo GIF](public/assets/demo.gif)

## Installation
There are several ways to install Matcha:

### Package Managers

#### Homebrew (macOS & Linux)

```bash
brew tap floatpane/matcha
brew install floatpane/matcha/matcha
```

After installation, run:

```bash
matcha
```

> [!WARNING]
> If you have the [*"other"* Matcha](https://github.com/piqoni/matcha) already installed, you will have to rename the executable


### Install using Snap (Linux)

> [!NOTE]
> When using Snap, the config folder is going to be located at `~/snap/matcha/36/.config`, see [why](https://github.com/floatpane/matcha/issues/117)

```bash
sudo snap install matcha
```

### Build from Source

Matcha is written in **Go**. To build it manually:

1. Ensure you have Go installed (`go version`).
2. Clone the repository:

   ```bash
   git clone https://github.com/floatpane/matcha.git
   ```

3. Navigate to the project folder:

   ```bash
   cd matcha
   ```

4. Build the binary:

   ```bash
   go build -trimpath -ldflags="-s -w" -o matcha
   ```

   For an even smaller binary, compress with UPX (install via `brew install upx` or `apt install upx`):

   ```bash
   upx --best --lzma matcha
   ```

> [!WARNING]
> UPX compression does NOT work on macOS ARM builds. See [#97](https://github.com/floatpane/matcha/pull/97)

5. Run it:
   ```bash
   ./matcha
   ```

### Open Build Service

Also, a build of Matcha is available on [OBS](https://build.opensuse.org/package/show/home:mantarimay:apps/matcha). Thanks, [@mantarimay](https://github.com/mantarimay)!

## Features

### Email Management

- **📬 Inbox & Sent Mail**: View and manage emails from both inbox and sent folders
- **📧 Multi-Account Support**: Manage multiple email accounts with an elegant tabbed interface
- **⚡ Smart Caching**: Instant inbox display with background refresh for optimal performance
- **🔄 Real-time Refresh**: Manually refresh your inbox at any time with a single keypress
- **♾️ Infinite Scroll**: Automatically loads more emails as you scroll through your inbox
- **🔍 Search & Filter**: Built-in filtering to quickly find emails by subject, sender, or content
- **📖 Rich Email Viewing**:
  - HTML email rendering with proper formatting
  - Markdown support for plain-text emails
  - Styled headers and body text
  - Proper handling of quoted-printable encoding
- **💬 Reply to Emails**: Quick reply with automatic quoting of original message
- **🗑️ Delete & Archive**: Manage your inbox by deleting or archiving messages
- **📎 Attachment Support**:
  - Download email attachments to your Downloads folder
  - Automatic file opening after download
  - Smart filename handling (prevents overwrites with auto-numbering)
  - Support for various attachment encodings

### Rich Content Display

Matcha supports modern terminal image protocols for displaying images directly in your terminal:

#### **🖼️ Image Protocol Support**

- **Kitty Graphics Protocol**: Full support for Kitty, Ghostty, WezTerm, Wayst, and Konsole terminals
- **iTerm2 Inline Images**: Native support for iTerm2 and Warp terminals
- **Inline Email Images**: Display images embedded in HTML emails (including CID references)
- **Remote Image Fetching**: Automatically fetches and displays remote images from URLs
- **Data URI Support**: Renders base64-encoded inline images
- **Smart Fallback**: Gracefully falls back to clickable links when images aren't supported

#### **🔗 Terminal Hyperlinks (OSC 8)**

- **Clickable Links**: Full OSC 8 hyperlink support for modern terminals
- **Supported Terminals**: Kitty, Ghostty, WezTerm, Alacritty, iTerm2, Hyper, VS Code terminal, GNOME Terminal, and more
- **Smart Detection**: Automatically detects terminal capabilities
- **Fallback Mode**: Shows plain text URLs in unsupported terminals

### Composing Emails

- **✍️ Compose New Emails**: Clean, intuitive interface for writing emails
- **📝 Markdown Support**: Write emails in Markdown that automatically converts to HTML
- **🖼️ Inline Images**: Embed images in your emails using Markdown syntax `![alt](path/to/image.png)`
- **📎 File Attachments**: Attach files with an integrated file picker
- **👥 Contact Autocomplete**: Smart suggestions from your contact history
- **💾 Auto-save Drafts**: Never lose your work - drafts are automatically saved
- **📨 Multi-Account Sending**: Choose which account to send from with a simple picker
- **↩️ Reply Threading**: Proper email threading with In-Reply-To and References headers
- **🎨 Rich Formatting**: Send both plain text and HTML versions of your emails

### Draft Management

- **📝 Automatic Draft Saving**: Drafts are saved when you exit the composer
- **📂 Draft Library**: View all your saved drafts in one place
- **▶️ Resume Editing**: Pick up where you left off by reopening any draft
- **🗑️ Draft Cleanup**: Delete drafts you no longer need
- **⏰ Time Tracking**: See when each draft was last modified
- **🔍 Search Drafts**: Filter through your drafts by subject or recipient

### Account Management

- **👤 Multiple Accounts**: Configure and manage multiple email accounts
- **🔄 Easy Switching**: Switch between accounts with keyboard shortcuts or tabs
- **✉️ Provider Presets**: Built-in support for:
  - **Gmail** (imap.gmail.com / smtp.gmail.com)
  - **iCloud** (imap.mail.me.com / smtp.mail.me.com)
  - **Custom IMAP/SMTP**: Configure any email provider with custom server settings
- **⚙️ Account Settings**:
  - Add new accounts
  - Remove existing accounts
  - Edit account details
  - Configure separate fetch and send addresses
- **🔐 Secure Storage**: Credentials stored locally in `~/.config/matcha/config.json`

### Contact Management

- **📇 Automatic Contact Saving**: Email addresses are automatically saved from:
  - Emails you receive
  - Emails you send
- **🔍 Smart Search**: Fuzzy search through your contacts while composing
- **⚡ Quick Autocomplete**: Contact suggestions appear as you type in the "To" field
- **💾 Persistent Storage**: Contacts are saved locally for offline access

### User Interface

- **🎨 Beautiful TUI**: Clean, modern terminal interface built with Bubble Tea
- **⌨️ Vim-like Keybindings**: Efficient keyboard navigation (`j/k`, `h/l`, etc.)
- **📱 Responsive Design**: Adapts to your terminal window size
- **🎯 Focus Management**: Clear visual indication of focused elements
- **📑 Tabbed Interface**: Switch between accounts with tab navigation
- **🎭 Styled Elements**: Color-coded interface elements for better readability
- **💬 Contextual Help**: Built-in help text shows available commands for each screen

### Advanced Features

- **🔄 Automatic Updates**: Built-in update checker notifies you of new releases
- **⬆️ Self-Update Command**: Update Matcha with a simple `matcha update` command
- **🎯 Smart Image Rendering**: Automatically calculates terminal cell size for proper image display
- **🐛 Debug Mode**: Environment variables for debugging image protocol issues
- **🔧 Flexible Configuration**: JSON-based configuration with automatic migration from legacy formats
- **🚀 Performance Optimized**: Concurrent email fetching for faster inbox loading
- **💾 Email Caching**: Instant inbox display on startup with background refresh

## Supported Email Providers

Matcha works with any email provider that supports IMAP and SMTP. Here are the built-in presets:

| Provider | IMAP Server | SMTP Server | Notes |
|----------|-------------|-------------|-------|
| **Gmail** | imap.gmail.com:993 | smtp.gmail.com:587 | Requires app-specific password |
| **iCloud** | imap.mail.me.com:993 | smtp.mail.me.com:587 | Requires app-specific password |
| **Custom** | Your server | Your server | Configure any IMAP/SMTP provider |

### Using Gmail or iCloud

For Gmail and iCloud, you'll need to generate an **app-specific password**:

- **Gmail**: [Create an App Password](https://support.google.com/accounts/answer/185833)
- **iCloud**: [Generate an app-specific password](https://support.apple.com/en-us/HT204397)



## Usage

### First Launch

On first launch, Matcha will prompt you to configure an email account. You'll need:

- Your email address
- Your password (or app-specific password for Gmail/iCloud)
- Email provider (Gmail, iCloud, or Custom)

### Keyboard Shortcuts

#### Main Menu
- `↑/↓` or `j/k` - Navigate menu items
- `Enter` - Select option
- `Esc` - Go back / Exit
- `Ctrl+C` - Quit application

#### Inbox View
- `↑/↓` or `j/k` - Navigate emails
- `←/→` or `h/l` - Switch between account tabs
- `Enter` - Open selected email
- `/` - Filter/search emails
- `r` - Refresh inbox
- `d` - Delete selected email
- `a` - Archive selected email
- `Esc` - Back to main menu

#### Email View
- `↑/↓` or `j/k` - Scroll email content
- `r` - Reply to email
- `d` - Delete email
- `a` - Archive email
- `Tab` - Focus attachments
- `Esc` - Back to inbox

#### Attachment View (when focused)
- `↑/↓` or `j/k` - Navigate attachments
- `Enter` - Download and open attachment
- `Tab` or `Esc` - Back to email body

#### Composer
- `Tab` / `Shift+Tab` - Navigate fields
- `Enter` -
  - On "From" field: Select account (if multiple)
  - On "Attachment" field: Open file picker
  - On "Send" button: Send email
- `↑/↓` - Navigate contact suggestions (when typing in "To" field)
- `Esc` - Save draft and exit

#### Settings
- `↑/↓` or `j/k` - Navigate accounts
- `Enter` - Add new account
- `d` - Delete selected account
- `Esc` - Back to main menu

### Updating Matcha

Check for updates and install the latest version:

```bash
matcha update
```

This command will:
1. Check for the latest release on GitHub
2. Detect your installation method (Homebrew, Snap, or binary)
3. Update using the appropriate method

## Terminal Compatibility

### Image Protocol Support

For the best experience with inline images, use a terminal that supports modern image protocols:

**Kitty Graphics Protocol:**
- [Kitty](https://sw.kovidgoyal.net/kitty/)
- [Ghostty](https://ghostty.org/)
- [WezTerm](https://wezfurlong.org/wezterm/)
- [Wayst](https://github.com/91861/wayst)
- [Konsole](https://konsole.kde.org/)

**iTerm2 Inline Images:**
- [iTerm2](https://iterm2.com/)
- [Warp](https://www.warp.dev/)

### Hyperlink Support (OSC 8)

Clickable links work in:
- Kitty, Ghostty, WezTerm, Alacritty, Foot
- iTerm2, Hyper, Warp
- VS Code integrated terminal
- GNOME Terminal, Tilix (VTE-based terminals)
- tmux, screen (when properly configured)

## Configuration

> [!WARNING]
> The passwords are stored in plain text as of right now, make sure your computer is not infected before using Matcha

Configuration is stored in `~/.config/matcha/config.json`

**Example configuration:**

```json
{
  "accounts": [
    {
      "id": "unique-id-1",
      "name": "John Doe",
      "email": "john@gmail.com",
      "password": "app-specific-password",
      "service_provider": "gmail",
      "fetch_email": "john@gmail.com"
    },
    {
      "id": "unique-id-2",
      "name": "Work Email",
      "email": "john@company.com",
      "password": "password",
      "service_provider": "custom",
      "fetch_email": "john@company.com",
      "imap_server": "imap.company.com",
      "imap_port": 993,
      "smtp_server": "smtp.company.com",
      "smtp_port": 587
    }
  ]
}
```

### Additional Data Locations

- **Drafts**: `~/.config/matcha/drafts/`
- **Email Cache**: `~/.config/matcha/cache.json`
- **Contacts**: `~/.config/matcha/contacts.json`

## Debugging

### Image Protocol Debugging

If images aren't displaying correctly, enable debug logging:

```bash
export DEBUG_IMAGE_PROTOCOL=1
export DEBUG_IMAGE_PROTOCOL_LOG=/tmp/matcha-images.log
matcha
```

Check the log file for detailed information about image rendering.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is distributed under the MIT License. See the `LICENSE` file for more information.

## Credits

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Style definitions
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [go-imap](https://github.com/emersion/go-imap) - IMAP client
- [go-message](https://github.com/emersion/go-message) - Email parsing
- [Goldmark](https://github.com/yuin/goldmark) - Markdown rendering
- [goquery](https://github.com/PuerkitoBio/goquery) - HTML parsing

---

<div align="center">

**[Report Bug](https://github.com/floatpane/matcha/issues/new?template=bug_report.md)** · **[Request Feature](https://github.com/floatpane/matcha/issues/new?template=feature_request.md)** · **[Contributing Guidelines](https://github.com/floatpane/matcha/blob/master/CONTRIBUTING.md)**

</div>
