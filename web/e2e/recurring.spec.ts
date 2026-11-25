import { apiAs, authFile, expect, seedWorkspace, test, type Seed, projectLink } from "./fixtures";

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("member");
});

const today = () => new Date().toISOString().slice(0, 10);

test("admin creates monthly and weekly rules; the page explains them in Azerbaijani", async ({ page }) => {
  await page.goto(seed.url);
  await projectLink(page, "Təkrarlanan");
  await expect(page.getByRole("heading", { name: "Təkrarlanan tapşırıqlar" })).toBeVisible();

  await page.getByRole("button", { name: "Yeni qayda" }).click();
  const dialog = page.getByRole("dialog", { name: "Yeni qayda" });
  await dialog.getByLabel("Başlıq şablonu").fill("Kirayə — {{month}}");
  await dialog.getByLabel("Tezlik").selectOption("monthly");
  await dialog.getByLabel("Ayın günü").selectOption("-1");
  await expect(dialog.getByText(/Hər ayın son günü · Növbəti tarixlər:/)).toBeVisible(); // server preview
  await dialog.getByRole("button", { name: "Yadda saxla" }).click();

  const rent = page.getByRole("listitem", { name: "Kirayə — {{month}}" });
  await expect(rent).toContainText("Hər ayın son günü");
  await expect(rent.getByText(/^\d{1,2} \S{3} \d{4}$/)).toHaveCount(5);

  await page.getByRole("button", { name: "Yeni qayda" }).click();
  await dialog.getByLabel("Başlıq şablonu").fill("Planlaşdırma görüşü");
  await dialog.getByLabel("Tezlik").selectOption("weekly");
  await dialog.getByLabel("cümə axşamı").check({ force: true }); // Monday is preselected
  await dialog.getByLabel("Hər neçə").fill("2");
  await dialog.getByRole("button", { name: "Yadda saxla" }).click();
  await expect(page.getByRole("listitem", { name: "Planlaşdırma görüşü" })).toContainText("Hər 2 həftədən bir: bazar ertəsi, cümə axşamı");
});

test("members can see the rules but not change them", async ({ browser }) => {
  const ctx = await browser.newContext({ storageState: authFile("bob") });
  const page = await ctx.newPage();
  await page.goto(seed.url + "/recurring");
  await expect(page.getByText("Qaydaları yalnız adminlər dəyişə bilər.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Yeni qayda" })).toHaveCount(0);
  await ctx.close();
});

test("the running scheduler creates the due item by itself", async ({ page }) => {
  test.setTimeout(150_000);
  const alice = await apiAs("alice");
  const r = await alice.post(`workspaces/${seed.ws}/projects/QA/recurrences`, {
    data: { title: "Kassa {{date}}", freq: "daily", every: 1, start_date: today(), lead_days: 0 },
  });
  expect(r.status()).toBe(201);
  // The worker ticks every minute.
  await expect
    .poll(async () => {
      const list = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/items?q=Kassa`)).json()) as { items: { title: string }[] };
      return list.items.length;
    }, { timeout: 100_000, intervals: [5_000] })
    .toBeGreaterThan(0);
  await alice.dispose();

  await page.goto(seed.url + "?q=Kassa");
  const row = page.getByRole("row").filter({ hasText: "Kassa " });
  await expect(row).toBeVisible();
  await row.click();
  const panel = page.getByRole("dialog", { name: /^QA-/ });
  await panel.getByRole("tab", { name: "Aktivlik" }).click();
  await expect(panel.getByText("Sistem")).toBeVisible();
});
