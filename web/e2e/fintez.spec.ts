import { readFileSync } from "node:fs";

import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// FinTez migration kit (ADR-0020): Azerbaijani workspace template, project
// templates and the Mattermost Boards import. Synthetic data only.

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("member");
});

test("a new workspace starts with Azerbaijani statuses by default", async ({ page }) => {
  const slug = `e2e-${process.env.E2E_RUN ?? "manual"}-az${Math.random().toString(36).slice(2, 6)}`; // cleaned by global-teardown
  await page.goto("/");
  await page.getByRole("button", { name: "Yeni workspace" }).click();
  await page.getByLabel("Ad", { exact: true }).fill("FinTez nümunə");
  await page.getByLabel("Qısa ad (URL)").fill(slug);
  await expect(page.getByLabel("Başlanğıc şablonu")).toHaveValue("az");
  await page.getByRole("button", { name: "Yarat" }).click();
  await expect(page).toHaveURL(new RegExp(`/w/${slug}`));

  const alice = await apiAs("alice");
  const types = (await (await alice.get(`workspaces/${slug}/item-types`)).json()) as { item_types: { name: string; statuses: { name: string }[] }[] };
  await alice.dispose();
  expect(types.item_types.map((x) => x.name).sort()).toEqual(["Tapşırıq", "Xəta"]);
  expect(types.item_types[0]?.statuses.map((s) => s.name)).toEqual(["Görüləcək", "İcrada", "Yoxlamada", "Tamamlandı", "Ləğv edildi"]);
});

test("a department project is created from another one as a template", async ({ page }) => {
  const alice = await apiAs("alice");
  const ws = `workspaces/${seed.ws}`;
  expect((await alice.post(`${ws}/projects`, { data: { key: "HR", name: "HR şöbəsi", visibility: "private" } })).status()).toBe(201);
  expect((await alice.post(`${ws}/projects/HR/recurrences`, {
    data: { title: "Aylıq əmək haqqı hesabatı", freq: "monthly", every: 1, month_day: 25, start_date: "2026-11-01" },
  })).status()).toBe(201);
  expect((await alice.post(`${ws}/projects/HR/views`, { data: { name: "Gecikənlər", kind: "table", shared: true, config: { filters: { due: "overdue" } } } })).status()).toBe(201);

  await page.goto(`/w/${seed.ws}`);
  await page.getByRole("button", { name: "Yeni layihə" }).click();
  await page.getByLabel("Ad", { exact: true }).fill("Maliyyə şöbəsi");
  await page.getByLabel("Açar").fill("FIN");
  await page.getByLabel("Şablon layihə").selectOption("HR");
  await expect(page.getByRole("checkbox", { name: /Gizli layihə/ })).toBeChecked(); // follows the private source
  await expect(page.getByRole("checkbox", { name: "Təkrarlanan tapşırıqlar" })).toBeChecked();
  await page.getByRole("button", { name: "Yarat" }).click();
  await expect(page).toHaveURL(new RegExp(`/w/${seed.ws}/p/FIN`));

  const recs = (await (await alice.get(`${ws}/projects/FIN/recurrences`)).json()) as { recurrences: { title: string }[] };
  const views = (await (await alice.get(`${ws}/projects/FIN/views`)).json()) as { views: { name: string }[] };
  await alice.dispose();
  expect(recs.recurrences.map((r) => r.title)).toEqual(["Aylıq əmək haqqı hesabatı"]);
  expect(views.views.map((v) => v.name)).toEqual(["Gecikənlər"]);
});

test("a Mattermost Boards archive is imported with people mapped to members", async ({ page }) => {
  await page.goto(seed.url);
  await page.getByRole("link", { name: "İdxal et" }).click();
  await page.getByLabel("Fayl seçin").setInputFiles({
    name: "archive-2026-11-24.boardarchive",
    mimeType: "application/zip",
    buffer: readFileSync("e2e/data/fintez-sample.boardarchive"),
  });
  await page.getByRole("button", { name: "Faylı oxu" }).click();
  await expect(page.getByLabel("Nebula sahəsi: A · Başlıq")).toHaveValue("title");
  await expect(page.getByLabel("Nebula sahəsi: B · Status")).toHaveValue("status");
  await expect(page.getByLabel("Nebula sahəsi: C · İcraçı")).toHaveValue("assignee");

  const people = page.getByRole("region", { name: "Mattermost istifadəçiləri" });
  await expect(people).toContainText("Aylıq ƏDV bəyannaməsi"); // sample card tells who mmBob is
  await people.getByRole("combobox").selectOption("bob@example.com");

  await page.getByRole("button", { name: "Yoxla" }).click();
  await expect(page.getByText(/Bütün 2 sətir düzgündür/)).toBeVisible();
  await page.getByRole("button", { name: "2 tapşırığı idxal et" }).click();
  await expect(page.getByText("2 tapşırıq yaradıldı")).toBeVisible();

  const alice = await apiAs("alice");
  const item = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/items/1`)).json()) as {
    title: string; assignee_id: string | null; due_date: string | null; description: string | null;
  };
  await alice.dispose();
  expect(item.title).toBe("Aylıq ƏDV bəyannaməsi");
  expect(item.assignee_id).toBe(seed.bobId);
  expect(item.due_date).toBe("2026-12-31");
  expect(item.description).toContain("Bank çıxarışı gözlənilir");
});
