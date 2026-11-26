import { authFile, expect, test } from "./fixtures";

// Colour themes (ADR-0049): the picker switches at once, the choice survives a
// reload (applied before the first paint), and dark palettes keep dark mode.
test.use({ storageState: authFile("alice") });

test("pick paper, midnight, green and high contrast; back to system", async ({ page }) => {
  await page.goto("/");
  const picker = page.getByRole("combobox", { name: "Rəng rejimi" }).first();
  const root = page.locator("html");

  // The default brand colour (teal), in whatever notation the browser reports.
  const brandOf = () => page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--color-brand-600"));
  const teal = await brandOf();

  await picker.selectOption("paper");
  await expect(root).toHaveAttribute("data-theme", "paper");
  await expect(root).not.toHaveClass(/dark/);
  await page.reload();
  await expect(root).toHaveAttribute("data-theme", "paper");

  await page.getByRole("combobox", { name: "Rəng rejimi" }).first().selectOption("midnight");
  await expect(root).toHaveAttribute("data-theme", "midnight");
  await expect(root).toHaveClass(/dark/);
  // The palette replaces the neutral scale: the page background is navy, not zinc.
  const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  expect(bg).not.toBe("rgb(9, 9, 11)");

  // FinTez green: a light palette whose brand colour is green, not teal.
  await page.getByRole("combobox", { name: "Rəng rejimi" }).first().selectOption("green");
  await expect(root).toHaveAttribute("data-theme", "green");
  await expect(root).not.toHaveClass(/dark/);
  // Chromium may report oklch() as lab(): compare with the default instead of the text.
  await expect.poll(brandOf).not.toBe(teal);
  await page.reload();
  await expect(root).toHaveAttribute("data-theme", "green");

  await page.getByRole("combobox", { name: "Rəng rejimi" }).first().selectOption("contrast");
  await expect(root).toHaveAttribute("data-theme", "contrast");
  await expect(root).not.toHaveClass(/dark/);

  await page.getByRole("combobox", { name: "Rəng rejimi" }).first().selectOption("system");
  await expect(root).not.toHaveAttribute("data-theme", /.+/);
});

test("the extra dark palettes keep dark mode and survive a reload (ADR-0058)", async ({ page }) => {
  await page.goto("/");
  const root = page.locator("html");
  for (const name of ["evergreen", "oled", "dusk", "mocha"]) {
    await page.getByRole("combobox", { name: "Rəng rejimi" }).first().selectOption(name);
    await expect(root).toHaveAttribute("data-theme", name);
    await expect(root).toHaveClass(/dark/);
  }
  await page.reload();
  await expect(root).toHaveAttribute("data-theme", "mocha");
  await expect(root).toHaveClass(/dark/);
  // OLED: a black page.
  await page.getByRole("combobox", { name: "Rəng rejimi" }).first().selectOption("oled");
  await expect.poll(() => page.evaluate(() => getComputedStyle(document.body).backgroundColor)).toMatch(/^(rgb\(0, 0, 0\)|oklch\(0 0 0\)|lab\(0 0 0\))$/);
  await page.getByRole("combobox", { name: "Rəng rejimi" }).first().selectOption("system");
});
