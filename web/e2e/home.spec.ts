import type { Page } from "@playwright/test";

import { apiAs, authFile, expect, test } from "./fixtures";

// The home page "Mənim işlərim" (ADR-0037), seen by bob (member) across two
// fresh workspaces owned by alice: an overdue item and an item due this
// week in A, a request waiting for bob's approval in B. Other specs give bob
// work too, so counters are checked against the API, and the order of our
// three rows against each other. Synthetic data only.
test.describe.configure({ mode: "serial" });
test.use({ storageState: authFile("bob") });

const run = process.env.E2E_RUN ?? "manual";
const rand = () => Math.random().toString(36).slice(2, 8);
const wsA = `e2e-${run}-homea${rand()}`; // cleaned by global-teardown (slug ^e2e-<run>-[0-9a-z]+$)
const wsB = `e2e-${run}-homeb${rand()}`;
const tag = rand();
const titles = { overdue: `Gecikən akt ${tag}`, approval: `Təsdiq ödənişi ${tag}`, week: `Həftəlik hesabat ${tag}` };

const addDays = (ymd: string, n: number) => {
  const d = new Date(ymd + "T00:00:00Z");
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
};

interface HomeBody {
  counters: { overdue: number; approvals: number; week: number; assigned: number };
  workspaces: { slug: string; today: string }[];
}

async function bobHome(): Promise<HomeBody> {
  const bob = await apiAs("bob");
  const res = await bob.get("me/home?limit=1");
  expect(res.status(), await res.text()).toBe(200);
  const body = (await res.json()) as HomeBody;
  await bob.dispose();
  return body;
}

test.beforeAll(async () => {
  const alice = await apiAs("alice");
  const bob = await apiAs("bob");
  const bobId = ((await (await bob.get("me")).json()) as { id: string }).id;
  for (const [slug, name] of [[wsA, "E2E Ev A"], [wsB, "E2E Ev B"]] as const) {
    expect((await alice.post("workspaces", { data: { slug, name } })).status()).toBe(201);
    expect((await alice.post(`workspaces/${slug}/projects`, { data: { key: "QA", name: "Keyfiyyət" } })).status()).toBe(201);
    expect((await alice.post(`workspaces/${slug}/members`, { data: { email: "bob@example.com", role: "member" } })).status()).toBe(201);
  }
  // Each workspace's own local date decides "overdue" and "this week".
  const today = (await bobHome()).workspaces.find((w) => w.slug === wsA)?.today ?? "";
  expect(today).toMatch(/^\d{4}-\d{2}-\d{2}$/);

  const a = `workspaces/${wsA}/projects/QA/items`;
  for (const data of [
    { title: titles.overdue, assignee_id: bobId, due_date: addDays(today, -3) },
    { title: titles.week, assignee_id: bobId, due_date: addDays(today, 2) },
  ]) {
    const res = await alice.post(a, { data });
    expect(res.status(), await res.text()).toBe(201);
  }

  // B: Done needs one member's approval; alice asks, so bob may decide.
  const wfs = (await (await alice.get(`workspaces/${wsB}/workflows`)).json()) as {
    workflows: { id: string; name: string; statuses: { id: string; name: string }[] }[];
  };
  const wf = wfs.workflows.find((w) => w.name === "Default") ?? wfs.workflows[0];
  const done = wf.statuses.find((s) => s.name === "Done")?.id ?? "";
  const pol = await alice.put(`workspaces/${wsB}/workflows/${wf.id}/statuses/${done}/approval-policy`, {
    data: { min_approvals: 1, approver_roles: ["member"], approver_user_ids: [] },
  });
  expect(pol.status(), await pol.text()).toBe(200);
  expect((await alice.post(`workspaces/${wsB}/projects/QA/items`, { data: { title: titles.approval } })).status()).toBe(201);
  const req = await alice.post(`workspaces/${wsB}/projects/QA/items/1/approvals`, { data: { version: 1, to_status_id: done } });
  expect(req.status(), await req.text()).toBe(201);
  await alice.dispose();
  await bob.dispose();
});

const isHome = (url: URL) => url.pathname === "/api/v1/me/home";
const row = (page: Page, title: string) => page.getByTestId("home-row").filter({ hasText: title });

test("counters match the API and the list puts overdue, approval, this week in order", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1, name: /^Salam/ })).toBeVisible();
  const api = await bobHome();
  expect(api.counters.overdue).toBeGreaterThanOrEqual(1);
  expect(api.counters.approvals).toBeGreaterThanOrEqual(1);
  expect(api.counters.week).toBeGreaterThanOrEqual(1);
  const counters = page.getByRole("navigation", { name: "Sayğaclar" });
  await expect(counters.getByRole("link", { name: `Gecikən: ${String(api.counters.overdue)}` })).toBeVisible();
  await expect(counters.getByRole("link", { name: `Təsdiqimi gözləyən: ${String(api.counters.approvals)}` })).toBeVisible();
  await expect(counters.getByRole("link", { name: `Bu həftə bitir: ${String(api.counters.week)}` })).toBeVisible();
  await expect(counters.getByRole("link", { name: `Mənə təyin olunan: ${String(api.counters.assigned)}` })).toBeVisible();

  // The full list has every row, so our three are all on it.
  await page.goto("/?show=attention");
  await expect(page.getByRole("heading", { name: "Diqqət tələb edən bütün işlər" })).toBeVisible();
  const all = page.getByTestId("home-list").getByTestId("home-row");
  await expect(row(page, titles.week)).toBeVisible();
  const texts = await all.allTextContents();
  const at = (title: string) => texts.findIndex((x) => x.includes(title));
  expect(at(titles.overdue)).toBeGreaterThanOrEqual(0);
  expect(at(titles.overdue)).toBeLessThan(at(titles.approval));
  expect(at(titles.approval)).toBeLessThan(at(titles.week));
  await expect(row(page, titles.overdue)).toHaveAttribute("data-reason", "overdue");
  await expect(row(page, titles.approval)).toHaveAttribute("data-reason", "approval");
  await expect(row(page, titles.week)).toHaveAttribute("data-reason", "week");
  await expect(row(page, titles.overdue)).toContainText("E2E Ev A");
  await expect(row(page, titles.approval)).toContainText("E2E Ev B");

  // Workspace cards: both, with the accessible names other specs rely on.
  const cards = page.getByTestId("workspace-cards");
  await expect(page.getByRole("heading", { name: "Workspace-lər" })).toBeVisible();
  await expect(cards.getByRole("link", { name: /E2E Ev A/ })).toBeVisible();
  await expect(cards.getByRole("link", { name: /E2E Ev B/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "Yeni workspace" })).toBeVisible();
});

test("rows open the item drawer in their project; counters open their list", async ({ page }) => {
  await page.goto("/?show=attention");
  await row(page, titles.overdue).click();
  await expect(page).toHaveURL(new RegExp(`/w/${wsA}/p/QA\\?item=1$`));
  await expect(page.getByRole("dialog", { name: "QA-1" })).toBeVisible();

  await page.goto("/?show=approval");
  await expect(page.getByRole("heading", { name: "Qərarımı gözləyən təsdiqlər" })).toBeVisible();
  await row(page, titles.approval).click();
  await expect(page).toHaveURL(new RegExp(`/w/${wsB}/p/QA\\?item=1&focus=approval$`));
  await expect(page.getByRole("dialog", { name: "QA-1" })).toBeVisible();

  await page.goto("/");
  await page.getByRole("navigation", { name: "Sayğaclar" }).getByRole("link", { name: /^Gecikən:/ }).click();
  await expect(page).toHaveURL(/\/\?show=overdue$/);
  await expect(page.getByRole("heading", { name: "Gecikən işlərim" })).toBeVisible();
  await expect(row(page, titles.overdue)).toBeVisible();
  await expect(row(page, titles.week)).toHaveCount(0);
  await page.getByRole("link", { name: "Mənim işlərim" }).last().click();
  await expect(page).toHaveURL(/\/$/);

  await page.getByTestId("workspace-cards").getByRole("link", { name: /E2E Ev B/ }).click();
  await expect(page).toHaveURL(new RegExp(`/w/${wsB}$`));
});

test("empty states: no workspace yet, and nothing that needs attention", async ({ page }) => {
  // bob has workspaces, so the server's answers for a new user are stubbed.
  const empty = { counters: { overdue: 0, approvals: 0, week: 0, assigned: 0 }, show: "attention", items: [], total: 0, workspaces: [], watching: [], recent_projects: [] };
  await page.route(isHome, (r) => r.fulfill({ json: empty }));
  await page.goto("/");
  await expect(page.getByTestId("home-no-workspaces")).toBeVisible();
  await expect(page.getByText(/keçid e-poçtunuzdadır/)).toBeVisible();
  await page.getByRole("button", { name: "Şablonla workspace yarat" }).click();
  await expect(page.getByRole("dialog", { name: "Yeni workspace" })).toBeVisible();
  await expect(page.getByRole("dialog").getByRole("group", { name: "Komanda növü" })).toBeVisible();
  await page.keyboard.press("Escape");

  await page.unroute(isHome);
  const calm = {
    ...empty,
    counters: { ...empty.counters, assigned: 2 },
    workspaces: [{ id: "x", slug: wsA, name: "E2E Ev A", role: "member", accent: null, timezone: "UTC", today: "2031-01-01", projects: 1, assigned: 2, overdue: 0, week: 0, approvals: 0, last_activity: null }],
    recent_projects: [{ workspace: { slug: wsA, name: "E2E Ev A", accent: null }, project: { key: "QA", name: "Keyfiyyət" }, last_activity: "2031-01-01T10:00:00Z" }],
  };
  await page.route(isHome, (r) => r.fulfill({ json: calm }));
  await page.reload();
  await expect(page.getByTestId("home-calm")).toContainText("Hazırda diqqətinizi tələb edən iş yoxdur");
  await page.getByTestId("home-calm").getByRole("link", { name: /Keyfiyyət · E2E Ev A/ }).click();
  await expect(page).toHaveURL(new RegExp(`/w/${wsA}/p/QA`));
});
