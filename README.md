<div align="center">

---

<img src = "assets/banner.jpg">

---

[![Go Version](https://img.shields.io/github/go-mod/go-version/floatpane/matcha)](https://golang.org)
[![Discord](https://img.shields.io/discord/1489296626827661323?logo=discord)](https://discord.gg/jVnYTeSPV8)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/floatpane/matcha)](https://github.com/floatpane/matcha/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/floatpane/matcha.svg)](https://pkg.go.dev/github.com/floatpane/matcha)
[![Awesome](https://awesome.re/badge.svg)](https://github.com/rothgar/awesome-tuis#messaging)

<a href="https://trendshift.io/repositories/26026" target="_blank"><img src="https://trendshift.io/api/badge/repositories/26026" alt="floatpane%2Fmatcha | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>


</div> 

> [!TIP]
> There are [nightly releases](https://github.com/floatpane/matcha/releases/tag/nightlyv0)!

**A powerful, feature-rich email client for your terminal.** Built with Go and the Bubble Tea TUI framework, Matcha brings a beautiful, modern email experience to the command line with support for rich content, multiple accounts, and advanced terminal features.

![Demo GIF](public/assets/demo.gif)

### Plugin Marketplace

Matcha has a built-in plugin system with 35+ community plugins. Browse and install them from the terminal or the [online marketplace](https://docs.matcha.email/marketplace).

```bash
matcha marketplace                # browse plugins in the TUI
matcha install <url_or_file>      # install a plugin
matcha config <plugin_name>       # configure an installed plugin
```

Anyone can submit their own plugin — just add an entry to `plugins/registry.json` and open a PR. [Learn more](https://docs.matcha.email/Features/Plugins#submit-your-plugin)

### AI Integration

**AI Agent Support:** Matcha can be used by autonomous AI agents to send emails on your behalf. The `matcha send` CLI command provides a non-interactive interface for composing and sending emails.

```bash
matcha send --to alice@example.com --subject "Hello" --body "Sent by my AI agent"
```

[Learn more](https://docs.matcha.email/Features/AI_AGENTS)

### Logging

Matcha supports global logging verbosity flags before the main command or subcommand:

```bash
matcha --verbose              # enable verbose logging
matcha -V daemon status       # short form for verbose logging
matcha --debug daemon status  # enable debug logging
```

The existing `-v` and `--version` flags continue to print the Matcha version.

**AI Rewrite Plugin:** Matcha includes an AI rewrite plugin that allows you to rewrite your email drafts using OpenAI, Ollama, Gemini, or Claude.

[Setup Guide](https://docs.matcha.email/setup-guides/ai-rewrite)

### macOS Menu Bar Helper

On macOS, Matcha ships with an optional menu bar helper. It lives in the menu bar
and shows your **unread count**, posts native **notifications with the Matcha icon**
on new mail, and lists your accounts so you can **open Matcha** in a terminal with a
click. It talks to the Matcha daemon, so no extra configuration is needed.

```bash
matcha helper install     # build & start the menu bar app (runs at login)
matcha helper status      # check whether it's installed and the daemon is up
matcha helper uninstall   # remove it
```

Building the helper uses the Xcode Command Line Tools' `swiftc` (the same requirement
as `matcha setup-mailto`).

## Documentation

Matcha Documention is available on [our website](https://docs.matcha.email)

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=floatpane/matcha&type=date&legend=top-left)](https://www.star-history.com/#floatpane/matcha&type=date&legend=top-left)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is distributed under the MIT License. See the `LICENSE` file for more information.

## Suggestions

For general suggestions and community discussion, please join our [Discord server](https://discord.gg/jVnYTeSPV8).

For security-related issues, please refer to the [Security Policy](https://github.com/floatpane/matcha/blob/master/SECURITY.md).

For urgent concerns, contact [support@floatpane.com](mailto:support@floatpane.com)

## Sponsors
>[!TIP]
> Want to sponsor our development and be featured here? You can do so [here](https://github.com/sponsors/floatpane) (or, if you prefer, [here](https://opencollective.com/floatpane)), or discuss it via email with [our team](mailto:us@floatpane.com)


Thank you to our sponsors for supporting Matcha's development!

### Individual Sponsors:

[David H. Colmenares](https://github.com/hipomenes) | Elliot Hornes | Robert M. | James L. | Chris D. 

<div align="center">

**[Report Bug](https://github.com/floatpane/matcha/issues/new?template=bug_report.md)** · **[Request Feature](https://github.com/floatpane/matcha/issues/new?template=feature_request.md)** · **[Contributing Guidelines](https://github.com/floatpane/matcha/blob/master/CONTRIBUTING.md)**

</div>
