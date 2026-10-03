/// <reference types="vitest/config" />
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig(({ mode }) => ({
  plugins: [react(), tailwindcss()],
  server: {
    // Kratos ui_urls and CORS allow exactly http://localhost:5173.
    host: "localhost",
    port: 5173,
    strictPort: true,
  },
  preview: {
    host: "localhost",
    port: 5173,
    strictPort: true,
  },
  build: {
    // No public source maps in production builds.
    sourcemap: mode !== "production",
    rollupOptions: {
      output: {
        manualChunks: {
          react: ["react", "react-dom", "react-router"],
          ory: ["@ory/client-fetch"],
          vendor: [
            "@tanstack/react-query",
            "i18next",
            "react-i18next",
            "openapi-fetch",
            "react-hook-form",
            "zod",
          ],
        },
      },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    css: false,
    env: {
      VITE_KRATOS_PUBLIC_URL: "http://kratos.test",
      VITE_API_URL: "http://api.test",
    },
  },
}));
