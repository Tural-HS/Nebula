import { authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

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
  await expect(page.getByLabel(/^Hadisələr/)).toHaveValue("item.*, comment.created");
  await page.getByRole("button", { name: "Yarat" }).click();

  await expect(page.getByText("https://chat.example.com/…9z", { exact: false })).toBeVisible();
  await expect(page.getByText("Mattermost / Slack", { exact: true })).toBeVisible();
  await expect(page.getByText(/secret yalnız indi göstərilir/)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Secret-i yenilə" })).toHaveCount(0);
  await expect(page.locator("body")).not.toContainText(token);

  await page.getByRole("button", { name: "Test göndər" }).click();
  await expect(page.getByText("Kanala test mesajı göndərildi")).toBeVisible();

  await page.reload();
  await expect(page.locator("body")).not.toContainText(token);
});
