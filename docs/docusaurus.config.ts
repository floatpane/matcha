import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";

const config: Config = {
  title: "Matcha",
  tagline:
    "A modern, beautiful, and feature-rich email client for the terminal.",
  favicon: "img/favicon.ico",

  url: "https://docs.matcha.floatpane.com",
  baseUrl: "/",

  organizationName: "floatpane",
  projectName: "matcha",

  onBrokenLinks: "warn",

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  presets: [
    [
      "classic",
      {
        docs: {
          sidebarPath: "./sidebars.ts",
          routeBasePath: "/",
          editUrl: "https://github.com/floatpane/matcha/tree/master/docs/",
        },
        blog: false,
        theme: {
          customCss: "./src/css/custom.css",
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    image: "img/logo.png",
    colorMode: {
      respectPrefersColorScheme: true,
      defaultMode: "dark",
    },
    navbar: {
      title: "Matcha",
      logo: {
        alt: "Matcha Logo",
        src: "img/logo.png", // We will update this later or rely on the text
      },
      items: [
        {
          to: "/marketplace",
          label: "Marketplace",
          position: "left",
        },
        {
          href: "https://github.com/floatpane/matcha",
          label: "GitHub",
          position: "right",
        },
      ],
    },
    footer: {
      style: "dark",
      links: [
        {
          title: "Docs",
          items: [
            {
              label: "Installation",
              to: "/installation",
            },
            {
              label: "Usage",
              to: "/usage",
            },
          ],
        },
        {
          title: "Community",
          items: [
            {
              label: "GitHub",
              href: "https://github.com/floatpane/matcha",
            },
            {
              label: "Mastodon",
              href: "https://fosstodon.org/@floatpane",
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Floatpane.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
