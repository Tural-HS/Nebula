import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Saved views, custom field columns and the totals row (ADR-0014).

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const alice = await apiAs("alice");
  const ws = `workspaces/${seed.ws}`;
  expect((await alice.post(`${ws}/fields`, { data: { key: "monthly", name: "Aylıq ödəniş", type: "number" } })).status()).toBe(201);
  expect((await alice.put(`${ws}/item-types/task/fields/monthly`, { data: { position: 1 } })).status()).toBe(200);
  const create = async (title: string, monthly: number | null, due?: string) => {
    const data = { title, due_date: due, fields: monthly === null ? {} : { monthly } };
    expect((await alice.post(`${ws}/projects/QA/items`, { data })).status()).toBe(201);
  };
  await create("Paşayev ödəniş", 1200.5, "2026-12-09"); // QA-1
  await create("Əliyev ödəniş", 800, "2026-11-30"); // QA-2
  await create("Tarixsiz iş", null); // QA-3
  await alice.dispose();
});

test.describe("as admin", () => {
  test.use({ storageState: authFile("alice") });

  test("custom field columns, server totals and a shared view", async ({ page }) => {
    await page.goto(seed.url);
    await page.getByText("Sütunlar").click();
    // A ticked column moves into the ordered list (a new element), so click and
    // assert instead of check(), which watches the original node.
    await page.getByRole("checkbox", { name: "Aylıq ödəniş" }).click();
    await expect(page.getByRole("checkbox", { name: "Aylıq ödəniş" })).toBeChecked();
    await page.keyboard.press("Escape");
    await expect(page).toHaveURL(/cols=/);
    await expect(page.getByRole("columnheader", { name: "Aylıq ödəniş" })).toBeVisible();
    await expect(page.getByRole("row").filter({ hasText: "Paşayev ödəniş" })).toContainText("1 200,5");

    // The total covers the whole filtered result, not only loaded rows.
    const total = page.getByRole("row", { name: "Cəmi" });
    await expect(total).toContainText("2 000,5");
    await page.getByLabel("Axtar…").fill("Əliyev");
    await expect(total).toContainText("800");

    await page.getByLabel("Axtar…").fill("");
    await page.getByLabel("Sıralama").selectOption("due");
    await expect(page).toHaveURL(/sort=due/);
    const keys = page.getByRole("row").filter({ hasText: "QA-" }).locator("[role=cell]").first();
    await expect(keys).toHaveText("QA-2"); // soonest due date first

    await page.getByRole("button", { name: "Görünüş kimi saxla" }).click();
    const dialog = page.getByRole("dialog", { name: "Yeni görünüş" });
    await dialog.getByLabel("Ad").fill("Plan 2027");
    await dialog.getByLabel(/Hamı ilə paylaş/).check();
    await dialog.getByRole("button", { name: "Yadda saxla" }).click();
    await expect(page).toHaveURL(/saved=/);
    await expect(page.getByLabel("Görünüş", { exact: true })).toHaveValue(/.+/);

    // Back to all items: default columns, no totals row.
    await page.getByLabel("Görünüş", { exact: true }).selectOption({ label: "Hamısı" });
    await expect(page.getByRole("columnheader", { name: "Aylıq ödəniş" })).toHaveCount(0);
    await expect(page).not.toHaveURL(/sort=due/);

    // Selecting the view restores its columns and sort; a change marks it modified.
    await page.getByLabel("Görünüş", { exact: true }).selectOption({ label: "Plan 2027" });
    await expect(page.getByRole("columnheader", { name: "Aylıq ödəniş" })).toBeVisible();
    await expect(page).toHaveURL(/sort=due/);
    await page.getByLabel("Son tarix", { exact: true }).selectOption("none");
    await expect(page.getByText("dəyişdirilib")).toBeVisible();
    // Wait for the save itself, so a failure names the API status instead of
    // only the indicator that stayed.
    const saved = page.waitForResponse((r) => r.request().method() === "PATCH" && /\/views\//.test(r.url()) && r.status() !== 429); // 429 is retried
    await page.getByRole("button", { name: "Dəyişiklikləri saxla" }).click();
    const res = await saved;
    expect(res.status(), await res.text()).toBe(200);
    await expect(page.getByText("dəyişdirilib")).toHaveCount(0);

    // A bare ?saved= link applies the view.
    const id = new URL(page.url()).searchParams.get("saved") ?? "";
    await page.goto(`${seed.url}?saved=${id}`);
    await expect(page).toHaveURL(/due=none/);
    await expect(page.getByRole("row").filter({ hasText: "QA-" })).toHaveCount(1);
  });
});

test.describe("as member", () => {
  test.use({ storageState: authFile("bob") });

  test("sees shared views read-only and keeps personal views private", async ({ page }) => {
    const alice = await apiAs("alice");
    const res = await alice.post(`workspaces/${seed.ws}/projects/QA/views`, {
      data: { name: "Gecikənlər", shared: true, config: { filters: { due: "overdue" }, columns: [], sort: "number", group_by: "status" } },
    });
    expect(res.status()).toBe(201);
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/views`, { data: { name: "Alice-in şəxsi görünüşü" } })).status()).toBe(201);
    await alice.dispose();

    await page.goto(seed.url);
    const picker = page.getByLabel("Görünüş", { exact: true });
    await expect(picker.locator("option", { hasText: "Gecikənlər" })).toHaveCount(1);
    await expect(picker.locator("option", { hasText: "Alice-in şəxsi görünüşü" })).toHaveCount(0);
    await picker.selectOption({ label: "Gecikənlər" });
    await expect(page).toHaveURL(/due=overdue/);
    await expect(page.getByLabel("Görünüş əməliyyatları")).toHaveCount(0); // no rename/share/delete

    // Members save personal views; sharing is not offered.
    await page.getByRole("button", { name: "Görünüş kimi saxla" }).click();
    const dialog = page.getByRole("dialog", { name: "Yeni görünüş" });
    await expect(dialog.getByLabel(/Hamı ilə paylaş/)).toHaveCount(0);
    await dialog.getByLabel("Ad").fill("Bob-un görünüşü");
    await dialog.getByRole("button", { name: "Yadda saxla" }).click();
    await expect(page.getByLabel("Görünüş əməliyyatları")).toBeVisible();
  });
});
