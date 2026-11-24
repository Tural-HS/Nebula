import { t as az } from "../src/locales/az/core";
import { t as ru } from "../src/locales/ru/core";
import { t as tr } from "../src/locales/tr/core";
import { authFile, expect, test } from "./fixtures";

// UI languages (ADR-0025). Each test has its own browser context, so the
// stored choice never leaks into other specs (they run with az-AZ).

test.use({ storageState: authFile("alice") });

test("the header switches the UI language and the login page follows it", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator("html")).toHaveAttribute("lang", "az");
  await expect(page.getByRole("button", { name: az.signOut })).toBeVisible();

  // The reload signs in again silently through the IdP, asking for Russian.
  const idp = page.waitForRequest((r) => r.url().includes("/protocol/openid-connect/auth") && r.url().includes("ui_locales=ru"));
  await page.getByRole("banner").getByLabel(az.language).selectOption("ru");
  await idp;
  await expect(page.locator("html")).toHaveAttribute("lang", "ru");
  await expect(page.getByRole("button", { name: ru.signOut })).toBeVisible();
  await expect(page.getByRole("banner").getByLabel(ru.language)).toHaveValue("ru");

  // The choice survives a reload and wins over the browser language.
  await page.reload();
  await expect(page.getByRole("button", { name: ru.signOut })).toBeVisible();
});

test.describe("browser language", () => {
  test.use({ locale: "tr-TR" });

  test("is used when nothing was chosen", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("html")).toHaveAttribute("lang", "tr");
    await expect(page.getByRole("button", { name: tr.signOut })).toBeVisible();
  });
});
