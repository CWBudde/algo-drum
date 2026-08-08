import { expect, test, type Page } from "@playwright/test";

import { encodeState } from "../src/algo/persistence";
import { createDefaultEngineState } from "../src/engine/engineState";

const EMPTY_SHARED_STATE = encodeState(createDefaultEngineState());

async function openEmptyMachine(page: Page): Promise<void> {
  await page.addInitScript(() => localStorage.clear());
  await page.goto(`./#${EMPTY_SHARED_STATE}`);
  await expect(
    page.getByRole("button", { name: "Play", exact: true }),
  ).toBeEnabled({ timeout: 30_000 });
}

async function dragAcross(page: Page, names: string[]): Promise<void> {
  const boxes = await Promise.all(
    names.map(async (name) => {
      const box = await page
        .getByRole("button", { name, exact: true })
        .boundingBox();
      if (!box) throw new Error(`No bounding box for ${name}`);
      return box;
    }),
  );
  const first = boxes[0]!;
  await page.mouse.move(first.x + first.width / 2, first.y + first.height / 2);
  await page.mouse.down();
  for (const box of boxes.slice(1)) {
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, {
      steps: 3,
    });
  }
  await page.mouse.up();
}

test("mouse paint commits once while clicks retain tri-state velocity", async ({
  page,
}) => {
  await openEmptyMachine(page);
  const step1 = page.getByRole("button", {
    name: "Cymbal step 1",
    exact: true,
  });
  const step2 = page.getByRole("button", {
    name: "Cymbal step 2",
    exact: true,
  });
  const step3 = page.getByRole("button", {
    name: "Cymbal step 3",
    exact: true,
  });

  await step1.click();
  await expect(step1).toHaveAttribute("aria-pressed", "true");
  await step1.click();
  await expect(step1).toHaveAttribute("aria-pressed", "mixed");
  await step2.click();

  const accentLed = await step1.locator(".dm-led").boundingBox();
  const normalLed = await step2.locator(".dm-led").boundingBox();
  expect(accentLed?.width).toBeGreaterThan(normalLed?.width ?? Infinity);

  await dragAcross(page, ["Cymbal step 1", "Cymbal step 2", "Cymbal step 3"]);
  await expect(step1).toHaveAttribute("aria-pressed", "false");
  await expect(step2).toHaveAttribute("aria-pressed", "false");
  await expect(step3).toHaveAttribute("aria-pressed", "false");

  // One undo restores the entire gesture, including the original accent.
  await page.keyboard.press("Control+z");
  await expect(step1).toHaveAttribute("aria-pressed", "mixed");
  await expect(step2).toHaveAttribute("aria-pressed", "true");
  await expect(step3).toHaveAttribute("aria-pressed", "false");

  // Painting from an empty cell uses the normal-velocity detent.
  await dragAcross(page, ["Cymbal step 3", "Cymbal step 4"]);
  await expect(step3).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("button", { name: "Cymbal step 4", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("row actions are accessible, atomic, and copy the independent length", async ({
  page,
}) => {
  await openEmptyMachine(page);
  const cymbal1 = page.getByRole("button", {
    name: "Cymbal step 1",
    exact: true,
  });
  const cymbal2 = page.getByRole("button", {
    name: "Cymbal step 2",
    exact: true,
  });
  await cymbal1.click();
  await cymbal2.click();
  await cymbal2.click();
  await page.getByLabel("Cymbal pattern length").selectOption("5");

  const opener = page.getByRole("button", { name: "Cymbal row actions" });
  await opener.click();
  const dialog = page.getByRole("dialog", { name: "Cymbal row actions" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Clear row" })).toBeFocused();
  await dialog.getByRole("button", { name: "Rotate right" }).click();
  await expect(cymbal1).toHaveAttribute("aria-pressed", "false");
  await expect(cymbal2).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("button", { name: "Cymbal step 3", exact: true }),
  ).toHaveAttribute("aria-pressed", "mixed");

  await page.keyboard.press("Control+z");
  await expect(cymbal1).toHaveAttribute("aria-pressed", "true");
  await expect(cymbal2).toHaveAttribute("aria-pressed", "mixed");

  await opener.click();
  await page
    .getByRole("dialog", { name: "Cymbal row actions" })
    .getByRole("button", { name: "Copy row", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Perc step 1", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("button", { name: "Perc step 2", exact: true }),
  ).toHaveAttribute("aria-pressed", "mixed");
  await expect(page.getByLabel("Perc pattern length")).toHaveValue("5");

  await page.keyboard.press("Control+z");
  await expect(
    page.getByRole("button", { name: "Perc step 1", exact: true }),
  ).toHaveAttribute("aria-pressed", "false");
  await expect(page.getByLabel("Perc pattern length")).toHaveValue("16");
});
