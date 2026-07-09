import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Dev mode: React dev server on :3000, API on :4545 (§13).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 3000,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:4545",
        ws: true,
      },
    },
  },
  build: {
    outDir: "dist",
  },
});
