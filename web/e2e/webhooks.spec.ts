import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
});

// The incoming-webhook URL is the credential: after creation the UI only ever
// sees a masked form. Delivery itself is covered by Go integration tests.
test("admin adds a Mattermost webhook; its URL is never shown again", async ({ page }) => {
  const token = "abcdefghijklmnopqrstuvwx9z";
  await page.goto(`/w/${seed.ws}/settings/webhooks`);

  await page.getByLabel("URL").fill(`https://chat.example.com/hooks/${token}`);
  await expect(page.getByText(/Mattermost\/Slack webhook URL-inə oxşayır/)).toBeVisible();
  await page.getByRole("button", { name: "Mattermost / Slack formatına keç" }).click();
  await expect(page.getByLabel("Format")).toHaveValue("slack");
  await expect(page.getByLabel(/^Hadisələr/)).toHaveValue("item.*, comment.created, approval.*, workspace.overdue_digest");
  await page.getByRole("button", { name: "Yarat" }).click();

  const masked = page.getByText("https://chat.example.com/…wx9z", { exact: false }); // scheme, host, last 4
  await expect(masked).toBeVisible();
  // The created webhook's card (not the format picker) shows its format.
  const card = masked.locator("xpath=ancestor::*[.//button[normalize-space()='Test göndər']][1]");
  await expect(card.getByText("Mattermost / Slack", { exact: true })).toBeVisible();
  await expect(page.getByText(/secret yalnız indi göstərilir/)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Secret-i yenilə" })).toHaveCount(0);
  await expect(page.locator("body")).not.toContainText(token);

  await page.getByRole("button", { name: "Test göndər" }).click();
  await expect(page.getByText("Kanala test mesajı göndərildi")).toBeVisible();

  await page.reload();
  await expect(page.locator("body")).not.toContainText(token);
});

test("a webhook can be limited to projects; private ones only when ticked", async ({ page }) => {
  const alice = await apiAs("alice");
  const created = await alice.post(`workspaces/${seed.ws}/projects`, { data: { key: "SEC", name: "Hüquq", visibility: "private" } });
  expect(created.status()).toBe(201);
  const secID = ((await created.json()) as { id: string }).id;

  await page.goto(`/w/${seed.ws}/settings/webhooks`);
  const form = page.locator("form").filter({ hasText: "URL" });
  await form.getByLabel("URL").fill("https://hooks.example.com/hr");
  await form.getByText(/^Layihələr: Bütün açıq layihələr/).click();
  await expect(form.getByText(/gizli layihələrin hadisələri bu webhook-a göndərilmir/)).toBeVisible();
  await form.getByRole("checkbox", { name: /SEC Hüquq/ }).check();
  await expect(form.getByText(/^Layihələr: 1 layihə/)).toBeVisible();
  await form.getByRole("button", { name: "Yarat" }).click();

  await expect
    .poll(async () => {
      const r = (await (await alice.get(`workspaces/${seed.ws}/webhooks`)).json()) as { webhooks: { url: string; project_ids: string[] }[] };
      return r.webhooks.find((w) => w.url.includes("hooks.example.com/hr"))?.project_ids;
    })
    .toEqual([secID]);
  await alice.dispose();
});
