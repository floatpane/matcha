# docs

Documentation website built with [Docusaurus](https://docusaurus.io/). Contains user-facing guides, feature documentation, and a blog.

## Architecture

A Docusaurus 3 project that generates a static documentation site. The content is organized into guides (installation, usage, configuration) and feature-specific pages.

## Files

| File | Description |
|------|-------------|
| `docusaurus.config.ts` | Main Docusaurus configuration (site metadata, theme, plugins). |
| `sidebars.ts` | Documentation sidebar navigation structure. |
| `package.json` | Node.js dependencies for building the docs site. |
| `tsconfig.json` | TypeScript configuration. |

### docs/

Documentation content pages:

| File | Description |
|------|-------------|
| `index.md` | Documentation home page. |
| `installation.md` | Installation guide for all platforms. |
| `usage.md` | Usage guide and keyboard shortcuts. |
| `Configuration.md` | Configuration file reference. |

### docs/Features/

Feature-specific documentation:

| File | Description |
|------|-------------|
| `ACCOUNTS.md` | Multi-account setup and management. |
| `ADVANCED.md` | Advanced features (S/MIME, custom servers). |
| `COMPOSING.md` | Email composition guide. |
| `CONTACTS.md` | Contact management and autocomplete. |
| `DRAFTS.md` | Draft saving and restoration. |
| `EMAIL_MANAGEMENT.md` | Email actions (delete, archive, move). |
| `Hyperlinks.md` | Terminal hyperlink support. |
| `Images.md` | Inline image rendering and protocols. |
| `Themes.md` | Theme customization and custom themes. |
| `UI.md` | General UI documentation. |

### docs/assets/

Screenshot images used in the documentation pages.

### src/

Custom React components and CSS for the documentation site theme.

### static/

Static assets (logos, images) served directly by the documentation site.
