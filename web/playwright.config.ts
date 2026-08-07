import { existsSync } from "node:fs";
import { defineConfig, devices } from "@playwright/test";

// The dev container ships a Chromium under PLAYWRIGHT_BROWSERS_PATH whose
// revision may not match the pinned @playwright/test. When that binary is
// present, drive it directly via executablePath; in CI (where
// `playwright install chromium` provisions the matching build) fall back to
// Playwright's managed browser.
const LOCAL_CHROMIUM = "/opt/pw-browsers/chromium";
const executablePath = existsSync(LOCAL_CHROMIUM) ? LOCAL_CHROMIUM : undefined;

// Allow concurrent worktrees/projects to choose another preview port while CI
// and the normal local command retain the documented default.
const PORT = Number(process.env.PLAYWRIGHT_PORT ?? 4173);
const BASE_URL = `http://localhost:${PORT}/algo-drum/`;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: "list",
  timeout: 60_000,
  use: {
    baseURL: BASE_URL,
    trace: "on-first-retry",
    launchOptions: {
      executablePath,
      // Let the AudioContext start without a user gesture so the sequencer
      // (and its step/playhead messages) run under headless Chromium.
      args: ["--autoplay-policy=no-user-gesture-required"],
    },
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], executablePath },
    },
  ],
  // Build the WASM engine + a production bundle, then serve it with
  // `vite preview`. Worker/worklet bundling differs dev vs prod, so the smoke
  // test deliberately runs against the built output.
  //
  // The rebuild is unconditional: audioWorker.ts checks REQUIRED_METHODS at
  // load and refuses to start against a .wasm older than the bundle, so a
  // "build only if missing" shortcut made every dev machine with a stale
  // artifact fail the suite with a rebuild-me error instead of running it.
  webServer: {
    command: `sh -c 'bash ../scripts/build-wasm.sh && bunx vite build && bunx vite preview --port ${PORT} --strictPort'`,
    url: BASE_URL,
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
  },
});
