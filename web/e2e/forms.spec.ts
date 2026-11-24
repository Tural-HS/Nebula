import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Intake forms (ADR-0032): alice (owner) builds a form, bob fills it in and
// follows his request on "My requests"; alice answers from the item. A second
// form for "anyone with an account" lives in a workspace bob is not in.
// Form addresses are unique per installation, so each run picks its own.
test.describe.configure({ mode: "serial" });

let seed: Seed;
const run = Math.random().toString(36).slice(2, 8);
const slug = `e2e-odenis-${run}`;
const openSlug = `e2e-giris-${run}`;
let outsiderWs = "";

test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const alice = await apiAs("alice");
  // A workspace without bob, with a private project and an "accounts" form.
  outsiderWs = `${seed.ws}-x`;
  expect((await alice.post("workspaces", { data: { slug: outsiderWs, name: "Xəzinə " + run } })).status()).toBe(201);
  expect((await alice.post(`workspaces/${outsiderWs}/projects`, { data: { key: "SEC", name: "Xəzinədarlıq", visibility: "private" } })).status()).toBe(201);
  const f = await alice.post(`workspaces/${outsiderWs}/projects/SEC/forms`, {
    data: {
      slug: openSlug, title: "Giriş tələbi", item_type: "task", title_template: "Giriş: {{requester}}", audience: "accounts",
      fields: [{ key: "description", label: "Nəyə giriş lazımdır?", required: true }],
    },
  });
  expect(f.status(), await f.text()).toBe(201);
  await alice.dispose();
});

test.describe("as the admin", () => {
  test.use({ storageState: authFile("alice") });

  test("builds a form in the project settings", async ({ page }) => {
    await page.goto(seed.url + "/settings");
    await page.getByRole("link", { name: "Formaları idarə et" }).click();
    await page.getByRole("button", { name: "Yeni forma" }).click();
    await page.getByLabel("Formanın adı").fill("Ödəniş tələbi");
    await expect(page.getByLabel("Ünvan")).toHaveValue("odenis-telebi"); // suggested from the name
    await page.getByLabel("Ünvan").fill(slug);
    await page.getByLabel("Giriş mətni").fill("Xarici təchizatçılara ödənişlər üçün.");

    await page.getByRole("combobox", { name: "Sahə seçin" }).selectOption({ label: "Təsvir" });
    await page.getByRole("button", { name: "Sahə əlavə et" }).click();
    await page.getByLabel("Etiket").first().fill("Təfərrüatlar");
    await page.getByLabel("Məcburi").first().check();
    await page.getByRole("combobox", { name: "Sahə seçin" }).selectOption({ label: "Son tarix" });
    await page.getByRole("button", { name: "Sahə əlavə et" }).click();
    // Keyboard reordering: the due date goes first.
    await page.getByRole("button", { name: "Son tarix: yuxarı" }).click();

    await page.getByLabel("Tapşırıq başlığının şablonu").fill("{{form}}: {{requester}}");
    // The preview shows the requester's view and the resulting title.
    const preview = page.getByRole("complementary", { name: "Önizləmə" });
    await expect(preview.getByLabel(/Təfərrüatlar/)).toBeVisible();
    await expect(preview).toContainText("Ödəniş tələbi: Alice Developer");

    await page.getByRole("button", { name: "Yadda saxla" }).click();
    const row = page.getByRole("listitem").filter({ hasText: "Ödəniş tələbi" });
    await expect(row).toContainText(`/f/${slug}`);
    await expect(row).toContainText("Aktiv");
  });
});

test.describe("as the requester", () => {
  test.use({ storageState: authFile("bob") });

  test("fills in the form, with an accessible error summary first", async ({ page }) => {
    await page.goto(`/f/${slug}`);
    await expect(page.getByRole("heading", { name: "Ödəniş tələbi" })).toBeVisible();
    await page.getByRole("button", { name: "Göndər" }).click();
    const summary = page.getByRole("alert");
    await expect(summary).toContainText("Formada 1 xəta var");
    await expect(summary).toBeFocused();
    await summary.getByRole("link", { name: /Təfərrüatlar/ }).click();
    await expect(page.getByLabel(/Təfərrüatlar/)).toBeFocused();

    await page.getByLabel(/Təfərrüatlar/).fill("Hesab-faktura INV-0001, 120 AZN");
    await page.getByRole("button", { name: "Göndər" }).click();
    await expect(page.getByRole("heading", { name: "Tələbiniz göndərildi" })).toBeVisible();
    await expect(page.getByTestId("submission-key")).toHaveText("QA-1");

    await page.getByRole("link", { name: "Tələblərim" }).last().click();
    await expect(page.getByRole("heading", { name: "Ödəniş tələbi: Bob Developer" })).toBeVisible();
    await expect(page.getByText("Hesab-faktura INV-0001, 120 AZN")).toBeVisible();
    await expect(page.getByText("Komanda hələ mesaj yazmayıb.")).toBeVisible();
  });

  test("works on a phone", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto(`/f/${slug}`);
    const field = page.getByLabel(/Təfərrüatlar/);
    await expect(field).toBeVisible();
    const box = await field.boundingBox();
    expect(box?.width ?? 0).toBeGreaterThan(300); // full width, no horizontal scroll
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });
});

test.describe("the team answers", () => {
  test.use({ storageState: authFile("alice") });

  test("writes a message to the requester from the item", async ({ page }) => {
    await page.goto(seed.url + "?item=1");
    const panel = page.getByRole("dialog", { name: "QA-1" });
    const req = panel.getByRole("region", { name: "Tələb" });
    await expect(req).toContainText("Bob Developer");
    await req.getByLabel("Tələb edənə mesaj").fill("Cümə günü ödəniləcək.");
    await req.getByRole("button", { name: "Mesajı saxla" }).click();
    await expect(page.getByText("Mesaj saxlanıldı")).toBeVisible();
  });
});

test.describe("the requester sees the update", () => {
  test.use({ storageState: authFile("bob") });

  test("my requests marks the news and shows the message", async ({ page }) => {
    await page.goto("/requests");
    const row = page.getByRole("link").filter({ hasText: "Ödəniş tələbi: Bob Developer" });
    await expect(row.getByText("Yeni")).toBeVisible();
    await row.click();
    await expect(page.getByText("Cümə günü ödəniləcək.")).toBeVisible();
    await page.getByRole("link", { name: /Bütün tələblər/ }).click();
    await expect(page.getByRole("link").filter({ hasText: "Ödəniş tələbi: Bob Developer" }).getByText("Yeni")).toHaveCount(0);
  });

  test("a form for any account works without workspace access", async ({ page }) => {
    await page.goto(`/f/${openSlug}`);
    await page.getByLabel(/Nəyə giriş lazımdır/).fill("Bank hesabatlarına baxış");
    await page.getByRole("button", { name: "Göndər" }).click();
    await expect(page.getByTestId("submission-key")).toHaveText("SEC-1");
    await page.goto("/requests");
    await expect(page.getByRole("link").filter({ hasText: "Giriş: Bob Developer" })).toBeVisible();
    // Still no access to the workspace or its private project.
    await page.goto(`/w/${outsiderWs}/p/SEC`);
    await expect(page.getByText("Tapılmadı").first()).toBeVisible();
  });
});
