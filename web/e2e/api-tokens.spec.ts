import { request } from "@playwright/test";

import { API, apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Workspace service tokens (ADR-0043): an admin creates a token, an
// integration uses it, the admin sees "last used" and revokes it.
test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
});

test("admin creates a token, an integration uses it, the admin revokes it", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/settings/api-tokens`);
  await expect(page.getByText("Hələ token yoxdur.")).toBeVisible();

  await page.getByRole("button", { name: "Yeni token" }).click();
  const dialog = page.getByRole("dialog", { name: "Yeni token" });
  await dialog.getByRole("textbox", { name: "Ad" }).fill("1C inteqrasiyası");
  await dialog.getByLabel(/Tapşırıq yaratmaq və dəyişmək/).check();
  await dialog.getByRole("button", { name: "30 gün" }).click();
  await dialog.getByRole("button", { name: "Token yarat" }).click();

  const secretDialog = page.getByRole("dialog", { name: "Token yaradıldı" });
  await expect(secretDialog.getByRole("alert")).toContainText("bir daha göstərilməyəcək");
  const secret = (await secretDialog.getByTestId("token-secret").textContent())?.trim() ?? "";
  expect(secret).toMatch(/^nbl_[0-9a-f]{32}_[0-9a-f]{64}$/);
  await secretDialog.getByRole("button", { name: "Saxladım, bağla" }).click();
  await expect(page.getByTestId("token-secret")).toHaveCount(0);

  const card = page.getByRole("region", { name: "1C inteqrasiyası" });
  await expect(card.getByText("Aktiv")).toBeVisible();
  await expect(card.getByTestId("token-last-used")).toHaveText("heç vaxt");

  // The integration: a plain HTTP client with the bearer token.
  const integration = await request.newContext({ baseURL: API + "/", extraHTTPHeaders: { Authorization: `Bearer ${secret}` } });
  const created = await integration.post(`workspaces/${seed.ws}/projects/${seed.project}/items`, { data: { title: "Bank çıxarışı — noyabr" } });
  expect(created.status()).toBe(201);
  const { number } = (await created.json()) as { number: number };

  // History names the token as the actor.
  const alice = await apiAs("alice");
  const activity = await alice.get(`workspaces/${seed.ws}/projects/${seed.project}/items/${number}/activity`);
  expect(await activity.text()).toContain("🔑 1C inteqrasiyası");
  await alice.dispose();

  await page.reload();
  await expect(card.getByTestId("token-last-used")).not.toHaveText("heç vaxt");

  await card.getByRole("button", { name: "Ləğv et" }).click();
  const confirm = page.getByRole("dialog", { name: "Token ləğv edilsin?" });
  await expect(confirm).toContainText("«1C inteqrasiyası» dərhal işləməyi dayandıracaq");
  await confirm.getByRole("button", { name: "Bəli, ləğv et" }).click();
  await expect(card.getByText("Ləğv edilib")).toBeVisible();

  const after = await integration.get(`workspaces/${seed.ws}/projects/${seed.project}/items`);
  expect(after.status()).toBe(401);
  expect(((await after.json()) as { error: { code: string } }).error.code).toBe("token_revoked");
  await integration.dispose();
});

test("the tokens screen fits a 320 px phone", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await page.goto(`/w/${seed.ws}/settings/api-tokens`);
  await page.getByRole("button", { name: "Yeni token" }).click();
  await expect(page.getByRole("dialog", { name: "Yeni token" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320);
});
