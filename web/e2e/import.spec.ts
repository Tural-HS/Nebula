import { apiAs, authFile, expect, seedWorkspace, test, type Seed, projectLink } from "./fixtures";

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  const field = await alice.post(`workspaces/${seed.ws}/fields`, { data: { key: "amount", name: "Məbləğ", type: "number" } });
  expect(field.status()).toBe(201);
  expect((await alice.put(`workspaces/${seed.ws}/item-types/task/fields/amount`, { data: { required: false } })).status()).toBe(200);
  await alice.dispose();
});

// Synthetic rows only. The second file has one bad amount, so the dry run must block the import.
const good = "\uFEFFMüştəri;Məbləğ\nŞəhla Məmmədova;1 250,50 ₼\nÖmər Əliyev;300\n";
const bad = "Müştəri;Məbləğ\nŞəhla Məmmədova;on manat\n";

test("admin imports a CSV: preview, mapping, dry run, import", async ({ page }) => {
  await page.goto(seed.url);
  await projectLink(page, "İdxal et");
  await expect(page.getByRole("heading", { name: "Cədvəldən idxal" })).toBeVisible();

  // A file with a bad value: the dry run lists the row error and offers no import.
  await page.getByLabel("Fayl seçin").setInputFiles({ name: "pis.csv", mimeType: "text/csv", buffer: Buffer.from(bad) });
  await page.getByRole("button", { name: "Faylı oxu" }).click();
  await expect(page.getByLabel("Nebula sahəsi: A · Müştəri")).toHaveValue("title");
  await expect(page.getByLabel("Nebula sahəsi: B · Məbləğ")).toHaveValue("field.amount");
  await page.getByRole("button", { name: "Yoxla" }).click();
  const errors = page.getByRole("region", { name: "Sətir xətaları" });
  await expect(errors.getByRole("row").filter({ hasText: "must be a number" })).toBeVisible();
  await expect(page.getByRole("button", { name: /idxal et$/ })).toHaveCount(0);

  // Start over with a clean file.
  await page.getByRole("button", { name: "Uyğunlaşdırmaya qayıt" }).click();
  await page.getByRole("button", { name: "Geri" }).click();
  await page.getByLabel("Fayl seçin").setInputFiles({ name: "müştərilər.csv", mimeType: "text/csv", buffer: Buffer.from(good) });
  await page.getByRole("button", { name: "Faylı oxu" }).click();
  await expect(page.getByText("müştərilər.csv: 2 sətir")).toBeVisible();
  await page.getByRole("button", { name: "Yoxla" }).click();
  await expect(page.getByText(/Bütün 2 sətir düzgündür/)).toBeVisible();
  await page.getByRole("button", { name: "2 tapşırığı idxal et" }).click();
  await expect(page.getByText("2 tapşırıq yaradıldı")).toBeVisible();
  await expect(page.getByText("QA-1 – QA-2")).toBeVisible();

  await page.getByRole("link", { name: "Layihəyə keç" }).click();
  await expect(page.getByText("Şəhla Məmmədova")).toBeVisible();
  await expect(page.getByText("Ömər Əliyev")).toBeVisible();
});

test.describe("as a member", () => {
  test.use({ storageState: authFile("bob") });
  test("members see no import entry", async ({ page }) => {
    await page.goto(seed.url);
    await expect(page.getByRole("button", { name: "Yeni tapşırıq" })).toBeVisible();
    // The project menu exists for members (Təkrarlanan), without the import entry.
    await page.getByLabel("Digər əməliyyatlar").click();
    await expect(page.getByRole("link", { name: "Təkrarlanan" })).toBeVisible();
    await expect(page.getByRole("link", { name: "İdxal et" })).toHaveCount(0);
  });
});
