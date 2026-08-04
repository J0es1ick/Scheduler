import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../../internal/siteui/dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    port: 15174,
    proxy: {
      "/api": "http://localhost:18081",
    },
  },
});
