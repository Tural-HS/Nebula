import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// The Status column header filters like a choice field (ADR-0048 menus).
test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  const wf = ((await (await alice.get(`workspaces/${seed.ws}/workflows`)).json()) as { workflows: { statuses: { id: string; category: string }[] }[] }).workflows[0];
  const done = wf?.statuses.find((s) => s.category === "done")?.id;
  for (const title of ["Açıq müştəri", "Bağlı müştəri"]) {
    const it = (await (await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title } })).json()) as { number: number; version: number };
    if (title === "Bağlı müştəri") {
      await alice.patch(`workspaces/${seed.ws}/projects/QA/items/${String(it.number)}`, { data: { version: it.version, status_id: done } });
    }
  }
  await alice.dispose();
});

test("the Status column menu filters by status", async ({ page }) => {
  await page.goto(`${seed.url}?cols=key,title,status`);
  await expect(page.getByRole("row").filter({ hasText: "Bağlı müştəri" })).toHaveCount(1);
  await page.getByRole("button", { name: "Status: filtr və sıralama" }).click();
  const menu = page.getByRole("dialog", { name: "Status: filtr və sıralama" });
  await menu.getByRole("checkbox", { name: "To Do" }).check();
  await expect(page).toHaveURL(/status=/);
  await expect(page.getByRole("row").filter({ hasText: "Bağlı müştəri" })).toHaveCount(0);
  await expect(page.getByRole("row").filter({ hasText: "Açıq müştəri" })).toHaveCount(1);
  await expect(page.getByRole("button", { name: /Status: filtr və sıralama, filtr var/ })).toBeVisible();
});
