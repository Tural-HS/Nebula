import { authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
});

test("create an item, edit it in the panel and see its history", async ({ page }) => {
  await page.goto(seed.url);
  await page.getByRole("button", { name: "Yeni tapşırıq" }).click();
  const dialog = page.getByRole("dialog", { name: "Yeni tapşırıq" });
  await dialog.getByLabel("Başlıq").fill("Ödəniş səhifəsi yavaşdır");
  await dialog.getByLabel("İcraçı").selectOption({ label: "Bob Developer" });
  await dialog.getByRole("button", { name: "Yarat" }).click();

  // Creating opens the new item's panel.
  const panel = page.getByRole("dialog", { name: "QA-1" });
  await expect(panel).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "QA-1 yaradıldı" })).toBeVisible(); // the toast, not a spinner

  await panel.getByLabel("Status").selectOption({ label: "In Progress" });
  await expect(panel.getByLabel("Status")).toHaveValue(/.+/);
  const title = panel.getByLabel("Başlıq");
  await title.fill("Ödəniş səhifəsi çox yavaşdır");
  await title.press("Enter");

  await panel.getByRole("tab", { name: "Aktivlik" }).click();
  await expect(panel.getByText("statusu dəyişdi")).toBeVisible();
  await expect(panel.getByText("başlığı dəyişdi")).toBeVisible();
  await expect(panel.getByText("tapşırığı yaratdı")).toBeVisible();

  await panel.getByRole("button", { name: "Bağla" }).click();
  const row = page.getByRole("row").filter({ hasText: "QA-1" });
  await expect(row).toContainText("Ödəniş səhifəsi çox yavaşdır");
  await expect(row).toContainText("In Progress");
});

test("search filters the table", async ({ page }) => {
  await page.goto(seed.url);
  await page.getByRole("button", { name: "Yeni tapşırıq" }).click();
  const dialog = page.getByRole("dialog", { name: "Yeni tapşırıq" });
  await dialog.getByLabel("Başlıq").fill("Unikal axtarış sözü zebra");
  await dialog.getByRole("button", { name: "Yarat" }).click();
  await page.getByRole("button", { name: "Bağla" }).click();

  await page.getByRole("searchbox", { name: "Axtar…" }).fill("zebra");
  await expect(page.getByRole("row").filter({ hasText: "zebra" })).toHaveCount(1);
  await expect(page.getByRole("row").filter({ hasText: "Ödəniş" })).toHaveCount(0);
  await expect(page).toHaveURL(/q=zebra/); // filters are shareable links
});
