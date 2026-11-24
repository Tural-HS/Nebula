import type { APIRequestContext, CDPSession, Locator, Page } from "@playwright/test";

import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// ---------------------------------------------------------------------------
// Phones and tablets (ADR-0019). Runs only in the mobile-* and tablet
// projects of playwright.config.ts (device emulation with touch). Touch input
// goes through the Chrome DevTools Protocol, so these are real touch events
// with the browser's own scrolling, not synthetic DOM events.
// ---------------------------------------------------------------------------

test.use({ storageState: authFile("alice") });

const isPhone = (page: Page) => (page.viewportSize()?.width ?? 0) < 768;

/** The API rate-limits each user; wait as told instead of loosening the limit. */
async function create(api: APIRequestContext, ws: string, data: Record<string, unknown>): Promise<void> {
  for (let attempt = 0; attempt < 20; attempt++) {
    const r = await api.post(`workspaces/${ws}/projects/QA/items`, { data });
    if (r.status() !== 429) {
      expect(r.status()).toBe(201);
      return;
    }
    const wait = Number(r.headers()["retry-after"] ?? "1");
    await new Promise((res) => setTimeout(res, Math.min(Math.max(wait, 1), 10) * 1000));
  }
  throw new Error("rate limited");
}

async function center(l: Locator): Promise<{ x: number; y: number }> {
  const b = await l.boundingBox();
  if (!b) throw new Error("layout not ready");
  return { x: b.x + b.width / 2, y: b.y + b.height / 2 };
}

/** One finger. Coordinates are CSS pixels in the viewport. */
class Finger {
  private constructor(private readonly cdp: CDPSession) {}
  static async on(page: Page): Promise<Finger> {
    return new Finger(await page.context().newCDPSession(page));
  }
  private point(x: number, y: number) {
    return [{ x: Math.round(x), y: Math.round(y), id: 1, radiusX: 4, radiusY: 4, force: 1 }];
  }
  async down(x: number, y: number) {
    await this.cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: this.point(x, y) });
  }
  async move(from: { x: number; y: number }, to: { x: number; y: number }, steps = 10) {
    for (let i = 1; i <= steps; i++) {
      await this.cdp.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: this.point(from.x + ((to.x - from.x) * i) / steps, from.y + ((to.y - from.y) * i) / steps),
      });
    }
  }
  async up() {
    await this.cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  }
  async detach() {
    await this.cdp.detach();
  }
}

/** Nothing on the page is wider than the screen (no sideways page scroll). */
async function expectNoHorizontalOverflow(page: Page) {
  await expect(page.locator("main")).toBeVisible();
  await expect(page.locator("main [role=status]").filter({ hasText: "Yüklənir" })).toHaveCount(0);
  // Compare with the device width, not innerWidth: on a mobile viewport a
  // too-wide page widens the layout viewport itself (innerWidth grows with
  // it), so innerWidth alone hides the overflow.
  const device = page.viewportSize()?.width ?? 0;
  await expect
    .poll(() => page.evaluate((w) => {
      const root = document.scrollingElement ?? document.documentElement;
      return Math.max(root.scrollWidth, document.body.scrollWidth, window.innerWidth) - w;
    }, device), { message: `horizontal overflow on ${page.url()}` })
    .toBeLessThanOrEqual(0);
}

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  await create(alice, seed.ws, { title: "Toxunuşla sürüklə" });
  await create(alice, seed.ws, { title: "Mənim tapşırığım", assignee_id: seed.aliceId });
  await create(alice, seed.ws, { title: "Başqasının tapşırığı", assignee_id: seed.bobId });
  // A long unbroken title must wrap or truncate, never widen the page.
  await create(alice, seed.ws, { title: "Çoxuzunbaşlıqboşluqsuz".repeat(12) });
  await alice.dispose();
});

test.describe("board by touch", () => {
  test("a swipe scrolls the columns and does not pick a card up", async ({ page }) => {
    test.skip(!isPhone(page), "phones only: on a tablet all columns fit");
    await page.goto(seed.url + "?view=board");
    const todo = page.getByRole("region", { name: "To Do" });
    const card = todo.getByText("Toxunuşla sürüklə");
    await expect(card).toBeVisible();
    const scroller = page.getByTestId("board-scroller");
    expect(await scroller.evaluate((el) => el.scrollLeft)).toBe(0);

    const finger = await Finger.on(page);
    const start = await center(card);
    await finger.down(start.x, start.y);
    await finger.move(start, { x: start.x - 220, y: start.y }, 8); // quick, no long press
    await finger.up();
    await finger.detach();

    // The board scrolled (and snapped to a column); the card did not move.
    await expect.poll(() => scroller.evaluate((el) => el.scrollLeft)).toBeGreaterThan(100);
    await expect(todo.getByText("Toxunuşla sürüklə")).toHaveCount(1);
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("a long press lifts a card; dropping it on another column changes the status", async ({ page }) => {
    await page.goto(seed.url + "?view=board");
    const todo = page.getByRole("region", { name: "To Do" });
    const doing = page.getByRole("region", { name: "In Progress" });
    const card = todo.getByText("Toxunuşla sürüklə");
    await expect(card).toBeVisible();
    await expect(doing.getByRole("heading")).toContainText("0");
    const width = page.viewportSize()?.width ?? 0;
    const scroller = page.getByTestId("board-scroller");

    const finger = await Finger.on(page);
    const start = await center(card);
    await finger.down(start.x, start.y);
    await page.waitForTimeout(400); // past the 250 ms press delay
    let at = { x: start.x + 10, y: start.y + 10 };
    await finger.move(start, at, 3);

    if (isPhone(page)) {
      // Only one column fits: hold near the right edge until auto-scroll
      // brings "In Progress" to the middle, then move the finger onto it,
      // out of the auto-scroll zone.
      const edge = { x: width * 0.9, y: at.y };
      await finger.move(at, edge, 6);
      at = edge;
      await expect
        .poll(() => doing.evaluate((el) => el.getBoundingClientRect().left), { timeout: 5_000, intervals: [25] })
        .toBeLessThan(width * 0.5);
    }
    const target = { x: Math.min((await center(doing)).x, width * 0.5), y: start.y + 40 };
    await finger.move(at, target, 6);
    // Scroll snapping is off while a card is lifted.
    await expect(scroller).not.toHaveClass(/snap-mandatory/);
    await finger.up();
    await finger.detach();

    await expect(doing.getByText("Toxunuşla sürüklə")).toBeVisible();
    await expect(todo.getByText("Toxunuşla sürüklə")).toHaveCount(0);
    await expect(doing.getByRole("heading")).toContainText("1");
    // The tap at the end of the drag did not open the item.
    await expect(page.getByRole("dialog")).toHaveCount(0);

    const alice = await apiAs("alice");
    const item = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/items/1`)).json()) as { status: { name: string } };
    expect(item.status.name).toBe("In Progress");
    await alice.dispose();
  });

  test("columns the workflow forbids are dimmed during a touch drag", async ({ page }) => {
    test.skip(isPhone(page), "tablet: both columns are on screen at once");
    const alice = await apiAs("alice");
    const wfs = (await (await alice.get(`workspaces/${seed.ws}/workflows`)).json()) as { workflows: { id: string; statuses: { id: string; name: string }[] }[] };
    const wf = wfs.workflows.at(0);
    if (!wf) throw new Error("no workflow");
    const id = (name: string) => wf.statuses.find((s) => s.name === name)?.id;
    expect((await alice.post(`workspaces/${seed.ws}/workflows/${wf.id}/transitions`, {
      data: { name: "Başla", from_status_id: id("To Do"), to_status_id: id("In Progress") },
    })).ok()).toBe(true);
    expect((await alice.patch(`workspaces/${seed.ws}/workflows/${wf.id}`, { data: { enforce_transitions: true } })).ok()).toBe(true);
    try {
      await page.goto(seed.url + "?view=board");
      const todo = page.getByRole("region", { name: "To Do" });
      const done = page.getByRole("region", { name: "Done" });
      const card = todo.getByText("Mənim tapşırığım");
      await expect(card).toBeVisible();

      const finger = await Finger.on(page);
      const start = await center(card);
      await finger.down(start.x, start.y);
      await page.waitForTimeout(400);
      const target = { x: Math.min((await center(done)).x, (page.viewportSize()?.width ?? 0) * 0.7), y: start.y + 40 };
      await finger.move(start, target, 12);
      await expect(done).toHaveClass(/opacity-40/);
      await expect(page.getByRole("region", { name: "In Progress" })).not.toHaveClass(/opacity-40/);
      await finger.up();
      await finger.detach();

      await expect(page.getByText("Workflow bu status keçidinə icazə vermir.")).toBeVisible();
      await expect(todo.getByText("Mənim tapşırığım")).toBeVisible();
    } finally {
      await alice.patch(`workspaces/${seed.ws}/workflows/${wf.id}`, { data: { enforce_transitions: false } });
      await alice.dispose();
    }
  });
});

test("the item panel fills a phone screen and closes with a large button", async ({ page }) => {
  await page.goto(seed.url);
  await page.getByRole("row").filter({ hasText: "Mənim tapşırığım" }).getByRole("button").tap();
  const panel = page.getByRole("dialog", { name: /^QA-\d+$/ });
  await expect(panel).toBeVisible();
  await expect(page).toHaveURL(/item=\d+/);

  const box = await panel.boundingBox();
  const vp = page.viewportSize();
  if (!box || !vp) throw new Error("layout not ready");
  if (isPhone(page)) {
    expect(box.x).toBe(0);
    expect(Math.round(box.width)).toBe(vp.width);
  }
  const close = panel.getByRole("button", { name: "Bağla" });
  const closeBox = await close.boundingBox();
  expect(closeBox?.height ?? 0).toBeGreaterThanOrEqual(44);
  expect(closeBox?.width ?? 0).toBeGreaterThanOrEqual(44);
  await expectNoHorizontalOverflow(page);

  await close.tap();
  await expect(panel).toHaveCount(0);
  await expect(page).not.toHaveURL(/item=/);
});

test("phone toolbar: one row, filters in a sheet", async ({ page }) => {
  test.skip(!isPhone(page), "phones only: wider screens keep the full toolbar");
  await page.goto(seed.url);
  await expect(page.getByRole("row").filter({ hasText: "Başqasının tapşırığı" })).toBeVisible();

  // Everything but search, the filter button and the view switch is in the sheet.
  await expect(page.getByRole("combobox", { name: "İcraçı", exact: true })).toHaveCount(0);
  const toolbar = page.getByRole("searchbox", { name: "Axtar…" }).locator("..");
  const bar = await toolbar.boundingBox();
  expect(bar?.height ?? 999).toBeLessThan(64); // a single row

  await page.getByRole("button", { name: "Filtrlər" }).tap();
  const sheet = page.getByRole("dialog", { name: "Filtrlər və görünüş" });
  await expect(sheet).toBeVisible();
  await sheet.getByRole("combobox", { name: "İcraçı", exact: true }).selectOption("me"); // not the column checkbox
  await expect(page).toHaveURL(/assignee=me/);
  await sheet.getByRole("button", { name: "Bağla" }).tap();
  await expect(sheet).toBeHidden();

  await expect(page.getByRole("button", { name: /Filtrlər\s*1/ })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Mənim tapşırığım" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Başqasının tapşırığı" })).toHaveCount(0);

  // Search stays in the toolbar.
  await page.getByRole("searchbox", { name: "Axtar…" }).fill("Mənim");
  await expect(page).toHaveURL(/q=M/);
  await expect(page.getByRole("row").filter({ hasText: "Mənim tapşırığım" })).toBeVisible();
});

test("no page scrolls sideways", async ({ page }) => {
  test.setTimeout(120_000);
  const project = seed.url;
  const ws = `/w/${seed.ws}`;
  const pages = [
    "/",
    "/?show=assigned", // the home page's full list (ADR-0037)
    ws,
    project,
    project + "?view=board",
    project + "/recurring",
    project + "/import",
    project + "/settings",
    ...["general", "members", "fields", "item-types", "workflows", "webhooks", "automations", "audit"].map((s) => `${ws}/settings/${s}`),
  ];
  for (const url of pages) {
    await page.goto(url);
    await expectNoHorizontalOverflow(page);
  }
});

test("the language picker is in the header from tablet width, on the home page on phones", async ({ page }) => {
  await page.goto("/");
  const header = page.getByRole("banner").getByLabel("Dil");
  const list = page.getByRole("main").getByLabel("Dil");
  if ((page.viewportSize()?.width ?? 0) < 640) {
    await expect(header).toBeHidden();
    await expect(list).toBeVisible();
    expect((await list.boundingBox())?.height).toBeGreaterThanOrEqual(44); // touch target (ADR-0019)
    const add = page.getByRole("button", { name: "Yeni workspace" });
    await expect(add.getByText("+")).toBeVisible(); // long labels (ru) would push the header out
    await expect(add.getByText("Yeni workspace")).toBeHidden();
  } else {
    await expect(header).toBeVisible();
    await expect(list).toBeHidden();
  }
});
