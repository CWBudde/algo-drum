import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

// `base` defaults to "/algo-drum/" (this repo's GitHub Pages path) but can be
// overridden via the VITE_BASE env var so forks/renames don't need a source
// change — the deploy workflow sets it from `github.event.repository.name`.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "VITE_");

  return {
    plugins: [react()],
    base: env.VITE_BASE || "/algo-drum/",
    // Cross-Origin-Opener-Policy / Cross-Origin-Embedder-Policy headers were
    // previously set here for the dev server only, but nothing in the app
    // needs SharedArrayBuffer today, and GitHub Pages can't send custom
    // response headers anyway. A future ring-buffer audio design (AGENTS.md
    // B1) would need both headers plus the coi-serviceworker workaround on
    // Pages to actually cross-origin-isolate the deployed site.
  };
});
