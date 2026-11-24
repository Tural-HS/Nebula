import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Item list and panel layout (ADR-0035): quick filters with counts, filter
// chips, the "Siqnal" column and the two-column panel beside the table.
// Desktop project (1280 px).

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  const create = async (data: Record<string, unknown>) => {
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data })).status()).toBe(201);
  };
  await create({ title: "Gecikmiş ödəniş", due_date: "2020-03-01" }); // QA-1
  await create({ title: "Mənim hesabatım", assignee_id: seed.aliceId }); // QA-2
  await create({ title: "Adi tapşırıq" }); // QA-3
  await alice.dispose();
});

test("quick filters show counts, filter the list and become removable chips", async ({ page }) => {
  await page.goto(seed.url);
  const queues = page.getByRole("group", { name: "Tez filtrlər" });
  await expect(queues.getByRole("button", { name: "Hamısı: 3" })).toHaveAttribute("aria-pressed", "true");
  await expect(queues.getByRole("button", { name: "Mənə təyin: 1" })).toBeVisible();

  await queues.getByRole("button", { name: "Gecikən: 1" }).click();
  await expect(page).toHaveURL(/due=overdue/);
  await expect(page.getByRole("row").filter({ hasText: "QA-" })).toHaveCount(1);
  await expect(queues.getByRole("button", { name: "Gecikən: 1" })).toHaveAttribute("aria-pressed", "true");

  // The applied filter is a chip; removing it shows everything again.
  const chips = page.getByRole("list", { name: "Aktiv filtrlər" });
  await chips.getByRole("button", { name: "Filtri götür: Son tarix: Gecikənlər" }).click();
  await expect(page).not.toHaveURL(/due=/);
  await expect(page.getByRole("row").filter({ hasText: "QA-" })).toHaveCount(3);

  // Signals moved out of the title into their own column.
  await expect(page.getByRole("columnheader", { name: "Siqnal" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Gecikmiş ödəniş" }).getByLabel("Son tarix: 1 mar 2020 (gecikir)")).toBeVisible();
});

test("the panel sits beside the table, with the workflow and the properties", async ({ page }) => {
  await page.goto(seed.url + "?item=1");
  const panel = page.getByRole("dialog", { name: "QA-1" });
  await expect(panel.getByRole("list", { name: "İş axını" }).locator("[aria-current=step]")).toHaveCount(1);
  const details = panel.getByRole("complementary", { name: "Xüsusiyyətlər" });
  await expect(details.getByRole("combobox", { name: "Status" })).toBeVisible();
  // Two columns at 1280 px: the properties are to the right of the title.
  const title = await panel.getByLabel("Başlıq").boundingBox();
  const props = await details.boundingBox();
  expect((props?.x ?? 0) > (title?.x ?? 0)).toBe(true);

  // The table stays usable: another row switches the panel.
  await page.getByRole("row").filter({ hasText: "Adi tapşırıq" }).getByRole("button", { name: "Adi tapşırıq" }).click();
  await expect(page.getByRole("dialog", { name: "QA-3" })).toBeVisible();
  await page.getByRole("dialog", { name: "QA-3" }).getByRole("button", { name: "Bağla" }).click();
  await expect(page).not.toHaveURL(/item=/);
});
