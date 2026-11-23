import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Sürüklə məni" } });
  await alice.dispose();
});

test("drag a card to another column changes its status", async ({ page }) => {
  await page.goto(seed.url + "?view=board");
  const todo = page.getByRole("region", { name: "Görüləcək" });
  const doing = page.getByRole("region", { name: "İcrada" });
  const card = todo.getByText("Sürüklə məni");
  await expect(card).toBeVisible();

  // dnd-kit needs real pointer movement past its activation distance.
  const from = await card.boundingBox();
  const to = await doing.boundingBox();
  if (!from || !to) throw new Error("layout not ready");
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(from.x + from.width / 2 + 20, from.y + 20, { steps: 5 });
  await page.mouse.move(to.x + to.width / 2, to.y + 80, { steps: 15 });
  await page.mouse.up();

  await expect(doing.getByText("Sürüklə məni")).toBeVisible();
  await expect(todo.getByText("Sürüklə məni")).toHaveCount(0);

  // The server agrees (not just the optimistic UI).
  const alice = await apiAs("alice");
  const item = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/items/1`)).json()) as { status: { category: string } };
  expect(item.status.category).toBe("in_progress");
  await alice.dispose();
});
