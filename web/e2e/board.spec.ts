import type { APIRequestContext, Page } from "@playwright/test";

import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

interface Wf {
  id: string;
  statuses: { id: string; name: string }[];
}

async function workflow(api: APIRequestContext, ws: string): Promise<Wf> {
  const body = (await (await api.get(`workspaces/${ws}/workflows`)).json()) as { workflows: Wf[] };
  const wf = body.workflows.at(0);
  if (!wf) throw new Error("no workflow");
  return wf;
}

/** dnd-kit needs real pointer movement past its activation distance. */
async function dragTo(page: Page, card: ReturnType<Page["getByText"]>, column: ReturnType<Page["getByRole"]>, onHover?: () => Promise<void>) {
  const from = await card.boundingBox();
  const to = await column.boundingBox();
  if (!from || !to) throw new Error("layout not ready");
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(from.x + from.width / 2 + 20, from.y + 20, { steps: 5 });
  await page.mouse.move(to.x + to.width / 2, to.y + 80, { steps: 15 });
  if (onHover) await onHover();
  await page.mouse.up();
}

/** The API rate-limits each user; wait as told instead of loosening the limit. */
async function createRespectingRateLimit(api: APIRequestContext, ws: string, title: string): Promise<number> {
  for (let attempt = 0; attempt < 20; attempt++) {
    const r = await api.post(`workspaces/${ws}/projects/QA/items`, { data: { title } });
    if (r.status() !== 429) return r.status();
    const wait = Number(r.headers()["retry-after"] ?? "1");
    await new Promise((res) => setTimeout(res, Math.min(Math.max(wait, 1), 10) * 1000));
  }
  return 429;
}

test.describe("board", () => {
  let seed: Seed;
  test.beforeAll(async () => {
    seed = await seedWorkspace();
    const alice = await apiAs("alice");
    expect(await createRespectingRateLimit(alice, seed.ws, "Sürüklə məni")).toBe(201);
    expect(await createRespectingRateLimit(alice, seed.ws, "Qaydaya tabe")).toBe(201);
    await alice.dispose();
  });

  test("has a column per status with true counts; drag changes the status", async ({ page }) => {
    await page.goto(seed.url + "?view=board");
    const todo = page.getByRole("region", { name: "To Do" });
    const doing = page.getByRole("region", { name: "In Progress" });
    for (const name of ["To Do", "In Progress", "Done", "Canceled"]) {
      await expect(page.getByRole("region", { name })).toBeVisible();
    }
    await expect(todo.getByRole("heading")).toContainText("2");
    await expect(doing.getByRole("heading")).toContainText("0");

    await dragTo(page, todo.getByText("Sürüklə məni"), doing);
    await expect(doing.getByText("Sürüklə məni")).toBeVisible();
    await expect(todo.getByText("Sürüklə məni")).toHaveCount(0);
    await expect(doing.getByRole("heading")).toContainText("1");
    await expect(todo.getByRole("heading")).toContainText("1");

    // The server agrees (not just the optimistic UI).
    const alice = await apiAs("alice");
    const item = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/items/1`)).json()) as { status: { name: string } };
    expect(item.status.name).toBe("In Progress");
    await alice.dispose();
  });

  test("columns the workflow forbids are dimmed and refuse the drop", async ({ page }) => {
    const alice = await apiAs("alice");
    const wf = await workflow(alice, seed.ws);
    const id = (name: string) => wf.statuses.find((s) => s.name === name)?.id;
    // Only To Do -> In Progress is allowed from now on.
    expect((await alice.post(`workspaces/${seed.ws}/workflows/${wf.id}/transitions`, {
      data: { name: "Başla", from_status_id: id("To Do"), to_status_id: id("In Progress") },
    })).ok()).toBe(true);
    expect((await alice.patch(`workspaces/${seed.ws}/workflows/${wf.id}`, { data: { enforce_transitions: true } })).ok()).toBe(true);

    await page.goto(seed.url + "?view=board");
    const todo = page.getByRole("region", { name: "To Do" });
    const done = page.getByRole("region", { name: "Done" });
    await dragTo(page, todo.getByText("Qaydaya tabe"), done, async () => {
      await expect(done).toHaveClass(/opacity-40/);
      await expect(page.getByRole("region", { name: "In Progress" })).not.toHaveClass(/opacity-40/);
    });
    await expect(page.getByText("Workflow bu status keçidinə icazə vermir.")).toBeVisible();
    await expect(todo.getByText("Qaydaya tabe")).toBeVisible();
    const item = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/items/2`)).json()) as { status: { name: string } };
    expect(item.status.name).toBe("To Do");
    await alice.patch(`workspaces/${seed.ws}/workflows/${wf.id}`, { data: { enforce_transitions: false } });
    await alice.dispose();
  });

  test("can group by category instead", async ({ page }) => {
    await page.goto(seed.url + "?view=board");
    await page.getByLabel("Qruplaşdır").selectOption("category");
    await expect(page).toHaveURL(/group=category/);
    // Independent of the other tests: "Qaydaya tabe" never leaves To Do.
    await expect(page.getByRole("region", { name: "Görüləcək" }).getByText("Qaydaya tabe")).toBeVisible();
    for (const name of ["İcrada", "Hazır", "Ləğv edilib"]) {
      await expect(page.getByRole("region", { name })).toBeVisible();
    }
  });
});

test("a large column shows its full count and loads more while scrolling", async ({ page }) => {
  test.setTimeout(120_000);
  const big = await seedWorkspace();
  // Bob (a member) creates them at ~10/s: well under his limit, and Alice's
  // budget stays free for the specs running in parallel.
  const bob = await apiAs("bob");
  for (let i = 1; i <= 60; i++) {
    expect(await createRespectingRateLimit(bob, big.ws, `Kart ${String(i)}`)).toBe(201);
    await new Promise((res) => setTimeout(res, 100));
  }
  await bob.dispose();

  await page.goto(big.url + "?view=board");
  const todo = page.getByRole("region", { name: "To Do" });
  await expect(todo.getByRole("heading")).toContainText("60");
  await expect(todo.getByText("Kart 60", { exact: true })).toBeVisible(); // newest first
  await expect(todo.getByText("Kart 1", { exact: true })).toHaveCount(0); // not loaded yet
  await todo.getByText(/^Kart \d+$/).last().scrollIntoViewIfNeeded();
  await expect(todo.getByText("Kart 1", { exact: true })).toBeAttached();
  await expect(todo.getByText(/^Kart \d+$/)).toHaveCount(60);
});
