import type { APIRequestContext } from "@playwright/test";

import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

interface ApiItem {
  number: number;
  version: number;
  due_date: string | null;
  assignee_id: string | null;
  status: { name: string };
}

async function create(api: APIRequestContext, ws: string, title: string) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const r = await api.post(`workspaces/${ws}/projects/QA/items`, { data: { title } });
    if (r.status() !== 429) {
      expect(r.status()).toBe(201);
      return;
    }
    await new Promise((res) => setTimeout(res, Math.min(Number(r.headers()["retry-after"] ?? "1"), 10) * 1000));
  }
  throw new Error("rate limited");
}

const get = async (api: APIRequestContext, ws: string, n: number) =>
  (await (await api.get(`workspaces/${ws}/projects/QA/items/${String(n)}`)).json()) as ApiItem;

test.describe("bulk and inline editing", () => {
  let seed: Seed;
  let alice: APIRequestContext;
  test.beforeEach(async () => {
    seed = await seedWorkspace();
    alice = await apiAs("alice");
    for (const t of ["Birinci", "İkinci", "Üçüncü", "Dördüncü"]) await create(alice, seed.ws, t);
  });
  test.afterEach(async () => { await alice.dispose(); });

  test("select rows, change status and due date for all of them", async ({ page }) => {
    await page.goto(seed.url);
    await page.getByRole("checkbox", { name: "QA-1 seç" }).check();
    await page.getByRole("checkbox", { name: "QA-2 seç" }).check();
    const bar = page.getByRole("toolbar", { name: "Seçilmiş tapşırıqlar üçün əməliyyatlar" });
    await expect(bar.getByRole("status")).toHaveText("2 seçilib");

    await bar.getByLabel("Statusu dəyiş").selectOption({ label: "In Progress" });
    await expect(page.getByRole("status").filter({ hasText: "2 tapşırıq yeniləndi" })).toBeVisible();
    await expect(bar).toBeHidden(); // the selection is cleared after a change
    await expect(page.getByRole("row").filter({ hasText: "QA-1" })).toContainText("In Progress");
    await expect(page.getByRole("row").filter({ hasText: "QA-3" })).toContainText("To Do");

    // Undo puts both back, each with its own previous value.
    await page.getByRole("button", { name: "Geri qaytar" }).click();
    await expect(page.getByRole("status").filter({ hasText: "2 tapşırıq əvvəlki vəziyyətinə qaytarıldı" })).toBeVisible();
    await expect(page.getByRole("row").filter({ hasText: "QA-1" })).toContainText("To Do");
    expect((await get(alice, seed.ws, 2)).status.name).toBe("To Do");

    // Select all: every row of the filtered list.
    await page.getByRole("checkbox", { name: "Hamısını seç" }).check();
    await expect(bar.getByRole("status")).toHaveText("4 seçilib");
    await bar.getByLabel("Son tarixi dəyiş").fill("2031-05-20");
    await bar.getByRole("button", { name: "Tətbiq et" }).click();
    await expect(page.getByRole("status").filter({ hasText: "4 tapşırıq yeniləndi" })).toBeVisible();
    for (const n of [1, 2, 3, 4]) expect((await get(alice, seed.ws, n)).due_date).toBe("2031-05-20");

    // A filter change starts a new selection.
    await page.getByRole("checkbox", { name: "QA-4 seç" }).check();
    await expect(bar).toBeVisible();
    await page.getByRole("searchbox", { name: "Axtar…" }).fill("Birinci");
    await expect(page.getByRole("row").filter({ hasText: "QA-" })).toHaveCount(1);
    await expect(bar).toBeHidden();
  });

  test("a stale row fails the whole change and lists why", async ({ page }) => {
    await page.goto(seed.url);
    await page.getByRole("checkbox", { name: "QA-1 seç" }).check();
    await page.getByRole("checkbox", { name: "QA-2 seç" }).check();
    // Someone else edits QA-2 meanwhile.
    const two = await get(alice, seed.ws, 2);
    expect((await alice.patch(`workspaces/${seed.ws}/projects/QA/items/2`, { data: { version: two.version, title: "Başqası dəyişdi" } })).status()).toBe(200);

    const bar = page.getByRole("toolbar", { name: "Seçilmiş tapşırıqlar üçün əməliyyatlar" });
    await bar.getByLabel("İcraçını dəyiş").selectOption({ label: "Bob Developer" });
    const dialog = page.getByRole("dialog", { name: "Heç nə dəyişdirilmədi" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("bulk-errors")).toContainText("QA-2");
    await expect(dialog.getByTestId("bulk-errors")).not.toContainText("QA-1");
    expect((await get(alice, seed.ws, 1)).assignee_id).toBeNull(); // all or nothing
  });

  test("admins delete in bulk after confirming", async ({ page }) => {
    await page.goto(seed.url);
    await page.getByRole("checkbox", { name: "QA-3 seç" }).check();
    await page.getByRole("checkbox", { name: "QA-4 seç" }).check();
    await page.getByRole("toolbar", { name: "Seçilmiş tapşırıqlar üçün əməliyyatlar" }).getByRole("button", { name: "Sil", exact: true }).click();
    const confirm = page.getByRole("dialog", { name: "2 tapşırıq silinsin?" });
    await expect(confirm).toContainText("geri qaytarmaq olmaz");
    await confirm.getByRole("button", { name: "2 tapşırığı sil", exact: true }).click();
    await expect(page.getByRole("status").filter({ hasText: "2 tapşırıq silindi" })).toBeVisible();
    await expect(page.getByRole("row").filter({ hasText: "QA-" })).toHaveCount(2);
    expect((await alice.get(`workspaces/${seed.ws}/projects/QA/items/3`)).status()).toBe(404);
  });

  test("inline edit: status and due date in place, Escape cancels", async ({ page }) => {
    await page.goto(seed.url + "?cols=key,title,status,assignee,due_date");
    const row = page.getByRole("row").filter({ hasText: "QA-1" });

    await row.getByRole("button", { name: "QA-1: Status dəyiş" }).click();
    await row.getByLabel("QA-1: Status dəyiş").selectOption({ label: "In Progress" });
    await expect(row).toContainText("In Progress");
    await expect.poll(async () => (await get(alice, seed.ws, 1)).status.name).toBe("In Progress");
    await expect(row.getByRole("button", { name: "QA-1: Status dəyiş" })).toBeFocused();

    // Escape leaves the value as it was.
    await row.getByRole("button", { name: "QA-1: Son tarix dəyiş" }).click();
    const due = row.getByLabel("QA-1: Son tarix dəyiş");
    await due.fill("2031-07-01");
    await due.press("Escape");
    await expect(row.getByRole("button", { name: "QA-1: Son tarix dəyiş" })).toBeVisible();
    expect((await get(alice, seed.ws, 1)).due_date).toBeNull();

    // Enter saves.
    await row.getByRole("button", { name: "QA-1: Son tarix dəyiş" }).press("Enter");
    await row.getByLabel("QA-1: Son tarix dəyiş").fill("2031-07-01");
    await row.getByLabel("QA-1: Son tarix dəyiş").press("Enter");
    await expect.poll(async () => (await get(alice, seed.ws, 1)).due_date).toBe("2031-07-01");

    // A cell click edits; the title still opens the item.
    await row.getByRole("button", { name: "Birinci" }).click();
    await expect(page.getByRole("dialog", { name: "QA-1" })).toBeVisible();
  });
});
