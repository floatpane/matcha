# Plugin Marketplace

The Matcha Plugin Marketplace is now live at **[marketplace.matcha.email](https://marketplace.matcha.email)**!

## Features

- **Browse Plugins**: Discover community-built plugins to enhance your Matcha experience
- **Submit Plugins**: Share your own plugins with the community
- **Security Verification**: All plugins undergo automated virus scanning and manual review
- **SHA256 Verification**: Ensures plugin integrity and prevents tampering
- **Version Control**: Track updates and manage plugin versions
- **Trusted Authors**: Verified maintainers get automatic approval and special badges

## Installing Plugins

### From the Marketplace Website

1. Visit [marketplace.matcha.email](https://marketplace.matcha.email)
2. Browse or search for plugins
3. Click "Install" on any plugin
4. The plugin will be downloaded and installed automatically

### From the CLI

```bash
matcha install <plugin_name>
```

### From the TUI

1. Open Matcha
2. Navigate to the Marketplace from the main menu
3. Use arrow keys to browse plugins
4. Press `Enter` to install a plugin

## Security & Trust

### Verification Process

All plugins submitted to the marketplace go through a rigorous verification process:

1. **Automated Virus Scanning**: Checks for malicious patterns like `os.execute`, `io.popen`, and `debug.*`
2. **SHA256 Verification**: Ensures the downloaded code matches the submitted hash
3. **Manual Review**: Suspicious plugins are flagged for manual review by the Matcha team
4. **Author Verification**: Trusted GitHub accounts receive verified badges

### Trust Indicators

When installing plugins, you'll see:

- ✅ **Verified Safe**: Plugin has passed all security checks
- ⚠️ **Unverified**: Plugin is from an untrusted source - confirmation required
- 🔒 **Verified Author**: Author is a trusted maintainer (no confirmation needed)

### Untrusted Sources

When installing from unverified sources, Matcha will:

1. Show a warning about the unverified author
2. Display the author's GitHub profile link
3. Ask for explicit confirmation before installing
4. Verify SHA256 hash and warn if mismatched

## Submitting Plugins

Want to share your plugin? Visit the [Submit Plugin](https://marketplace.matcha.email/submit) page.

### Requirements

- Plugin must be written in Lua
- Must include a valid SHA256 hash
- Repository URL must be public (GitHub/GitLab)
- Follow Matcha plugin API guidelines

### Submission Process

1. Fill out the submission form with plugin details
2. Provide the plugin code and SHA256 hash
3. Submit for review
4. Wait for automated scanning and manual approval
5. Once approved, your plugin will be available to all users

## For Developers`

### Trusted Maintainer Program

To become a verified/trusted maintainer:

1. Contribute high-quality plugins to the marketplace
2. Maintain your plugins with regular updates
3. Follow security best practices
4. Apply for verified status through GitHub

Verified maintainers enjoy:
- Automatic plugin approval (no manual review)
- Special badge on marketplace
- No confirmation prompts when users install their plugins

## Migration from Old Registry

The old `registry.json` system is being phased out. The new marketplace provides:

- Better security with automated scanning
- Version tracking and updates
- Author verification and trust badges
- Rich metadata and search capabilities
- Community submissions

Old plugins from `registry.json` will continue to work, but we encourage migrating to the new marketplace.
