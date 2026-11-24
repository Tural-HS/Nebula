import { tth as az } from "../src/locales/az/theme";
import { authFile, expect, test } from "./fixtures";

// Colour theme (ADR-0035). Each test has its own browser context, so the
// stored choice never leaks into other specs. The desktop project is 1280 px
// wide, so the picker is in the header (xl and up).

test.use({ storageState: authFile("alice") });

const html = (page: import("@playwright/test").Page) => page.locator("html");

test("the header switches the theme at once and the choice survives a reload", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");
  await expect(html(page)).not.toHaveClass(/\bdark\b/);
  const picker = page.getByRole("banner").getByLabel(az.theme, { exact: true });
  // The sign-in redirect is a real page load: set the marker once the app runs.
  await expect(picker).toBeVisible();

  // No reload: a marker on window survives the switch.
  await page.evaluate(() => { (window as unknown as { __noReload: boolean }).__noReload = true; });
  await picker.selectOption("dark");
  await expect(html(page)).toHaveClass(/\bdark\b/);
  expect(await page.evaluate(() => (window as unknown as { __noReload?: boolean }).__noReload)).toBe(true);
  expect(await page.evaluate(() => getComputedStyle(document.body).backgroundColor)).not.toBe("rgb(250, 250, 250)");

  // Applied before the first paint after a reload (theme-init.js), and wins over the OS.
  await page.reload();
  await expect(html(page)).toHaveClass(/\bdark\b/);
  await expect(page.getByRole("banner").getByLabel(az.theme, { exact: true })).toHaveValue("dark");

  await page.getByRole("banner").getByLabel(az.theme, { exact: true }).selectOption("light");
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(html(page)).not.toHaveClass(/\bdark\b/);
});

test("system follows the OS setting live", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");
  await expect(page.getByRole("banner").getByLabel(az.theme, { exact: true })).toHaveValue("system");
  await expect(html(page)).not.toHaveClass(/\bdark\b/);
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(html(page)).toHaveClass(/\bdark\b/);
  await page.reload();
  await expect(html(page)).toHaveClass(/\bdark\b/);
});

test.describe("narrow screens", () => {
  test.use({ viewport: { width: 375, height: 760 } });

  test("pick the theme on the home page, not in the header", async ({ page }) => {
    await page.emulateMedia({ colorScheme: "light" });
    await page.goto("/");
    await expect(page.getByRole("banner").getByLabel(az.theme, { exact: true })).toBeHidden();
    await page.getByRole("main").getByLabel(az.theme, { exact: true }).selectOption("dark");
    await expect(html(page)).toHaveClass(/\bdark\b/);
    await page.reload();
    await expect(html(page)).toHaveClass(/\bdark\b/);
    await expect(page.getByRole("main")).toBeVisible(); // past the sign-in redirect (a page load)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });
});
