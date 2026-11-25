import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Subtasks (ADR-0021): drawer list + inline add, progress, parent breadcrumb,
// table/board badges and the activity line for a parent change.
test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  const create = async (data: Record<string, unknown>) => {
    const res = await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data });
    expect(res.status(), await res.text()).toBe(201);
  };
  await create({ title: "Buraxılış 2.0" }); // QA-1
  await create({ title: "Testləri yaz", parent: "QA-1" }); // QA-2
  await create({ title: "Ayrıca tapşırıq" }); // QA-3
  await alice.dispose();
});

test("add subtasks inline, follow the breadcrumb and see progress", async ({ page }) => {
  await page.goto(seed.url + "?item=1");
  const panel = page.getByRole("dialog", { name: "QA-1" });
  const section = panel.getByRole("region", { name: "Alt tapşırıqlar" });
  await expect(section.getByRole("link", { name: /QA-2.*Testləri yaz/ })).toBeVisible();
  await expect(section.getByRole("progressbar", { name: "0/1 tamamlanıb" })).toBeVisible();

  await section.getByLabel("Alt tapşırıq əlavə et").fill("Sənədləri yenilə");
  await section.getByLabel("Alt tapşırıq əlavə et").press("Enter");
  await expect(section.getByRole("link", { name: /QA-4.*Sənədləri yenilə/ })).toBeVisible();
  await expect(section.getByRole("progressbar", { name: "0/2 tamamlanıb" })).toBeVisible();
  await expect(section.getByLabel("Alt tapşırıq əlavə et")).toHaveValue("");

  // Open a subtask: it shows its parent and has no subtask section itself.
  await section.getByRole("link", { name: /QA-2/ }).click();
  const sub = page.getByRole("dialog", { name: "QA-2" });
  const crumb = sub.getByRole("navigation", { name: "Əsas tapşırıq" });
  await expect(crumb.getByRole("link", { name: /QA-1.*Buraxılış 2.0/ })).toBeVisible();
  await expect(sub.getByRole("region", { name: "Alt tapşırıqlar" })).toHaveCount(0);
  await sub.getByLabel("Status").selectOption({ label: "Done" });

  await crumb.getByRole("link", { name: /QA-1/ }).click();
  await expect(page).toHaveURL(/item=1/);
  await expect(
    page.getByRole("dialog", { name: "QA-1" }).getByRole("progressbar", { name: "1/2 tamamlanıb" }),
  ).toBeVisible();
});

test("table and board show a progress badge on parents", async ({ page }) => {
  const alice = await apiAs("alice");
  const res = await alice.get(`workspaces/${seed.ws}/projects/QA/items/1`);
  const { subtasks } = (await res.json()) as { subtasks: { count: number; done: number } };
  await alice.dispose();
  const label = `Alt tapşırıqlar: ${String(subtasks.done)}/${String(subtasks.count)} tamamlanıb`;

  await page.goto(seed.url);
  await expect(page.getByRole("row").filter({ hasText: "Buraxılış 2.0" }).getByLabel(label)).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Ayrıca tapşırıq" }).getByLabel(/^Alt tapşırıqlar:/)).toHaveCount(0);

  await page.goto(seed.url + "?view=board");
  await expect(page.getByRole("region", { name: "To Do" }).getByLabel(label)).toBeVisible();
});

test("detaching a subtask is recorded in its history", async ({ page }) => {
  await page.goto(seed.url + "?item=4");
  const sub = page.getByRole("dialog", { name: "QA-4" });
  await sub.getByRole("button", { name: "Əsas tapşırıqdan ayır" }).click();
  await expect(sub.getByRole("navigation", { name: "Əsas tapşırıq" })).toHaveCount(0);
  // Now top-level, it may get subtasks of its own (behind "+ Alt tapşırıq" while it has none).
  await sub.getByRole("button", { name: "+ Alt tapşırıq" }).click();
  await expect(sub.getByRole("region", { name: "Alt tapşırıqlar" })).toBeVisible();

  await sub.getByRole("tab", { name: "Aktivlik" }).click();
  await expect(sub.getByText("əsas tapşırığı dəyişdi")).toBeVisible();
  await expect(sub.getByText("QA-1", { exact: true })).toBeVisible();
});
