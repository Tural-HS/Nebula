import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// ADR-0061: completion fields and overdue-coloured fields.
test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  for (const [key, name, type] of [["paid_e2e", "Ödədi", "boolean"], ["portal_e2e", "Portal", "boolean"], ["bal_e2e", "Qalıq", "number"]] as const) {
    expect((await alice.post(`workspaces/${seed.ws}/fields`, { data: { key, name, type } })).status()).toBe(201);
  }
  expect((await alice.put(`workspaces/${seed.ws}/item-types/task/fields/bal_e2e`, { data: {} })).ok()).toBe(true);
  const d = new Date();
  d.setDate(d.getDate() - 20);
  const due = `${String(d.getFullYear())}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Gecikən müştəri", due_date: due, fields: { bal_e2e: 500 } } })).status()).toBe(201);
  await alice.dispose();
});

test("admins set the rules; overdue cells take the band colour", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/p/QA/settings`);
  const section = page.getByRole("region", { name: /Tamamlanma və gecikmə qaydaları/ });
  const complete = section.getByRole("group", { name: /Tamamlanma sahələri/ });
  await complete.getByRole("checkbox", { name: "Ödədi" }).check();
  await complete.getByRole("checkbox", { name: "Portal" }).check();
  await section.getByRole("group", { name: /Gecikmə rəngi olan sahələr/ }).getByRole("checkbox", { name: "Qalıq" }).check();
  await section.getByRole("button", { name: "Yadda saxla" }).click();
  await expect(page.getByText("Qaydalar yadda saxlanıldı")).toBeVisible();

  const alice = await apiAs("alice");
  const rules = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/rules`)).json()) as { complete_fields: string[]; late_fields: string[] };
  expect(rules.complete_fields.sort()).toEqual(["paid_e2e", "portal_e2e"]);
  expect(rules.late_fields).toEqual(["bal_e2e"]);
  await alice.dispose();

  // 20 days late: orange title and balance cells.
  await page.goto(`${seed.url}?cols=key,title,field.bal_e2e,due_date`);
  const row = page.getByRole("row").filter({ hasText: "Gecikən müştəri" });
  await expect(row.locator("[data-late='2']")).toHaveCount(3); // title, balance, due chip
});

test("the summary bar totals the listed items (ADR-0064)", async ({ page }) => {
  const alice = await apiAs("alice");
  expect((await alice.post(`workspaces/${seed.ws}/fields`, { data: { key: "monthly_e2e", name: "Aylıq", type: "number" } })).status()).toBe(201);
  expect((await alice.put(`workspaces/${seed.ws}/item-types/task/fields/monthly_e2e`, { data: {} })).ok()).toBe(true);
  for (const [title, monthly, bal] of [["Bir", 1000, 400], ["İki", 500, 0]] as const) {
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title, fields: { monthly_e2e: monthly, bal_e2e: bal } } })).status()).toBe(201);
  }
  expect((await alice.put(`workspaces/${seed.ws}/projects/QA/rules`, { data: { complete_fields: [], late_fields: [], summary: [
    { label: "Aylıq ödəniş cəmi", field: "monthly_e2e", unit: "₼" },
    { label: "Ödənilib", field: "monthly_e2e", minus: "bal_e2e", unit: "₼" },
  ] } })).ok()).toBe(true);
  await alice.dispose();
  await page.goto(seed.url);
  const bar = page.getByTestId("summary-bar");
  await expect(bar).toContainText("hesabatı");
  await expect(bar).toContainText(/Aylıq ödəniş cəmi:\s*1\s500\s₼/);
  // 1500 − (400 + 0 + the overdue item's 500).
  await expect(bar).toContainText(/Ödənilib:\s*600\s₼/);
});

test("a project chooses its counters; 'due this month' counts open items (ADR-0065)", async ({ page }) => {
  const alice = await apiAs("alice");
  expect((await alice.put(`workspaces/${seed.ws}/projects/QA/rules`, { data: { complete_fields: [], late_fields: [], summary: [], queues: ["all", "overdue", "done", "month"] } })).ok()).toBe(true);
  await alice.dispose();
  await page.goto(seed.url);
  await expect(page.getByRole("button", { name: /Tamamlanmış/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Bu ay bitir/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Mənə təyin/ })).toHaveCount(0);
  await page.getByRole("button", { name: /Bu ay bitir/ }).click();
  await expect(page).toHaveURL(/due=month/);
  await expect(page).toHaveURL(/category=todo/);
});

test("the report panel shows plan, paid, lateness and the longest debt (ADR-0066)", async ({ page }) => {
  const alice = await apiAs("alice");
  expect((await alice.put(`workspaces/${seed.ws}/projects/QA/rules`, { data: {
    complete_fields: [], late_fields: ["bal_e2e"], summary: [], queues: [],
    report: { plan_field: "monthly_e2e", remaining_field: "bal_e2e" },
  } })).ok()).toBe(true);
  await alice.dispose();
  await page.goto(seed.url);
  const panel = page.getByTestId("report-panel");
  await expect(panel.getByRole("img", { name: "40%" })).toBeVisible();
  await expect(panel).toContainText(/Aylıq plan\s*1\s500\s₼/);
  await expect(panel).toContainText(/Ödənilib\s*600\s₼/);
  await expect(panel).toContainText(/Yığılası qalıq\s*900\s₼/);
  await expect(panel).toContainText("1 · 16–30 gün");
  await expect(panel).toContainText("20 gün");
  await page.screenshot({ path: process.env.REPORT_SHOT ?? "test-results/report-panel.png", clip: { x: 0, y: 0, width: 1280, height: 420 } });
  // Collapse to one line; the choice survives a reload.
  await panel.getByRole("button", { name: "Paneli yığ" }).click();
  await expect(panel).toContainText(/Plan\s*1\s500\s₼/);
  await expect(panel).toContainText("1 gecikir");
  await page.reload();
  await expect(page.getByTestId("report-panel").getByRole("button", { name: "Paneli aç" })).toBeVisible();
  await expect(page.getByTestId("report-panel")).toContainText(/Plan\s*1\s500/);
  await page.screenshot({ path: (process.env.REPORT_SHOT ?? "test-results/report-panel.png").replace(".png", "-collapsed.png"), clip: { x: 0, y: 0, width: 1280, height: 260 } });
  await page.getByTestId("report-panel").getByRole("button", { name: "Paneli aç" }).click();
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(page.getByTestId("report-panel")).toContainText(/Ödənilib\s*600/);
  await page.screenshot({ path: (process.env.REPORT_SHOT ?? "test-results/report-panel.png").replace(".png", "-dark.png"), clip: { x: 0, y: 0, width: 1280, height: 420 } });
});
