import { expect, test } from "@playwright/test";

import { PHYSICAL_TOM_PARAMS } from "../src/engine/voiceParams.generated";

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
  await expect(page.locator(".dm-track-label")).toHaveCount(7);
  await expect(
    page.getByRole("button", { name: "Tom 2 voice settings" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Percussion voice settings" }),
  ).toBeVisible();

  // Toggle a grid cell (mouse click also blurs it, freeing Space for the
  // transport). Use the highest new engine track so the seven-track WASM
  // bridge is covered; aria-pressed must flip off → on.
  const cell = page.getByRole("button", { name: /^Perc step 1:/ });
  await expect(cell).toHaveAttribute("aria-pressed", "false");
  await cell.click();
  await expect(cell).toHaveAttribute("aria-pressed", "true");

  // Space starts playback (focus is on the body after the cell click blur).
  await page.keyboard.press("Space");
  const pause = page.getByRole("button", { name: "Pause", exact: true });
  const stop = page.getByRole("button", { name: "Stop", exact: true });
  await expect(pause).toBeVisible({ timeout: 10_000 });

  // The playhead should land on a column within a couple of steps.
  await expect(page.locator(".dm-cell[data-playhead]")).not.toHaveCount(0, {
    timeout: 6_000,
  });

  // Space again pauses at the held step; Stop is the distinct reset action
  // that returns to step 1 and clears the playhead.
  await page.keyboard.press("Space");
  await expect(play).toBeVisible({ timeout: 10_000 });
  await expect(page.locator(".dm-cell[data-playhead]")).not.toHaveCount(0);
  await stop.click();
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

test.describe("engine load retry", () => {
  // The production service worker precaches algo_drum.wasm and can issue a
  // second, unrelated request while this test is controlling the worker's
  // streaming/fallback pair. Blocking it keeps the failure deterministic.
  test.use({ serviceWorkers: "block" });

  test("retains in-memory machine state across a failed load and retry", async ({
    page,
  }) => {
    let releaseFirstRequest!: () => void;
    let reportFirstRequest!: () => void;
    const firstRequestHeld = new Promise<void>((resolve) => {
      releaseFirstRequest = resolve;
    });
    const firstRequestSeen = new Promise<void>((resolve) => {
      reportFirstRequest = resolve;
    });
    let failAttempt = true;
    let failedRequests = 0;

    await page.route("**/algo_drum.wasm", async (route) => {
      if (!failAttempt) {
        await route.continue();
        return;
      }

      failedRequests++;
      if (failedRequests === 1) {
        reportFirstRequest();
        await firstRequestHeld;
      }

      await route.abort("failed");
    });

    await page.goto("./");
    await firstRequestSeen;

    // The machine mounts while the engine loads, so an edit can exist only in
    // React and the bridge's pending command queue when the worker fails.
    const shell = page.locator(".app-machine");
    const cell = page.getByRole("button", { name: /^Cymbal step 16:/ });
    await expect(shell).toBeVisible();
    await expect(cell).toHaveAccessibleName("Cymbal step 16: off");
    await cell.click();
    await expect(cell).toHaveAccessibleName("Cymbal step 16: on");

    // A DOM sentinel proves Retry kept the same mounted subtree rather than
    // reconstructing an equivalent-looking machine from persistence.
    await shell.evaluate((element) => {
      element.setAttribute("data-retry-sentinel", "retained");
    });

    // Abort the held streaming request. The worker retries with arrayBuffer,
    // and the route aborts that fallback too before App enters its error UI.
    releaseFirstRequest();
    const fault = page.locator(".app-fault");
    await expect(fault).toBeVisible({ timeout: 30_000 });
    await expect(
      fault.getByRole("heading", { name: "Audio engine failed to load" }),
    ).toBeVisible();
    expect(failedRequests).toBeGreaterThanOrEqual(2);

    // The failed machine is unavailable to users but remains mounted, keeping
    // reducer state and the bridge's queued edits alive for Retry.
    await expect(shell).toBeAttached();
    await expect(shell).toBeHidden();
    await expect(shell).toHaveAttribute("inert", "");
    await expect(shell).toHaveAttribute("data-retry-sentinel", "retained");

    failAttempt = false;
    await page.getByRole("button", { name: "Retry" }).click();
    const play = page.getByRole("button", { name: "Play", exact: true });
    await expect(play).toBeEnabled({ timeout: 30_000 });
    await expect(shell).toBeVisible();
    await expect(shell).toHaveAttribute("data-retry-sentinel", "retained");

    // Let setState + setCell flush and both authoritative state echoes settle.
    // The final snapshot must still contain the edit made before the failure.
    await page.waitForTimeout(250);
    await expect(cell).toHaveAccessibleName("Cymbal step 16: on");
  });
});

test("mixer and mute state survive authoritative echoes and persistence", async ({
  page,
}) => {
  await page.goto("./");
  await expect(
    page.getByRole("button", { name: "Play", exact: true }),
  ).toBeEnabled({ timeout: 30_000 });

  const volume = page.getByRole("slider", { name: "Bass volume" });
  const mute = page.getByRole("button", { name: "Mute Bass" });
  await volume.focus();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  const edited = await volume.getAttribute("aria-valuenow");

  await mute.click();
  await expect(mute).toHaveAttribute("aria-pressed", "true");
  await page.waitForTimeout(500);

  await page.reload();
  await expect(
    page.getByRole("button", { name: "Play", exact: true }),
  ).toBeEnabled({ timeout: 30_000 });
  await expect(
    page.getByRole("slider", { name: "Bass volume" }),
  ).toHaveAttribute("aria-valuenow", edited!);
  await expect(page.getByRole("button", { name: "Mute Bass" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
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
  await physical.locator("..").click();
  await expect(physical).toBeChecked();
  const physicalInfo = dialog.getByRole("button", {
    name: "About physical modelling",
  });
  await physicalInfo.focus();
  await expect(
    dialog.getByRole("tooltip", { name: /Double-headed physical drum/ }),
  ).toBeVisible();
  // Derived from the generated table so adding a physical parameter doesn't
  // break this test; the named check keeps it from degrading into "16 knobs".
  await expect(dialog.getByRole("slider")).toHaveCount(
    PHYSICAL_TOM_PARAMS.length,
  );
  await expect(
    dialog.getByRole("slider", { name: "Tom damping frequency tilt" }),
  ).toBeVisible();

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

test("Tom 2 has an independent physical model and parameter bank", async ({
  page,
}) => {
  await page.goto("./");

  const play = page.getByRole("button", { name: "Play", exact: true });
  await expect(play).toBeEnabled({ timeout: 30_000 });
  await page.getByRole("button", { name: "Tom 2 voice settings" }).click();

  let dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("radio", { name: "Algorithmic" }),
  ).toBeChecked();
  const physical = dialog.getByRole("radio", { name: /Physical/ });
  await physical.locator("..").click();
  await expect(physical).toBeChecked();

  const tuning = dialog.getByRole("slider", {
    name: "Tom 2 batter head tension",
  });
  await tuning.focus();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  const edited = await tuning.getAttribute("aria-valuenow");
  await page.waitForTimeout(500);
  await dialog.getByRole("button", { name: /^Close Tom 2/ }).click();

  // The original Tom keeps its own model selection.
  await page.getByRole("button", { name: "Tom voice settings" }).click();
  dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("radio", { name: "Algorithmic" }),
  ).toBeChecked();
  await dialog.getByRole("button", { name: /^Close Tom voice/ }).click();

  await page.reload();
  await expect(play).toBeEnabled({ timeout: 30_000 });
  await page.getByRole("button", { name: "Tom 2 voice settings" }).click();
  dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("radio", { name: /Physical/ })).toBeChecked();
  await expect(
    dialog.getByRole("slider", { name: "Tom 2 batter head tension" }),
  ).toHaveAttribute("aria-valuenow", edited!);
});
