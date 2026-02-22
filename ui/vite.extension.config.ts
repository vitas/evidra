import { defineConfig } from "vite";

export default defineConfig({
  build: {
    outDir: "dist-argocd-extension",
    emptyOutDir: true,
    lib: {
      entry: "src/argocd-extension.js",
      formats: ["iife"],
      name: "EvidraArgoExtension",
      fileName: () => "evidra-argocd-extension.js",
    },
    rollupOptions: {
      external: ["react", "react-dom"],
    },
  },
});
