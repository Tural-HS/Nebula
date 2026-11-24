import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Archive, restore and permanent deletion of projects and workspaces
// (ADR-0039). alice owns every seeded workspace, bob is a member.

test.describe("owner", () => {
  test.use({ storageState: authFile("alice") });

  test("archives a project, restores it, then deletes it with the typed key", async ({ page }) => {
    const seed = await seedWorkspace();
    const alice = await apiAs("alice");
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Hesab-faktura 1001" } })).status()).toBe(201);

    await page.goto(`${seed.url}/settings`);
    const danger = page.getByRole("region", { name: "Təhlükəli zona" });
    await danger.getByRole("button", { name: "Arxivlə" }).click();
    const dialog = page.getByRole("dialog", { name: /Keyfiyyət.*arxivlənsin/ });
    await dialog.getByRole("button", { name: "Arxivlə" }).click();

    // The deep link says "archived" and offers Restore.
    await expect(page).toHaveURL(new RegExp(`${seed.url}$`));
    await expect(page.getByRole("heading", { name: "Bu layihə arxivlənib" })).toBeVisible();

    // Hidden from the project list, listed under "Arxivlənmiş layihələr".
    await page.goto(`/w/${seed.ws}`);
    await expect(page.getByText("Hələ layihə yoxdur.")).toBeVisible();
    const archived = page.getByRole("region", { name: "Arxivlənmiş layihələr" });
    await archived.getByRole("link", { name: /QA/ }).click();
    await page.getByRole("button", { name: "Bərpa et" }).click();
    await expect(page.getByText("Hesab-faktura 1001").first()).toBeVisible();

    // Delete: archived first, then the key must be typed.
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/archive`)).status()).toBe(200);
    await page.reload();
    await page.getByRole("button", { name: "Layihəni birdəfəlik sil" }).click();
    const del = page.getByRole("dialog", { name: /birdəfəlik silinsin/ });
    const confirm = del.getByRole("button", { name: "Birdəfəlik sil" });
    await expect(confirm).toBeDisabled();
    await del.getByLabel(/layihənin açarını yazın: QA/).fill("qa");
    await expect(confirm).toBeDisabled();
    await del.getByLabel(/layihənin açarını yazın: QA/).fill("QA");
    await confirm.click();
    await expect(page).toHaveURL(new RegExp(`/w/${seed.ws}$`));
    await expect(page.getByText("Layihə silindi.")).toBeVisible();
    expect((await alice.get(`workspaces/${seed.ws}/projects/QA`)).status()).toBe(404);
    await alice.dispose();
  });

  test("archives a workspace (read-only, listed as archived) and deletes it", async ({ page }) => {
    const seed = await seedWorkspace();
    const alice = await apiAs("alice");

    await page.goto(`/w/${seed.ws}/settings/general`);
    await page.getByRole("region", { name: "Təhlükəli zona" }).getByRole("button", { name: "Arxivlə" }).click();
    await page.getByRole("dialog", { name: /arxivlənsin/ }).getByRole("button", { name: "Arxivlə" }).click();
    await expect(page.getByRole("status").filter({ hasText: "Bu workspace arxivlənib" })).toBeVisible();

    // Writes are refused while archived.
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "x" } })).status()).toBe(409);

    // Home: not among the workspace cards, but in the archived section with Restore.
    await page.goto("/");
    const section = page.getByTestId("archived-workspaces");
    await expect(section.getByText(`E2E ${seed.ws}`, { exact: true })).toBeVisible();
    await expect(page.getByTestId("workspace-cards").getByText(`E2E ${seed.ws}`, { exact: true })).toHaveCount(0);
    await section.getByRole("button", { name: `Bərpa et: E2E ${seed.ws}` }).click();
    await expect(page.getByTestId("workspace-cards").getByText(`E2E ${seed.ws}`, { exact: true })).toBeVisible();

    // Delete: archive again; the slug and the evidence acknowledgement are required.
    expect((await alice.post(`workspaces/${seed.ws}/archive`)).status()).toBe(200);
    await page.goto(`/w/${seed.ws}/settings/general`);
    await page.getByRole("region", { name: "Təhlükəli zona" }).getByRole("button", { name: "Birdəfəlik sil" }).click();
    const del = page.getByRole("dialog", { name: /birdəfəlik silinsin/ });
    await expect(del.getByRole("link", { name: /Sübut paketini endir/ })).toBeVisible();
    const confirm = del.getByRole("button", { name: "Birdəfəlik sil" });
    await del.getByLabel(/qısa adını yazın/).fill(seed.ws);
    await expect(confirm).toBeDisabled();
    await del.getByLabel(/Sübut paketini endirmişəm/).check();
    await confirm.click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByText(/Workspace (silindi|silinir)/)).toBeVisible();
    await expect.poll(async () => (await alice.get(`workspaces/${seed.ws}`)).status(), { timeout: 70_000 }).toBe(404);
    await alice.dispose();
  });
});

test.describe("member", () => {
  test.use({ storageState: authFile("bob") });

  let seed: Seed;
  test.beforeAll(async () => {
    seed = await seedWorkspace();
    const alice = await apiAs("alice");
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/archive`)).status()).toBe(200);
    await alice.dispose();
  });

  test("sees an archived project's state without Restore", async ({ page }) => {
    await page.goto(seed.url);
    await expect(page.getByRole("heading", { name: "Bu layihə arxivlənib" })).toBeVisible();
    await expect(page.getByText("workspace admininə müraciət edin")).toBeVisible();
    await expect(page.getByRole("button", { name: "Bərpa et" })).toHaveCount(0);
  });
});
