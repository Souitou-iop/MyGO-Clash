import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: {
    // The app loads this address during `mygo dev` (devUrl in mygo.json).
    port: 5173,
    strictPort: true,
    watch: { ignored: ["**/.mygo/**", "**/build/**", "**/internal/**"] },
  },
  build: {
    target: "es2022",
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks: (id) => (id.includes("@codemirror") || id.includes("@lezer") ? "editor" : undefined),
      },
    },
  },
});
