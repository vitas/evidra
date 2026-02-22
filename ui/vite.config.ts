import { defineConfig } from "vite";
import pkg from "./package.json";

export default defineConfig({
  base: "/ui/",
  define: {
    "import.meta.env.VITE_UI_BUILD": JSON.stringify(`v${pkg.version}`),
  },
});
