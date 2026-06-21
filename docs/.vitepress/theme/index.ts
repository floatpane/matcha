import type { Theme } from "vitepress";
import DefaultTheme from "vitepress/theme";
import "../../styles/custom.css";
import Marketplace from "../../components/marketplace.vue";

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component("Marketplace", Marketplace);
  },
} satisfies Theme;
