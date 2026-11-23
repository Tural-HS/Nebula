import { authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("member"); // alice owner, bob member
});

test("a private project is invisible to non-members until an admin adds them", async ({ browser }) => {
  const alice = await (await browser.newContext({ storageState: authFile("alice") })).newPage();
  const bob = await (await browser.newContext({ storageState: authFile("bob") })).newPage();

  // Alice creates a private project.
  await alice.goto(`/w/${seed.ws}`);
  await alice.getByRole("button", { name: "Yeni layihə" }).click();
  const dialog = alice.getByRole("dialog", { name: "Yeni layihə" });
  await dialog.getByLabel("Ad", { exact: true }).fill("Hüquq şöbəsi");
  await dialog.getByLabel(/Açar|Layihə açarı/).fill("HUQ");
  await dialog.getByLabel(/Gizli layihə/).check();
  await dialog.getByRole("button", { name: "Yarat" }).click();
  await expect(alice).toHaveURL(new RegExp(`/w/${seed.ws}/p/HUQ$`));

  // Bob can neither list nor open it.
  await bob.goto(`/w/${seed.ws}`);
  await expect(bob.getByText("QA", { exact: true })).toBeVisible();
  await expect(bob.getByText("HUQ", { exact: true })).toHaveCount(0);
  await bob.goto(`/w/${seed.ws}/p/HUQ`);
  await expect(bob.getByText("Tapılmadı").first()).toBeVisible();

  // Alice cannot assign Bob yet: he is not offered.
  await alice.getByRole("button", { name: "Yeni tapşırıq" }).click();
  const create = alice.getByRole("dialog", { name: "Yeni tapşırıq" });
  await expect(create.getByLabel("İcraçı").locator("option", { hasText: "Bob Developer" })).toHaveCount(0);
  await create.getByRole("button", { name: "Ləğv et" }).click();

  // Alice adds Bob in the project settings.
  await alice.getByRole("link", { name: "Layihə ayarları" }).click();
  await expect(alice.getByRole("note")).toContainText("webhook");
  const members = alice.getByRole("region", { name: "Layihə üzvləri" });
  await members.getByLabel("Üzv əlavə et").selectOption({ label: "Bob Developer (Üzv)" });
  await members.getByRole("button", { name: "Üzv əlavə et" }).click();
  await expect(members.getByText("Bob Developer")).toBeVisible();

  // Now Bob sees it, with the lock badge, and can be assigned.
  await bob.goto(`/w/${seed.ws}`);
  await expect(bob.getByRole("link", { name: /HUQ/ })).toContainText("Gizli");
  await bob.getByRole("link", { name: /HUQ/ }).click();
  await expect(bob.getByRole("button", { name: "Yeni tapşırıq" })).toBeVisible();
  await alice.goto(`/w/${seed.ws}/p/HUQ`);
  await alice.getByRole("button", { name: "Yeni tapşırıq" }).click();
  await expect(alice.getByRole("dialog", { name: "Yeni tapşırıq" }).getByLabel("İcraçı").locator("option", { hasText: "Bob Developer" })).toHaveCount(1);

  await alice.context().close();
  await bob.context().close();
});
