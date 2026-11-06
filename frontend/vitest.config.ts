import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

const src = fileURLToPath(new URL("./src", import.meta.url));

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: [
      { find: "server-only", replacement: `${src}/tests/server-only.ts` },
      { find: /^@\//, replacement: `${src}/` },
    ],
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    css: false,
    coverage: {
      provider: "v8",
      include: ["src/**/*.{ts,tsx}"],
      exclude: [
        "src/**/*.test.*",
        "src/tests/**",
        "src/domain/trip/types.ts",
        "src/app/layout.tsx",
      ],
    },
  },
});
