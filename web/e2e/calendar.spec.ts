import type { APIRequestContext, Locator, Page } from "@playwright/test";

import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

// March 2031 starts on a Saturday: the grid runs from Mon 24 Feb to Sun 6 Apr.
const MONTH = "2031-03";

async function create(api: APIRequestContext, ws: string, title: string, due?: string) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const r = await api.post(`workspaces/${ws}/projects/QA/items`, { data: { title, due_date: due } });
    if (r.status() !== 429) {
      expect(r.status()).toBe(201);
      return;
    }
    await new Promise((res) => setTimeout(res, Math.min(Number(r.headers()["retry-after"] ?? "1"), 10) * 1000));
  }
  throw new Error("rate limited");
}

async function dueOf(api: APIRequestContext, ws: string, n: number): Promise<string | null> {
  return ((await (await api.get(`workspaces/${ws}/projects/QA/items/${String(n)}`)).json()) as { due_date: string | null }).due_date;
}

/** dnd-kit needs real pointer movement past its activation distance. */
async function dragTo(page: Page, from: Locator, to: Locator) {
  const a = await from.boundingBox();
  const b = await to.boundingBox();
  if (!a || !b) throw new Error("layout not ready");
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2);
  await page.mouse.down();
  await page.mouse.move(a.x + a.width / 2 + 15, a.y + a.height / 2 + 15, { steps: 5 });
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 15 });
  await page.mouse.up();
}

const day = (page: Page, d: string) => page.locator(`[role="gridcell"][data-day="${d}"]`);

test.describe("calendar", () => {
  let seed: Seed;
  let alice: APIRequestContext;
  test.beforeAll(async () => {
    seed = await seedWorkspace();
    alice = await apiAs("alice");
    await create(alice, seed.ws, "Mart hesabatı", "2031-03-10"); // QA-1
    for (const n of [1, 2, 3, 4]) await create(alice, seed.ws, `Sıx gün ${String(n)}`, "2031-03-12"); // QA-2..5
    await create(alice, seed.ws, "Tarixsiz iş"); // QA-6
    await create(alice, seed.ws, "Köhnə borc", "2020-03-05"); // QA-7, overdue
  });
  test.afterAll(async () => { await alice.dispose(); });

  test("month grid: placement, +N more, open, navigate", async ({ page }) => {
    await page.goto(`${seed.url}?view=calendar&cal=${MONTH}`);
    await expect(page.getByTestId("calendar-month")).toHaveText("Mart 2031");
    await expect(page.getByRole("tab", { name: "Təqvim" })).toHaveAttribute("aria-selected", "true");
    // Monday first: the first cell is Monday 24 February.
    await expect(page.getByRole("gridcell").first()).toHaveAttribute("data-day", "2031-02-24");
    await expect(day(page, "2031-03-10")).toContainText("Mart hesabatı");

    // Four items on one day: two shown and "+2 daha", which lists all four.
    const busy = day(page, "2031-03-12");
    await expect(busy.getByTestId("calendar-item")).toHaveCount(2);
    await busy.getByRole("button", { name: "+2 daha" }).click();
    const list = page.getByRole("dialog", { name: /12 mar 2031/i });
    await expect(list.getByTestId("calendar-item")).toHaveCount(4);
    await list.getByText("Sıx gün 4").click();
    await expect(page.getByRole("dialog", { name: "QA-5" })).toBeVisible();
    await expect(page).toHaveURL(/item=5/);
    await page.getByRole("dialog", { name: "QA-5" }).getByRole("button", { name: "Bağla" }).click();

    // Undated items are beside the grid.
    await expect(page.getByTestId("calendar-undated")).toContainText("Tarixsiz iş");

    await page.getByRole("button", { name: "Növbəti ay" }).click();
    await expect(page).toHaveURL(/cal=2031-04/);
    await expect(page.getByTestId("calendar-month")).toHaveText("Aprel 2031");
    await page.getByRole("button", { name: "Bu ay" }).click();
    await expect(page).not.toHaveURL(/cal=/);
  });

  test("overdue items are marked", async ({ page }) => {
    await page.goto(`${seed.url}?view=calendar&cal=2020-03`);
    const late = day(page, "2020-03-05").getByTestId("calendar-item");
    await expect(late).toHaveAttribute("data-overdue", "true");
    await expect(late).toContainText("gecikir");
  });

  test("drag to another day changes the due date; drag from the side list sets one", async ({ page }) => {
    await page.goto(`${seed.url}?view=calendar&cal=${MONTH}`);
    await dragTo(page, day(page, "2031-03-10").getByText("Mart hesabatı"), day(page, "2031-03-20"));
    await expect(day(page, "2031-03-20")).toContainText("Mart hesabatı");
    await expect(page.getByRole("status").filter({ hasText: "QA-1: son tarix" })).toBeVisible();
    await expect.poll(() => dueOf(alice, seed.ws, 1)).toBe("2031-03-20");

    await dragTo(page, page.getByTestId("calendar-undated").getByText("Tarixsiz iş"), day(page, "2031-03-05"));
    await expect(day(page, "2031-03-05")).toContainText("Tarixsiz iş");
    await expect.poll(() => dueOf(alice, seed.ws, 6)).toBe("2031-03-05");
    await expect(page.getByTestId("calendar-undated")).not.toContainText("Tarixsiz iş");

    // Back to the side list clears it again.
    await dragTo(page, day(page, "2031-03-05").getByText("Tarixsiz iş"), page.getByTestId("calendar-undated"));
    await expect.poll(() => dueOf(alice, seed.ws, 6)).toBeNull();
  });

  test("a calendar can be saved as a view; the month is not part of it", async ({ page }) => {
    await page.goto(`${seed.url}?view=calendar&cal=${MONTH}`);
    await page.getByRole("button", { name: "Görünüş kimi saxla" }).click();
    const dialog = page.getByRole("dialog", { name: "Yeni görünüş" });
    await dialog.getByLabel("Ad").fill("Təqvim planı");
    await dialog.getByRole("button", { name: "Yadda saxla" }).click();
    await expect(page).toHaveURL(/saved=/);
    // Another month is navigation, not a change of the view.
    await page.getByRole("button", { name: "Növbəti ay" }).click();
    await expect(page.getByText("dəyişdirilib")).toHaveCount(0);
    const views = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/views`)).json()) as { views: { name: string; kind: string }[] };
    expect(views.views.find((v) => v.name === "Təqvim planı")?.kind).toBe("calendar");
  });

  test.describe("phone", () => {
    test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

    test("shows an agenda of the month instead of the grid", async ({ page }) => {
      await page.goto(`${seed.url}?view=calendar&cal=${MONTH}`);
      const agenda = page.getByTestId("calendar-agenda");
      await expect(agenda).toBeVisible();
      await expect(page.getByTestId("calendar-grid")).toHaveCount(0);
      await expect(agenda.getByRole("region", { name: /12 mar 2031/i }).getByTestId("calendar-item")).toHaveCount(4);
      await agenda.getByText("Sıx gün 1").tap();
      await expect(page.getByRole("dialog", { name: "QA-2" })).toBeVisible();
    });
  });
});
