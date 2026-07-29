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

// The per-voice synthesis editor (PLAN.md G20): the modal opens from the strip,
// its knobs drive the engine, Escape is shared with the knobs' reset-to-default,
// audition must not start the transport, and edits must survive a reload
// through the versioned persistence blob.
test("voice editor opens, edits, auditions, and persists", async ({ page }) => {
  await page.goto("./");

  const play = page.getByRole("button", { name: "Play", exact: true });
  await expect(play).toBeEnabled({ timeout: 30_000 });

  await page.getByRole("button", { name: "Bass Drum voice settings" }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  // The title is uppercased in CSS but natural-case in the DOM.
  await expect(dialog).toHaveAccessibleName(/bass drum voice/i);

  const close = dialog.getByRole("button", { name: /^Close Bass Drum/ });
  await expect(close).toBeFocused();

  // Space inside the dialog must not reach the window transport handler. Click
  // the hint text first so focus is NOT on a button or slider — those are
  // already covered by the handler's original target check, and pressing Space
  // on the focused close button would just activate it.
  await dialog.locator(".dm-voice-hint").click();
  await page.keyboard.press("Space");
  await expect(play).toBeVisible();
  await expect(dialog).toBeVisible();

  // Escape on a moved knob resets it and keeps the dialog open, rather than
  // closing the dialog out from under the edit.
  const knob = dialog.getByRole("slider").nth(1);
  await knob.focus();
  const initial = await knob.getAttribute("aria-valuenow");
  await page.keyboard.press("ArrowUp");
  await expect(knob).not.toHaveAttribute("aria-valuenow", initial!);
  await page.keyboard.press("Escape");
  await expect(knob).toHaveAttribute("aria-valuenow", initial!);
  await expect(dialog).toBeVisible();

  // Auditioning a voice must not start the sequencer.
  await dialog.getByRole("button", { name: "Audition Bass Drum" }).click();
  await expect(play).toBeVisible();

  // Edit, let the 300 ms save debounce settle, then close.
  await knob.focus();
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("ArrowUp");
  const edited = await knob.getAttribute("aria-valuenow");
  expect(edited).not.toBe(initial);
  await page.waitForTimeout(500);

  await close.click();
  await expect(dialog).toBeHidden();

  // The edit must survive a reload through localStorage.
  await page.reload();
  await expect(play).toBeEnabled({ timeout: 30_000 });
  await page.getByRole("button", { name: "Bass Drum voice settings" }).click();
  await expect(
    page.getByRole("dialog").getByRole("slider").nth(1),
  ).toHaveAttribute("aria-valuenow", edited!);

  // RESET puts it back.
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Reset Bass Drum voice to defaults" })
    .click();
  await expect(
    page.getByRole("dialog").getByRole("slider").nth(1),
  ).toHaveAttribute("aria-valuenow", initial!);
});

// The only automated proof that the voice-parameter tail survives
// base64url + URL hash + decode in a real browser.
test("voice edits travel in a share link", async ({ page }) => {
  await page.goto("./");
  await expect(
    page.getByRole("button", { name: "Play", exact: true }),
  ).toBeEnabled({ timeout: 30_000 });

  await page.getByRole("button", { name: "Snare voice settings" }).click();

  const dialog = page.getByRole("dialog");
  const knob = dialog.getByRole("slider").first();
  await knob.focus();
  for (let i = 0; i < 5; i++) await page.keyboard.press("ArrowUp");
  const edited = await knob.getAttribute("aria-valuenow");
  await dialog.getByRole("button", { name: /^Close Snare/ }).click();

  await page.getByRole("button", { name: /Copy shareable link/i }).click();
  const shared = page.url();
  expect(shared).toContain("#");

  // Clear localStorage first: loadInitialState prefers the hash, but with the
  // same-origin autosave still present a passing assertion would not prove the
  // hash carried anything.
  await page.evaluate(() => localStorage.clear());
  await page.goto(shared);
  await expect(
    page.getByRole("button", { name: "Play", exact: true }),
  ).toBeEnabled({ timeout: 30_000 });

  await page.getByRole("button", { name: "Snare voice settings" }).click();
  await expect(
    page.getByRole("dialog").getByRole("slider").first(),
  ).toHaveAttribute("aria-valuenow", edited!);
});

test("Tom can select, audition, and persist the physical model", async ({
  page,
}) => {
  await page.goto("./");

  const play = page.getByRole("button", { name: "Play", exact: true });
  await expect(play).toBeEnabled({ timeout: 30_000 });
  await page.getByRole("button", { name: "Tom voice settings" }).click();

  const dialog = page.getByRole("dialog");
  const algorithmic = dialog.getByRole("radio", { name: "Algorithmic" });
  const physical = dialog.getByRole("radio", { name: /Physical/ });
  await expect(algorithmic).toBeChecked();

  // The native radio is visually hidden; click its visible label just as a
  // user does instead of asking Playwright to click the transparent input.
  await dialog.getByText("Physical", { exact: false }).click();
  await expect(physical).toBeChecked();
  await expect(dialog.getByText("Double-headed physical drum")).toBeVisible();
  await expect(dialog.getByRole("slider")).toHaveCount(13);

  const tuning = dialog.getByRole("slider", {
    name: "Tom batter head tension",
  });
  await tuning.focus();
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("ArrowUp");
  const edited = await tuning.getAttribute("aria-valuenow");

  const audition = dialog.getByRole("button", { name: "Audition Tom" });
  await audition.click({ position: { x: 4, y: 8 } });
  const box = await audition.boundingBox();
  expect(box).not.toBeNull();
  await audition.click({ position: { x: box!.width - 4, y: 8 } });
  await expect(play).toBeVisible();

  await page.waitForTimeout(500);
  await dialog.getByRole("button", { name: /^Close Tom/ }).click();
  await page.reload();
  await expect(play).toBeEnabled({ timeout: 30_000 });
  await page.getByRole("button", { name: "Tom voice settings" }).click();
  await expect(
    page.getByRole("dialog").getByRole("radio", { name: /Physical/ }),
  ).toBeChecked();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("slider", { name: "Tom batter head tension" }),
  ).toHaveAttribute("aria-valuenow", edited!);

  // The physical bank is also part of the URL payload, not just localStorage.
  await page
    .getByRole("dialog")
    .getByRole("button", { name: /^Close Tom/ })
    .click();
  await page.getByRole("button", { name: /Copy shareable link/i }).click();
  const shared = page.url();
  await page.evaluate(() => localStorage.clear());
  await page.goto(shared);
  await expect(play).toBeEnabled({ timeout: 30_000 });
  await page.getByRole("button", { name: "Tom voice settings" }).click();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("slider", { name: "Tom batter head tension" }),
  ).toHaveAttribute("aria-valuenow", edited!);
});
