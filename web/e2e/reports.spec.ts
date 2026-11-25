import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Workspace reports "Hesabatlar" (ADR-0041), seen by bob (member) in a fresh
// workspace owned by alice: in QA three open items (two overdue, both bob's)
// and one completed today; in a private project SEC an overdue item bob
// must never see counted. Synthetic data only.
test.describe.configure({ mode: "serial" });
test.use({ storageState: authFile("bob") });

let seed: Seed;
const tag = Math.random().toString(36).slice(2, 8);

const addDays = (ymd: string, n: number) => {
  const d = new Date(ymd + "T00:00:00Z");
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
};

test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const alice = await apiAs("alice");
  const ws = (await (await alice.get(`workspaces/${seed.ws}`)).json()) as { timezone: string };
  const today = new Intl.DateTimeFormat("en-CA", { timeZone: ws.timezone, year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date());
  const items = `workspaces/${seed.ws}/projects/QA/items`;
  for (const data of [
    { title: `Akt-üzləşmə ${tag}`, assignee_id: seed.bobId, due_date: addDays(today, -3) },
    { title: `ƏDV bəyannaməsi ${tag}`, assignee_id: seed.bobId, due_date: addDays(today, -40) },
    { title: `Kassa ${tag}` },
    { title: `Bağlanış ${tag}`, assignee_id: seed.bobId },
  ]) {
    const res = await alice.post(items, { data });
    expect(res.status(), await res.text()).toBe(201);
  }
  const wfs = (await (await alice.get(`workspaces/${seed.ws}/workflows`)).json()) as { workflows: { name: string; statuses: { id: string; name: string }[] }[] };
  const done = wfs.workflows.find((w) => w.name === "Default")?.statuses.find((s) => s.name === "Done")?.id ?? "";
  const moved = await alice.patch(`${items}/4`, { data: { version: 1, status_id: done } });
  expect(moved.status(), await moved.text()).toBe(200);

  expect((await alice.post(`workspaces/${seed.ws}/projects`, { data: { key: "SEC", name: "Hüquq", visibility: "private" } })).status()).toBe(201);
  const sec = await alice.post(`workspaces/${seed.ws}/projects/SEC/items`, { data: { title: `Gizli ${tag}`, due_date: addDays(today, -5) } });
  expect(sec.status(), await sec.text()).toBe(201);
  await alice.dispose();
});

test("numbers are visible, private projects are not counted, a drill-down opens the filtered list", async ({ page }) => {
  await page.goto(`/w/${seed.ws}`);
  await page.getByRole("link", { name: "Hesabatlar" }).click();
  await expect(page).toHaveURL(new RegExp(`/w/${seed.ws}/reports$`));
  await expect(page.getByRole("heading", { level: 1, name: "Hesabatlar" })).toBeVisible();

  await expect(page.getByTestId("kpi-open-value")).toHaveText("3");
  await expect(page.getByTestId("kpi-overdue-value")).toHaveText("2"); // SEC's overdue item is not bob's to see
  await expect(page.getByTestId("kpi-created-value")).toHaveText("4");
  await expect(page.getByTestId("kpi-completed-value")).toHaveText("1");
  await expect(page.getByTestId("kpi-awaiting-value")).toHaveText("0");
  await expect(page.getByTestId("status-SEC")).toHaveCount(0);
  await expect(page.getByTestId("aging-late").getByText("31–90 gün")).toBeVisible();

  // Workload: bob has 2 open (both overdue) and 1 completed; the unassigned row 1 open.
  const bobRow = page.getByTestId("workload-row").filter({ hasText: "Bob Developer" });
  await expect(bobRow.getByRole("cell")).toHaveText(["2", "2", "1"]);
  await expect(page.getByTestId("workload-row").filter({ hasText: "Təyin olunmayıb" }).getByRole("cell")).toHaveText(["1", "0", "0"]);

  // The tile links into QA's list with the same filter.
  await page.getByTestId("kpi-overdue").getByRole("link", { name: /Siyahını aç/ }).click();
  await expect(page).toHaveURL(new RegExp(`/w/${seed.ws}/p/QA\\?due=overdue$`));
  await expect(page.getByText(`Akt-üzləşmə ${tag}`).first()).toBeVisible();
  await expect(page.getByText(`Kassa ${tag}`)).toHaveCount(0);
});

test("filters live in the URL; the workload link applies the assignee", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/reports`);
  await page.getByRole("combobox", { name: "İcraçı" }).selectOption({ label: "Təyin olunmayıb" });
  await expect(page).toHaveURL(/assignee=none/);
  await expect(page.getByTestId("kpi-open-value")).toHaveText("1");
  await page.getByRole("combobox", { name: "İcraçı" }).selectOption({ label: "Hamı" });
  await expect(page.getByTestId("kpi-open-value")).toHaveText("3");

  await page.getByRole("combobox", { name: "Dövr" }).selectOption({ label: "Keçən ay" });
  await expect(page).toHaveURL(/range=last_month/);
  await expect(page.getByTestId("kpi-created-value")).toHaveText("0");
  await expect(page.getByTestId("kpi-open-value")).toHaveText("3"); // "now" numbers do not depend on the period
  await page.getByRole("combobox", { name: "Dövr" }).selectOption({ label: "Bu ay" });

  const bobRow = page.getByTestId("workload-row").filter({ hasText: "Bob Developer" });
  await bobRow.getByRole("link", { name: "2" }).first().click();
  await expect(page).toHaveURL(new RegExp(`/w/${seed.ws}/p/QA\\?assignee=${seed.bobId}&status=`));
  await expect(page.getByText(`Bağlanış ${tag}`)).toHaveCount(0); // done: not open
  await expect(page.getByText(`ƏDV bəyannaməsi ${tag}`).first()).toBeVisible();
});

test("phone: 320 px without horizontal scroll", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 720 });
  await page.goto(`/w/${seed.ws}/reports`);
  await expect(page.getByTestId("kpi-open-value")).toHaveText("3");
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBeLessThanOrEqual(0);
  // The chart has a table alternative.
  await page.getByTestId("w-throughput").getByText("Cədvəl kimi göstər").click();
  await expect(page.getByTestId("w-throughput").getByRole("table")).toBeVisible();
});
