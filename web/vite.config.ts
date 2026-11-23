import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vitest/config";

// The API is served on the same origin (dev: this proxy, prod: the ingress),
// so the browser never needs CORS and cookies/tokens stay first-party.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: "localhost",
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: false },
    },
  },
  // `vite preview` serves the production build for E2E tests, same-origin like prod.
  preview: {
    host: "localhost",
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: false },
    },
  },
  build: {
    sourcemap: "hidden",
    target: "es2023",
    rolldownOptions: {
      output: {
        // Libraries every page needs go into one long-cached vendor chunk, so
        // a release that only changes app code does not re-download them.
        // Libraries of lazy pages (dnd-kit for the board) stay with those pages.
        codeSplitting: {
          groups: [{ name: "vendor", test: /[\\/]node_modules[\\/](react|react-dom|scheduler|react-router|@tanstack|oidc-client-ts|jwt-decode)[\\/]/ }],
        },
      },
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
  },
});
