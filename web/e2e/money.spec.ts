import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Money fields (ADR-0044): an admin creates one, values are edited inline
// in two currencies, and the totals row shows one total per currency.

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const alice = await apiAs("alice");
  const items = `workspaces/${seed.ws}/projects/QA/items`;
  for (const title of ["Lizinq ödənişi", "Valyuta köçürməsi", "Komissiya"]) {
    expect((await alice.post(items, { data: { title } })).status()).toBe(201);
  }
  await alice.dispose();
});

test.use({ storageState: authFile("alice") });

test("create a money field, set amounts in two currencies, see per-currency totals", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/settings/fields`);
  const form = page.locator("form").filter({ has: page.getByRole("button", { name: "Yarat" }) });
  await form.getByLabel("Ad", { exact: true }).fill("Qiymət");
  await form.getByLabel(/Açar/).fill("price");
  await form.getByLabel("Tip").selectOption("money");
  await expect(form.getByLabel("İcazəli valyutalar")).toHaveValue("AZN, USD, EUR");
  await expect(form.getByLabel("Əsas valyuta")).toHaveValue("AZN");
  await form.getByRole("button", { name: "Yarat" }).click();
  await expect(page.getByText("price", { exact: true })).toBeVisible();

  const alice = await apiAs("alice");
  expect((await alice.put(`workspaces/${seed.ws}/item-types/task/fields/price`, { data: { position: 1 } })).status()).toBe(200);
  await alice.dispose();

  await page.goto(`${seed.url}?cols=key,title,field.price`);
  const row = (title: string) => page.getByRole("row").filter({ hasText: title });

  // AZN (the default currency), typed the Azerbaijani way.
  await row("Lizinq").getByRole("button", { name: /QA-1: Qiymət dəyiş/ }).click();
  await page.getByLabel("Qiymət: məbləğ").fill("1 250,50");
  await page.getByLabel("Qiymət: məbləğ").press("Enter");
  await expect(row("Lizinq")).toContainText("1 250,50 AZN");

  // USD: amount and currency.
  await row("Valyuta").getByRole("button", { name: /QA-2: Qiymət dəyiş/ }).click();
  await page.getByLabel("Qiymət: məbləğ").fill("300");
  await page.getByLabel("Qiymət: valyuta").selectOption("USD");
  await page.getByLabel("Qiymət: məbləğ").press("Enter");
  await expect(row("Valyuta")).toContainText("300,00 USD");

  // Currencies are never added together.
  const total = page.getByRole("row", { name: "Cəmi" });
  await expect(total).toContainText("1 250,50 AZN · 300,00 USD");

  // Too many decimals: refused with how to fix it, the value stays empty.
  await row("Komissiya").getByRole("button", { name: /QA-3: Qiymət dəyiş/ }).click();
  await page.getByLabel("Qiymət: məbləğ").fill("1,005");
  await page.getByLabel("Qiymət: məbləğ").press("Enter");
  await expect(page.getByText("ən çox 2 onluq rəqəm")).toBeVisible();
  await expect(row("Komissiya")).not.toContainText("AZN");

  // Filtering by currency narrows the list and the totals.
  await page.goto(`${seed.url}?cols=key,title,field.price&field.price=USD`);
  await expect(page.getByRole("row", { name: "Cəmi" })).toContainText("300,00 USD");
  await expect(page.getByRole("row", { name: "Cəmi" })).not.toContainText("AZN");
  await expect(row("Lizinq")).toHaveCount(0);
});
