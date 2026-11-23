import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
});

test("admin configures a required field and an automation; both take effect", async ({ page }) => {
  // 1. A select field with three choices.
  await page.goto(`/w/${seed.ws}/settings/fields`);
  await page.getByLabel(/^Ad/).first().fill("Prioritet");
  await expect(page.getByLabel(/^Açar/)).toHaveValue("prioritet");
  await page.getByLabel("Tip").selectOption("select");
  await page.getByLabel(/Seçimlər/).fill("Aşağı\nOrta\nYüksək");
  await page.getByRole("button", { name: "Yarat" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: "Yüksək" })).toBeVisible();

  // 2. Required on bugs.
  await page.getByRole("link", { name: "Tapşırıq tipləri" }).click();
  const bug = page.getByRole("region", { name: "Bug" });
  await bug.getByRole("checkbox").check();
  await bug.getByRole("button", { name: "Bağla" }).click();
  await expect(bug.getByText("Prioritet", { exact: true })).toBeVisible();

  // 3. Rule: high-priority bugs go to Bob.
  await page.getByRole("link", { name: "Avtomatlaşdırma" }).click();
  await page.getByRole("button", { name: "Yeni qayda" }).click();
  await page.getByLabel(/^Ad/).fill("Yüksək bug-lar Bob-a");
  await page.getByRole("button", { name: "Şərt əlavə et" }).click();
  await page.getByLabel("field", { exact: true }).selectOption("fields.prioritet");
  await page.getByLabel("Dəyər").selectOption({ label: "Yüksək" });
  await page.getByLabel("İcraçı", { exact: true }).selectOption({ label: "Bob Developer" });
  await page.getByRole("button", { name: "Yadda saxla" }).click();
  await expect(page.getByText("Yüksək bug-lar Bob-a")).toBeVisible();

  // 4. The UI enforces the required field when creating a bug.
  await page.goto(seed.url);
  await page.getByRole("button", { name: "Yeni tapşırıq" }).click();
  const dialog = page.getByRole("dialog", { name: "Yeni tapşırıq" });
  await dialog.getByLabel("Başlıq").fill("Prod çökdü");
  await dialog.getByLabel("Tip").selectOption({ label: "Bug" });
  await dialog.getByRole("button", { name: "Yarat" }).click();
  await expect(dialog.getByRole("alert")).toContainText("is required");
  await dialog.getByLabel("Prioritet *").selectOption({ label: "Yüksək" });
  await dialog.getByRole("button", { name: "Yarat" }).click();
  await expect(page.getByRole("dialog", { name: "QA-1" })).toBeVisible();

  // 5. The automation assigned it to Bob (asynchronously, via the worker).
  const alice = await apiAs("alice");
  await expect
    .poll(async () => ((await (await alice.get(`workspaces/${seed.ws}/projects/QA/items/1`)).json()) as { assignee_id: string | null }).assignee_id, {
      timeout: 15_000,
    })
    .toBe(seed.bobId);
  await alice.dispose();
});

test("member roles can be changed, the last owner is protected", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/settings/members`);
  await page.getByLabel("Bob Developer role").selectOption("admin");
  await expect(page.getByText("Yadda saxlanıldı").first()).toBeVisible();
  await page.getByLabel("Alice Developer role").selectOption("member");
  await expect(page.getByRole("alert").filter({ hasText: "at least one owner" })).toBeVisible();
});
