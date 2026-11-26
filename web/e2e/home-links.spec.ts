import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Home page links (ADR-0053): an admin picks projects in the workspace
// settings; they appear on everyone's home page, private ones only for members.
test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  expect((await alice.post(`workspaces/${seed.ws}/projects`, { data: { key: "REY", name: "Lizinq Reyestri", visibility: "private" } })).status()).toBe(201);
  await alice.dispose();
});

test("an admin pins projects to the home page; members see the ones they can open", async ({ page, browser }) => {
  await page.goto(`/w/${seed.ws}/settings/general`);
  const card = page.getByRole("group", { name: "Ana səhifə keçidləri" });
  await card.getByRole("checkbox", { name: /^Lizinq Reyestri/ }).check();
  await card.getByRole("checkbox", { name: /^Keyfiyyət/ }).check();
  // Order: Keyfiyyət first.
  await card.getByRole("button", { name: "Keyfiyyət: Yuxarı" }).click();
  await card.getByRole("button", { name: "Saxla" }).click();
  await expect(page.getByText("Ana səhifə keçidləri saxlanıldı")).toBeVisible();

  await page.goto("/");
  const links = page.getByRole("navigation", { name: "Sürətli keçidlər" });
  const mine = links.getByRole("link").filter({ hasText: seed.ws });
  await expect(mine).toHaveCount(2);
  await expect(mine.nth(0)).toContainText("Keyfiyyət");
  await expect(mine.nth(1)).toContainText("Lizinq Reyestri");
  await mine.nth(1).click();
  await expect(page).toHaveURL(new RegExp(`/w/${seed.ws}/p/REY`));

  // bob is a member of the workspace but not of the private register.
  const ctx = await browser.newContext({ storageState: authFile("bob") });
  const bob = await ctx.newPage();
  await bob.goto("/");
  const bobLinks = bob.getByRole("navigation", { name: "Sürətli keçidlər" }).getByRole("link").filter({ hasText: seed.ws });
  await expect(bobLinks).toHaveCount(1);
  await expect(bobLinks).toContainText("Keyfiyyət");
  await ctx.close();
});
