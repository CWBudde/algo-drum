import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

async function waitForEngine(page: import("@playwright/test").Page) {
  await expect(
    page.getByRole("button", { name: "Play", exact: true }),
  ).toBeEnabled({ timeout: 30_000 });
}

test.use({ serviceWorkers: "block" });

test("global shortcuts are discoverable, focused, and undoable", async ({
  page,
}) => {
  await page.goto("./");
  await waitForEngine(page);

  const preset = page.getByRole("combobox", { name: "Load preset pattern" });
  await preset.selectOption({ label: "Rock" });
  const opener = page.getByRole("button", {
    name: "Cymbal step 1",
    exact: true,
  });
  await opener.focus();
  await page.keyboard.press("?");

  const dialog = page.getByRole("dialog", { name: "Keyboard shortcuts" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("Shift + Backspace");
  await expect(dialog).toContainText("Mute or unmute a track");
  expect(
    (await new AxeBuilder({ page }).include(".dm-shortcuts").analyze())
      .violations,
  ).toEqual([]);

  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(opener).toBeFocused();

  await page.keyboard.press("p");
  await expect(preset).toHaveValue("1");
  await page.keyboard.press("Shift+p");
  await expect(preset).toHaveValue("0");

  const cymbalMute = page.getByRole("button", { name: "Mute Cymbal" });
  await expect(cymbalMute).toHaveAttribute("aria-pressed", "false");
  await page.keyboard.press("Digit1");
  await expect(cymbalMute).toHaveAttribute("aria-pressed", "true");

  await page.keyboard.press("Shift+Backspace");
  const activeCells = page.locator(
    ".dm-cell[aria-pressed='true'], .dm-cell[aria-pressed='mixed']",
  );
  await expect(activeCells).toHaveCount(0);
  await page.keyboard.press("Control+z");
  await expect(activeCells).not.toHaveCount(0);
});

test("saved count-in owns transport before playback and remains stoppable", async ({
  page,
}) => {
  await page.goto("./");
  await waitForEngine(page);

  const metronome = page.getByRole("button", { name: "Playback metronome" });
  const countIn = page.getByRole("combobox", { name: "Count-in bars" });
  await metronome.click();
  await countIn.selectOption("1");
  await expect(metronome).toHaveAttribute("aria-pressed", "true");
  await expect(countIn).toHaveValue("1");

  // State persistence is debounced by 300 ms.
  await page.waitForTimeout(400);
  await page.reload();
  await waitForEngine(page);
  await expect(metronome).toHaveAttribute("aria-pressed", "true");
  await expect(countIn).toHaveValue("1");

  await page.getByRole("button", { name: "Play", exact: true }).click();
  const counting = page.getByRole("button", { name: "Counting in" });
  await expect(counting).toBeDisabled({ timeout: 10_000 });
  await expect(page.locator(".dm-cell[data-playhead]")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Stop" })).toBeEnabled();

  await expect(
    page.getByRole("button", { name: "Pause", exact: true }),
  ).toBeVisible({ timeout: 5_000 });
  await expect(page.locator(".dm-cell[data-playhead]")).not.toHaveCount(0, {
    timeout: 3_000,
  });

  await page.getByRole("button", { name: "Stop" }).click();
  await page.getByRole("button", { name: "Play", exact: true }).click();
  await expect(counting).toBeDisabled({ timeout: 3_000 });
  await page.getByRole("button", { name: "Stop" }).click();
  await expect(
    page.getByRole("button", { name: "Play", exact: true }),
  ).toBeEnabled();
  await expect(page.locator(".dm-cell[data-playhead]")).toHaveCount(0);
});
