import { defineConfig } from "vitepress";
import { withMermaid } from "vitepress-plugin-mermaid";

export default withMermaid(
  defineConfig({
  title: "Matcha",
  description:
    "A modern, beautiful, and feature-rich email client for the terminal.",
  lang: "en-US",

  lastUpdated: true,
  cleanUrls: true,

  head: [
    ["link", { rel: "icon", href: "/favicon.ico" }],
    ["link", { rel: "icon", type: "image/png", href: "/logo.png" }],
    ["meta", { property: "og:title", content: "Matcha" }],
    ["meta", { property: "og:description", content: "A modern, beautiful, and feature-rich email client for the terminal." }],
    ["meta", { property: "og:image", content: "https://docs.matcha.email/og-image.png" }],
    ["meta", { property: "og:image:width", content: "1200" }],
    ["meta", { property: "og:image:height", content: "630" }],
    ["meta", { property: "twitter:card", content: "summary_large_image" }],
    ["meta", { property: "twitter:image", content: "https://docs.matcha.email/og-image.png" }],
  ],

  srcDir: "docs",
  outDir: ".vitepress/dist",
  assetsDir: "assets",
  publicDir: "../public",

  vite: {
    publicDir: "../public",
  },

  markdown: {
    lineNumbers: true,
    config: (md) => {
      md.set({
        html: true,
        xhtmlOut: true,
        linkify: true,
      });
    },
  },

  themeConfig: {
    logo: { src: "/logo.png", alt: "Matcha" },
    siteTitle: false,

    search: {
      provider: "local",
    },

    nav: [
      { text: "Docs", link: "/" },
      { text: "Marketplace", link: "/marketplace" },
    ],

    sidebar: [
      {
        text: "Get Started",
        items: [
          { text: "Welcome", link: "/" },
          { text: "Installation", link: "/installation" },
          { text: "Usage", link: "/usage" },
          { text: "Configuration", link: "/Configuration" },
          { text: "Localization", link: "/localization" },
        ],
      },
      {
        text: "Setup Guides",
        items: [
          { text: "Gmail", link: "/setup-guides/gmail" },
          { text: "iCloud", link: "/setup-guides/icloud" },
          { text: "Outlook", link: "/setup-guides/outlook" },
          { text: "AI Rewrite", link: "/setup-guides/ai-rewrite" },
        ],
      },
      {
        text: "Features",
        items: [
          { text: "Accounts", link: "/Features/ACCOUNTS" },
          { text: "Composing", link: "/Features/COMPOSING" },
          { text: "Contacts", link: "/Features/CONTACTS" },
          { text: "Drafts", link: "/Features/DRAFTS" },
          { text: "Email Management", link: "/Features/EMAIL_MANAGEMENT" },
          { text: "UI", link: "/Features/UI" },
          { text: "Themes", link: "/Features/Themes" },
          { text: "Keybinds", link: "/Features/Keybinds" },
          { text: "CLI", link: "/Features/CLI" },
          { text: "AI Agents", link: "/Features/AI_AGENTS" },
          { text: "Plugins", link: "/Features/Plugins" },
          { text: "PGP", link: "/Features/PGP" },
          { text: "Encryption", link: "/Features/Encryption" },
          { text: "S/MIME", link: "/Features/SMIME" },
          { text: "Spellcheck", link: "/Features/Spellcheck" },
          { text: "Calendar", link: "/Features/CALENDAR" },
          { text: "Daemon", link: "/Features/DAEMON" },
          { text: "Split View", link: "/Features/SPLIT_VIEW" },
          { text: "Threaded View", link: "/Features/THREADED_VIEW" },
          { text: "Images", link: "/Features/Images" },
          { text: "Hyperlinks", link: "/Features/Hyperlinks" },
          { text: "Advanced", link: "/Features/ADVANCED" },
          { text: "PassCmd", link: "/Features/PassCmd" },
        ],
      },
      {
        text: "For Developers",
        items: [
          { text: "Code Blocks", link: "/for-developers/code-blocks" },
          { text: "Patch Email Support", link: "/for-developers/patch-email" },
        ],
      },
      {
        text: "Community",
        items: [
          { text: "Marketplace", link: "/marketplace" },
          { text: "GitHub", link: "https://github.com/floatpane/matcha" },
          { text: "Discord", link: "https://discord.gg/RxNrJgfatk" },
          { text: "Mastodon", link: "https://fosstodon.org/@floatpane" },
        ],
      },
    ],

    socialLinks: [
      { icon: "github", link: "https://github.com/floatpane/matcha" },
      { icon: "discord", link: "https://discord.gg/RxNrJgfatk" },
    ],

    footer: {
      message: "Released under the MIT License.",
      copyright: `Copyright © ${new Date().getFullYear()} Floatpane`,
    },

    editLink: {
      pattern: "https://github.com/floatpane/matcha/edit/master/docs/:path",
      text: "Edit this page on GitHub",
    },
  },
  mermaid: {
    theme: "dark",
  },
})
);
