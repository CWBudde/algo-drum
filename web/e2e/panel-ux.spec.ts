import { devices, expect, test } from "@playwright/test";

async function waitForEngine(page: import("@playwright/test").Page) {
  await expect(
    page.getByRole("button", { name: "Play", exact: true }),
  ).toBeEnabled({ timeout: 30_000 });
}

async function machineLayout(page: import("@playwright/test").Page) {
  return page.locator(".dm-machine").evaluate((machine) => {
    const style = getComputedStyle(machine);
    return {
      scale: Number.parseFloat(style.getPropertyValue("--dm-scale")),
      zoom: Number.parseFloat(style.zoom),
      pageWidth: document.documentElement.scrollWidth,
      viewportWidth: document.documentElement.clientWidth,
    };
  });
}

test.use({ serviceWorkers: "block" });

test("preset identity follows edits and destructive history", async ({
  page,
}) => {
  await page.goto("./");
  await waitForEngine(page);

  const preset = page.getByRole("combobox", {
    name: "Load preset pattern",
  });

  await preset.selectOption({ label: "Rock" });
  await expect(preset).toHaveValue("0");

  // A direct cell edit is external to AlgoPanel's former local selector
  // state, so it is the regression case for a stale preset label.
  await page.getByRole("button", { name: "Bass step 2", exact: true }).click();
  await expect(preset).toHaveValue("-1");

  // Undo history covers destructive pattern actions. Restoring a byte-for-
  // byte preset must restore its label even though no select change occurred.
  await preset.selectOption({ label: "House" });
  await expect(preset).toHaveValue("1");
  await page.getByRole("button", { name: "Clear pattern" }).click();
  await expect(preset).toHaveValue("-1");
  await page.getByRole("button", { name: "Undo pattern action" }).click();
  await expect(preset).toHaveValue("1");
});

test("preset loading and MUTATE rewrite the audible pattern atomically", async ({
  page,
}) => {
  await page.goto("./");
  await waitForEngine(page);

  const preset = page.getByRole("combobox", { name: "Load preset pattern" });
  await preset.selectOption({ label: "Rock" });
  await expect(
    page.getByRole("button", { name: "Bass step 1", exact: true }),
  ).toHaveAttribute("aria-pressed", "mixed");
  await expect(
    page.getByRole("button", { name: "Bass step 9", exact: true }),
  ).toHaveAttribute("aria-pressed", "mixed");
  await expect(
    page.getByRole("button", { name: "Snare step 5", exact: true }),
  ).toHaveAttribute("aria-pressed", "mixed");
  await expect(
    page.getByRole("button", { name: "HiHat step 3", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("button", { name: "Bass step 2", exact: true }),
  ).toHaveAttribute("aria-pressed", "false");

  const cells = page.locator(".dm-cell");
  const snapshot = async () =>
    cells.evaluateAll((elements) =>
      elements.map((element) => element.getAttribute("aria-pressed")).join(),
    );
  const before = await snapshot();
  await page.getByRole("button", { name: "Mutate pattern" }).click();
  await expect.poll(snapshot).not.toBe(before);
  await page.getByRole("button", { name: "Undo pattern action" }).click();
  await expect.poll(snapshot).toBe(before);
});

test("Euclid number drafts commit on blur, Enter, and Fill", async ({
  page,
}) => {
  await page.goto("./");
  await waitForEngine(page);

  await page.getByText("EUCLID", { exact: true }).click();
  const hits = page.getByLabel("HITS", { exact: true });
  const rotation = page.getByLabel("ROTATE", { exact: true });

  await hits.fill("");
  await expect(hits).toHaveValue("");
  await hits.blur();
  await expect(hits).toHaveValue("0");

  // Out-of-range text remains untouched while editing, then rounds/clamps at
  // the explicit keyboard commit boundary.
  await hits.fill("99");
  await expect(hits).toHaveValue("99");
  await hits.press("Enter");
  await expect(hits).toHaveValue("16");

  await rotation.fill("99");
  await expect(rotation).toHaveValue("99");
  await page
    .locator(".dm-euclid-panel")
    .getByRole("button", { name: "FILL", exact: true })
    .click();
  await expect(rotation).toHaveValue("15");
});

test("Euclid FILL replaces only the selected track with the requested rhythm", async ({
  page,
}) => {
  await page.goto("./");
  await waitForEngine(page);
  await page
    .getByRole("combobox", { name: "Load preset pattern" })
    .selectOption({ label: "Rock" });

  await page.getByText("EUCLID", { exact: true }).click();
  const euclid = page.locator(".dm-euclid-panel");
  await euclid.locator("select").selectOption({ label: "Snare" });
  await euclid.getByLabel("HITS", { exact: true }).fill("4");
  await euclid.getByLabel("ROTATE", { exact: true }).fill("0");
  await euclid.getByRole("button", { name: "FILL", exact: true }).click();

  for (let step = 1; step <= 16; step++) {
    await expect(
      page.getByRole("button", { name: `Snare step ${step}`, exact: true }),
    ).toHaveAttribute(
      "aria-pressed",
      [1, 5, 9, 13].includes(step) ? "true" : "false",
    );
  }
  await expect(
    page.getByRole("button", { name: "Bass step 1", exact: true }),
  ).toHaveAttribute("aria-pressed", "mixed");
});

test("master knobs accept keyboard and pointer edits and persist them", async ({
  page,
}) => {
  await page.goto("./");
  await waitForEngine(page);

  const steps = page.getByRole("slider", { name: "Pattern length" });
  const probability = page.getByRole("slider", {
    name: "Trigger probability",
  });
  const humanize = page.getByRole("slider", { name: "Humanize" });
  const reverb = page.getByRole("slider", { name: "REVERB", exact: true });

  await steps.focus();
  await page.keyboard.press("Home");
  await expect(steps).toHaveAttribute("aria-valuetext", "1 steps");
  await probability.focus();
  await page.keyboard.press("Home");
  await expect(probability).toHaveAttribute("aria-valuenow", "0");
  await humanize.focus();
  await page.keyboard.press("End");
  await expect(humanize).toHaveAttribute("aria-valuenow", "100");

  const box = await reverb.boundingBox();
  if (!box) throw new Error("Reverb knob has no bounding box");
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x, y - 75, { steps: 5 });
  await page.mouse.up();
  await expect(reverb).toHaveAttribute("aria-valuenow", "50");

  await page.waitForTimeout(500);
  await page.reload();
  await waitForEngine(page);
  await expect(steps).toHaveAttribute("aria-valuetext", "1 steps");
  await expect(probability).toHaveAttribute("aria-valuenow", "0");
  await expect(humanize).toHaveAttribute("aria-valuenow", "100");
  await expect(reverb).toHaveAttribute("aria-valuenow", "50");
});

test("the themed loading status transitions to the visible ready state", async ({
  page,
}) => {
  let releaseWasm!: () => void;
  let reportWasmRequest!: () => void;
  const wasmReleased = new Promise<void>((resolve) => {
    releaseWasm = resolve;
  });
  const wasmRequested = new Promise<void>((resolve) => {
    reportWasmRequest = resolve;
  });

  await page.route("**/algo_drum.wasm", async (route) => {
    reportWasmRequest();
    await wasmReleased;
    await route.continue();
  });

  await page.goto("./");
  await wasmRequested;

  const loading = page.getByRole("status").filter({
    hasText: "Loading audio engine",
  });
  await expect(loading).toBeVisible();
  await expect(loading).toContainText("Preparing the WebAssembly instrument");
  await expect(loading.locator(".app-loading-indicator")).toBeVisible();
  await expect(page.locator(".app-machine")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Play", exact: true }),
  ).toBeDisabled();

  const status = page.locator(".dm-status");
  await expect
    .poll(() =>
      status.evaluate((element) =>
        getComputedStyle(element, "::after").content.replaceAll('"', ""),
      ),
    )
    .toBe("ENGINE LOADING");

  releaseWasm();
  await waitForEngine(page);
  await expect(loading).toHaveCount(0);
  await expect(status).toHaveClass(/dm-status-ready/);
  await expect
    .poll(() =>
      status.evaluate((element) =>
        getComputedStyle(element, "::after").content.replaceAll('"', ""),
      ),
    )
    .toBe("ENGINE READY");
});

test("fine-pointer layouts scale the complete panel without page overflow", async ({
  page,
}) => {
  await page.goto("./");
  await waitForEngine(page);

  for (const expected of [
    { width: 390, height: 844, scale: 0.9 },
    { width: 900, height: 900, scale: 1 },
    { width: 1440, height: 1000, scale: 1.2 },
  ]) {
    await page.setViewportSize({
      width: expected.width,
      height: expected.height,
    });
    await expect
      .poll(async () => (await machineLayout(page)).scale)
      .toBe(expected.scale);

    const layout = await machineLayout(page);
    expect(layout.zoom).toBe(expected.scale);
    expect(layout.pageWidth).toBeLessThanOrEqual(layout.viewportWidth);
  }
});

test.describe("coarse-pointer scaling", () => {
  const pixel = devices["Pixel 5"];
  test.use({
    userAgent: pixel.userAgent,
    viewport: pixel.viewport,
    deviceScaleFactor: pixel.deviceScaleFactor,
    isMobile: pixel.isMobile,
    hasTouch: pixel.hasTouch,
    serviceWorkers: "block",
  });

  test("keeps the mobile scale and touch targets at their accessibility floors", async ({
    page,
  }) => {
    await page.goto("./");
    await waitForEngine(page);

    const layout = await machineLayout(page);
    expect(layout.scale).toBe(1);
    expect(layout.zoom).toBe(1);
    expect(layout.pageWidth).toBeLessThanOrEqual(layout.viewportWidth);

    const pad = await page
      .getByRole("button", { name: "Cymbal step 1", exact: true })
      .boundingBox();
    const tap = await page
      .getByRole("button", { name: "Tap tempo" })
      .boundingBox();
    expect(pad?.width).toBeGreaterThanOrEqual(44);
    expect(pad?.height).toBeGreaterThanOrEqual(44);
    expect(tap?.width).toBeGreaterThanOrEqual(44);
    expect(tap?.height).toBeGreaterThanOrEqual(44);
  });
});
