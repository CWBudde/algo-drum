import { defineConfig } from "vitest/config";

// Unit tests only. The pure algo/ and knob-math modules need no DOM, so the
// default Node environment is used. Playwright e2e specs live under e2e/ and
// are excluded here (they are run by `test:e2e`, not Vitest).
export default defineConfig({
  test: {
    include: ["src/**/*.test.{ts,tsx}"],
    environment: "node",
  },
});
