import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("bob") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("guest");
  const alice = await apiAs("alice");
  await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Yalnız oxumaq üçün" } });
  await alice.dispose();
});

test("a guest can read but not change anything", async ({ page }) => {
  await page.goto(seed.url);
  await expect(page.getByRole("row").filter({ hasText: "Yalnız oxumaq üçün" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Yeni tapşırıq" })).toHaveCount(0);
  await expect(page.getByText("yalnız oxuma hüququnuz var")).toBeVisible();

  await page.goto(seed.url + "?item=1");
  const panel = page.getByRole("dialog", { name: "QA-1" });
  await expect(panel.getByLabel("Status")).toHaveCount(0); // a badge, not a selector
  await expect(panel.getByRole("button", { name: "Sil" })).toHaveCount(0);
  await expect(panel.getByRole("textbox", { name: /Şərh yazın/ })).toHaveCount(0);
});

test("a guest cannot open admin settings", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/settings/fields`);
  await expect(page.getByText("Bu bölmə yalnız adminlər üçündür.")).toBeVisible();
  // The member list is visible to everyone, but without controls.
  await page.goto(`/w/${seed.ws}/settings/members`);
  await expect(page.getByText("Alice Developer")).toBeVisible();
  await expect(page.getByRole("button", { name: "Üzv əlavə et" })).toHaveCount(0);
});

test("the API rejects a guest even if the UI were bypassed", async () => {
  const bob = await apiAs("bob");
  const res = await bob.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "hack" } });
  expect(res.status()).toBe(403);
  await bob.dispose();
});

test("non-members cannot see that a workspace exists", async ({ page }) => {
  const other = await seedWorkspace("member");
  const alice = await apiAs("alice");
  await alice.delete(`workspaces/${other.ws}/members/${other.bobId}`);
  await alice.dispose();
  await page.goto(`/w/${other.ws}`);
  await expect(page.getByRole("alert")).toContainText("Tapılmadı");
});
