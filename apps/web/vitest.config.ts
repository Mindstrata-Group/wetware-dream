import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  css: {
    // Unit/component tests do not assert generated CSS. Supplying an explicit
    // empty PostCSS config prevents Vite from loading Tailwind's native
    // optional binding on older CI Node/npm combinations.
    postcss: { plugins: [] },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    exclude: ["node_modules", "e2e", ".next"],
    fileParallelism: true,
    pool: "threads",
    poolOptions: {
      threads: {
        maxThreads: "100%",
      },
    },
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      reportsDirectory: "coverage",
      include: [
        "src/lib/api.ts",
        "src/lib/useApi.ts",
        "src/lib/useAutoSave.tsx",
        "src/app/home-helpers.ts",
        "src/app/_home/_dots/{interaction,physics,render}.ts",
        "src/app/tester/_parts/{CheckRow,HScrollTabs,StatusIcon,buildChecksFromStatus,computeStats}.ts*",
        "src/app/access/_parts/_utils.ts",
        "src/app/chat/{storage,utils}.ts",
        "src/app/profile/_parts/_utils.ts",
        "src/app/register/_parts/oauth.ts",
      ],
      thresholds: {
        statements: 85,
        branches: 70,
        functions: 85,
        lines: 85,
      },
    },
  },
});
