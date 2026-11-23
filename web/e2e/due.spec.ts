import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  const create = async (title: string, due?: string) => {
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title, due_date: due } })).status()).toBe(201);
  };
  await create("Ödəniş gecikib", "2020-03-01"); // QA-1: overdue
  await create("Gələn il", "2199-01-15"); // QA-2
  await create("Tarixsiz"); // QA-3
  await alice.dispose();
});

test("overdue items are marked and can be filtered", async ({ page }) => {
  await page.goto(seed.url);
  const late = page.getByRole("row").filter({ hasText: "Ödəniş gecikib" });
  await expect(late.getByLabel("Son tarix: 1 mar 2020 (gecikir)")).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Gələn il" }).getByLabel("Son tarix: 15 yan 2199")).toBeVisible();

  await page.getByLabel("Son tarix", { exact: true }).selectOption("overdue");
  await expect(page).toHaveURL(/due=overdue/);
  await expect(page.getByRole("row").filter({ hasText: "QA-" })).toHaveCount(1);
  await expect(late).toBeVisible();

  // The board uses the same filter and shows the chip on the card.
  await page.goto(seed.url + "?view=board&due=overdue");
  const todo = page.getByRole("region", { name: "To Do" });
  await expect(todo.getByRole("heading")).toContainText("1");
  await expect(todo.getByLabel("Son tarix: 1 mar 2020 (gecikir)")).toBeVisible();
});

test("set and clear a due date in the panel; history records it", async ({ page }) => {
  await page.goto(seed.url + "?item=3");
  const panel = page.getByRole("dialog", { name: "QA-3" });
  const input = panel.getByLabel("Son tarix");
  await input.fill("2030-05-17");
  await input.blur();
  await expect(panel.getByRole("button", { name: "Təmizlə" })).toBeVisible();

  const alice = await apiAs("alice");
  const read = async () => ((await (await alice.get(`workspaces/${seed.ws}/projects/QA/items/3`)).json()) as { due_date: string | null }).due_date;
  await expect.poll(read).toBe("2030-05-17");

  await panel.getByRole("tab", { name: "Aktivlik" }).click();
  await expect(panel.getByText("son tarixi dəyişdi")).toBeVisible();
  await expect(panel.getByText("17 may 2030")).toBeVisible();

  await panel.getByRole("button", { name: "Təmizlə" }).click();
  await expect.poll(read).toBeNull();
  await alice.dispose();
});

test("admins set the time zone and the daily overdue digest", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/settings/general`);
  await page.getByLabel("Saat qurşağı").selectOption("Asia/Baku");
  await page.getByLabel("Gündəlik gecikmə xülasəsi").selectOption("9");
  await page.getByRole("button", { name: "Yadda saxla" }).click();
  await expect(page.getByText("Yadda saxlanıldı").first()).toBeVisible();

  const alice = await apiAs("alice");
  const ws = (await (await alice.get(`workspaces/${seed.ws}`)).json()) as { timezone: string; overdue_digest_hour: number | null };
  expect(ws).toMatchObject({ timezone: "Asia/Baku", overdue_digest_hour: 9 });
  await alice.dispose();
});
