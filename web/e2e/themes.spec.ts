import { authFile, expect, test } from "./fixtures";

// Colour themes (ADR-0049): the picker switches at once, the choice survives a
// reload (applied before the first paint), and dark palettes keep dark mode.
test.use({ storageState: authFile("alice") });

test("pick paper, midnight, green and high contrast; back to system", async ({ page }) => {
  await page.goto("/");
  const picker = page.getByRole("combobox", { name: "Rəng rejimi" }).first();
  const root = page.locator("html");

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
  const brand = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--color-brand-600"));
  expect(brand).toContain("165");
  await page.reload();
  await expect(root).toHaveAttribute("data-theme", "green");

  await page.getByRole("combobox", { name: "Rəng rejimi" }).first().selectOption("contrast");
  await expect(root).toHaveAttribute("data-theme", "contrast");
  await expect(root).not.toHaveClass(/dark/);

  await page.getByRole("combobox", { name: "Rəng rejimi" }).first().selectOption("system");
  await expect(root).not.toHaveAttribute("data-theme", /.+/);
});
