import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Ledger records (imported customer cards): number units, the key facts card,
// tap-to-call phone numbers, folded empty sections and the pinned title.

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("guest");
  const alice = await apiAs("alice");
  const ws = `workspaces/${seed.ws}`;
  const field = async (data: Record<string, unknown>, position: number) => {
    expect((await alice.post(`${ws}/fields`, { data })).status()).toBe(201);
    expect((await alice.put(`${ws}/item-types/task/fields/${String(data.key)}`, { data: { position } })).status()).toBe(200);
  };
  await field({ key: "balance", name: "Qalıq", type: "number", options: { unit: "₼" } }, 1);
  await field({ key: "phone", name: "Əlaqə nömrəsi", type: "text" }, 2);
  await field({ key: "nv", name: "NV nömrəsi", type: "text" }, 3);
  for (const [title, balance] of [["Rəşad Quliyev Hikmət oğlu", 1200], ["Kullinan MMC", 800]] as const) {
    const data = { title, fields: { balance, phone: "515614145", nv: "10YX930" } };
    expect((await alice.post(`${ws}/projects/QA/items`, { data })).status()).toBe(201);
  }
  await alice.dispose();
});

test.describe("as admin", () => {
  test.use({ storageState: authFile("alice") });

  test("units in cells and totals, key facts, folded sections", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,status,field.balance,field.phone,field.nv`);
    await expect(page.getByRole("row").filter({ hasText: "Kullinan MMC" })).toContainText("800 ₼");
    await expect(page.getByRole("row", { name: "Cəmi" })).toContainText("2 000 ₼");
    // No row has a signal: no empty column.
    await expect(page.getByRole("columnheader", { name: "Siqnal" })).toHaveCount(0);

    await page.getByRole("button", { name: "Kullinan MMC" }).click();
    const panel = page.getByRole("dialog", { name: "QA-2" });
    const facts = panel.getByRole("region", { name: "Əsas məlumat" });
    await expect(facts).toContainText("800 ₼");
    await expect(facts.getByRole("link", { name: "515614145" })).toHaveAttribute("href", "tel:515614145");
    // An account-like number is not a phone link.
    await expect(facts.getByRole("link", { name: "10YX930" })).toHaveCount(0);

    // Empty sections fold into one row and open on demand.
    await expect(panel.getByRole("heading", { name: "Fayllar" })).toHaveCount(0);
    await panel.getByRole("button", { name: "+ Fayl" }).click();
    await expect(panel.getByRole("heading", { name: /Fayllar/ })).toBeVisible();
    await expect(panel.getByRole("button", { name: "+ Fayl" })).toHaveCount(0);
  });

  test("the title stays while the table scrolls sideways", async ({ page }) => {
    const many = ["key", "title", "status", "assignee", "due_date", "created_at", "updated_at", "field.balance", "field.phone", "field.nv"];
    await page.setViewportSize({ width: 900, height: 700 });
    await page.goto(`${seed.url}?cols=${many.join(",")}`);
    const title = page.getByRole("row").filter({ hasText: "Kullinan MMC" }).getByRole("cell").nth(1);
    const before = await title.boundingBox();
    await page.getByRole("table").evaluate((el) => { el.scrollLeft = 600; });
    await expect.poll(async () => (await page.getByRole("table").evaluate((el) => el.scrollLeft))).toBeGreaterThan(100);
    const after = await title.boundingBox();
    expect(Math.abs((after?.x ?? 0) - (before?.x ?? 0))).toBeLessThan(20);
  });

  test("an admin sets a number field's unit in the settings", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/settings/fields`);
    const unit = page.getByRole("textbox", { name: /^Vahid/ }).first();
    await unit.fill("AZN");
    await page.getByRole("button", { name: "Vahidi saxla" }).click();
    // Saved: the button goes once the field comes back with the new unit.
    await expect(page.getByRole("button", { name: "Vahidi saxla" })).toHaveCount(0);
    await page.goto(`${seed.url}?cols=key,title,field.balance`);
    await expect(page.getByRole("row").filter({ hasText: "Kullinan MMC" })).toContainText("800 AZN");
  });
});

test.describe("as guest", () => {
  test.use({ storageState: authFile("bob") });

  test("read-only rows link phone numbers and show no empty-section buttons", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,field.phone`);
    await expect(page.getByRole("row").filter({ hasText: "Kullinan MMC" }).getByRole("link", { name: "515614145" })).toHaveAttribute("href", "tel:515614145");
    await page.getByRole("button", { name: "Kullinan MMC" }).click();
    await expect(page.getByRole("dialog", { name: "QA-2" }).getByRole("button", { name: "+ Fayl" })).toHaveCount(0);
  });
});
