import { expect, test } from "@playwright/test";

// End-to-end smoke test against the production build: the page loads, the WASM
// engine reports ready, a grid cell toggles, Play starts the playhead, and
// Space toggles the transport. Audio output can't be asserted headlessly, so
// engine/UI state stands in for "it's running".
test("loads, plays, and responds to input", async ({ page }) => {
  await page.goto("./");

  // WASM load is the slow part — give it room. Readiness = Play enabled and
  // the "Loading engine" text gone.
  const play = page.getByRole("button", { name: "Play", exact: true });
  await expect(play).toBeEnabled({ timeout: 30_000 });
  await expect(page.getByText("Loading engine")).toHaveCount(0);

  // Toggle a grid cell (mouse click also blurs it, freeing Space for the
  // transport). aria-pressed must flip off → on.
  const cell = page.getByRole("button", { name: /^Bass step 1:/ });
  await expect(cell).toHaveAttribute("aria-pressed", "false");
  await cell.click();
  await expect(cell).toHaveAttribute("aria-pressed", "true");

  // Space starts playback (focus is on the body after the cell click blur).
  await page.keyboard.press("Space");
  const stop = page.getByRole("button", { name: "Stop", exact: true });
  await expect(stop).toBeVisible({ timeout: 10_000 });

  // The playhead should land on a column within a couple of steps.
  await expect(page.locator(".dm-cell[data-playhead]")).not.toHaveCount(0, {
    timeout: 6_000,
  });

  // Space again stops playback and clears the playhead.
  await page.keyboard.press("Space");
  await expect(play).toBeVisible({ timeout: 10_000 });
  await expect(page.locator(".dm-cell[data-playhead]")).toHaveCount(0, {
    timeout: 4_000,
  });
});

// The engine owns the pattern: every edit is echoed back as the engine's
// authoritative copy and reconciled into the UI mirror. Cycle a cell through
// all three velocity states and check each one still holds after the echo
// round-trip has had time to land (a bad echo would revert the cell).
test("cell edits survive the engine's authoritative pattern echo", async ({
  page,
}) => {
  await page.goto("./");
  const play = page.getByRole("button", { name: "Play", exact: true });
  await expect(play).toBeEnabled({ timeout: 30_000 });

  const cell = page.getByRole("button", { name: /^Snare step 5:/ });
  await expect(cell).toHaveAccessibleName("Snare step 5: off");

  for (const state of ["on", "accent", "off"]) {
    await cell.click();
    await page.waitForTimeout(150); // worker echo round-trip
    await expect(cell).toHaveAccessibleName(`Snare step 5: ${state}`);
  }
});
