import { defineConfig } from "vitest/config";

// Pure modules and worker shims stay in Node so browser globals cannot mask a
// missing fake. React components get their own jsdom project and setup file.
// Playwright specs live under e2e/ and are run separately against production.
export default defineConfig({
  test: {
    projects: [
      {
        test: {
          name: "node",
          include: ["src/**/*.test.ts"],
          exclude: ["src/**/*.component.test.tsx"],
          environment: "node",
        },
      },
      {
        test: {
          name: "components",
          include: ["src/**/*.component.test.tsx"],
          environment: "jsdom",
          setupFiles: ["src/test/setup.ts"],
        },
      },
    ],
    coverage: {
      provider: "v8",
      include: ["src/**/*.{ts,tsx}", "public/worklet.js"],
      exclude: [
        "src/**/*.d.ts",
        "src/**/*.test.{ts,tsx}",
        "src/engine/voiceParams.generated.ts",
      ],
      reporter: ["text", "json-summary", "html"],
      reportsDirectory: "coverage",
      thresholds: {
        statements: 80,
        branches: 75,
        functions: 80,
        lines: 80,
      },
    },
  },
});
