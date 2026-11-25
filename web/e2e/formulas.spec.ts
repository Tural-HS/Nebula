import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Formula fields (ADR-0030): the editor validates live against the server,
// values are computed on write, shown read-only with ƒ, totalled and sorted.

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const alice = await apiAs("alice");
  const ws = `workspaces/${seed.ws}`;
  for (const [key, name] of [["amount", "Məbləğ"], ["paid", "Ödənilib"]] as const) {
    expect((await alice.post(`${ws}/fields`, { data: { key, name, type: "number" } })).status()).toBe(201);
    expect((await alice.put(`${ws}/item-types/task/fields/${key}`, { data: { position: 1 } })).status()).toBe(200);
  }
  // QA-1 exists before the formula: the editor previews against it.
  expect((await alice.post(`${ws}/projects/QA/items`, { data: { title: "Nümunə faktura", fields: { amount: 1200.5, paid: 200 } } })).status()).toBe(201);
  await alice.dispose();
});

test.describe("as admin", () => {
  test.use({ storageState: authFile("alice") });

  test("create a formula with live validation, then see it in the table and drawer", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/settings/fields`);
    const form = page.locator("form").filter({ has: page.getByRole("button", { name: "Yarat" }) });
    await form.getByLabel("Ad", { exact: true }).fill("Qalıq");
    await form.getByLabel(/Açar/).fill("remaining");
    await form.getByLabel("Tip").selectOption("formula");

    // A typo is reported with its position, before anything is saved.
    const expr = form.getByRole("textbox", { name: /Düstur/ });
    await expr.fill("amount - pad");
    const status = form.getByTestId("formula-status");
    await expect(status).toContainText("Mövqe 10");
    await expect(status).toContainText('unknown field "pad"');
    await expect(expr).toHaveAttribute("aria-invalid", "true");

    await expr.fill("amount - paid");
    await form.getByLabel("Nümunə tapşırıq").fill("QA-1");
    await expect(status).toContainText("Düstur düzgündür");
    await expect(form.getByTestId("formula-result")).toHaveText("1 000,5");
    await form.getByRole("button", { name: "Yarat" }).click();
    await expect(page.getByText("remaining", { exact: true })).toBeVisible();

    // Items written after the formula exists get their value on write.
    const alice = await apiAs("alice");
    // The drawer shows the fields bound to the item type (ADR-0003), formulas included.
    expect((await alice.put(`workspaces/${seed.ws}/item-types/task/fields/remaining`, { data: { position: 3 } })).status()).toBe(200);
    const items = `workspaces/${seed.ws}/projects/QA/items`;
    expect((await alice.post(items, { data: { title: "Lizinq", fields: { amount: 900, paid: 150 } } })).status()).toBe(201); // QA-2: 750
    expect((await alice.post(items, { data: { title: "Kredit", fields: { amount: 300.25, paid: 0.25 } } })).status()).toBe(201); // QA-3: 300
    // The value is read-only through the API too.
    expect((await alice.patch(`${items}/2`, { data: { version: 1, fields: { remaining: 1 } } })).status()).toBe(422);
    await alice.dispose();

    await page.goto(`${seed.url}?cols=key,title,field.amount,field.remaining&q=Lizinq%20OR%20Kredit`);
    const header = page.getByRole("columnheader", { name: /Qalıq/ });
    await expect(header).toContainText("ƒ");
    await expect(page.getByRole("row").filter({ hasText: "Lizinq" })).toContainText("750");
    await expect(page.getByRole("row", { name: "Cəmi" })).toContainText("1 050");

    // Sorting by the formula, descending, is a server-side sort in the URL.
    await page.getByLabel("Sıralama", { exact: true }).selectOption("-field.remaining");
    await expect(page).toHaveURL(/sort=-field\.remaining/);
    const firstKey = page.getByRole("row").filter({ hasText: "QA-" }).first().locator("[role=cell]").first();
    await expect(firstKey).toHaveText("QA-2");
    await page.getByLabel("Sıralama", { exact: true }).selectOption("field.remaining");
    await expect(firstKey).toHaveText("QA-3");

    // The drawer shows the value read-only, with the formula as a hint.
    await page.getByRole("row").filter({ hasText: "Lizinq" }).getByText("Lizinq").click();
    const value = page.locator("output[data-formula]");
    await expect(value).toContainText("750");
    await expect(value).toHaveAttribute("title", /ƒ = amount - paid/);
    await expect(page.locator("input#field-remaining, textarea#field-remaining")).toHaveCount(0);
  });
});

test.describe("as member", () => {
  test.use({ storageState: authFile("bob") });

  test("cannot use the formula preview", async () => {
    const bob = await apiAs("bob");
    expect((await bob.post(`workspaces/${seed.ws}/fields/formula-preview`, { data: { expression: "1" } })).status()).toBe(403);
    await bob.dispose();
  });
});
