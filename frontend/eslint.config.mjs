import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

export default defineConfig([
  ...nextVitals,
  ...nextTs,
  globalIgnores([".next/**", "coverage/**", "node_modules/**", "next-env.d.ts"]),
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: [
      "src/server/trips/datasource.ts",
      "src/server/trips/mock/**",
      "src/tests/**",
      "**/*.test.*",
    ],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["**/mock/*", "**/mock"],
              message: "Only server/trips/datasource.ts imports the mock.",
            },
          ],
        },
      ],
    },
  },
]);
