import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Birthday reminder (ADR-0054): an admin picks a date field in the project
// settings. Sending is covered by the Go tests (the scheduler runs in the worker).
test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  expect((await alice.post(`workspaces/${seed.ws}/fields`, { data: { key: "birthday", name: "Ad günü", type: "date" } })).status()).toBe(201);
  await alice.dispose();
});

test("an admin turns the birthday reminder on and off", async ({ page }) => {
  await page.goto(`${seed.url}/settings`);
  const section = page.getByRole("region", { name: /Ad günü xatırlatması/ });
  const select = section.getByLabel("Tarix sahəsi");
  await expect(select).toHaveValue("");
  await select.selectOption("birthday");
  await section.getByRole("button", { name: "Yadda saxla" }).click();
  await expect(page.getByText("Ad günü xatırlatması saxlanıldı")).toBeVisible();
  await page.reload();
  await expect(page.getByRole("region", { name: /Ad günü xatırlatması/ }).getByLabel("Tarix sahəsi")).toHaveValue("birthday");

  await page.getByRole("region", { name: /Ad günü xatırlatması/ }).getByLabel("Tarix sahəsi").selectOption("");
  await page.getByRole("region", { name: /Ad günü xatırlatması/ }).getByRole("button", { name: "Yadda saxla" }).click();
  await expect(page.getByText("Ad günü xatırlatması saxlanıldı").last()).toBeVisible();
  await expect(page.getByRole("region", { name: /Ad günü xatırlatması/ }).getByRole("button", { name: "Yadda saxla" })).toBeDisabled();
  await page.reload();
  await expect(page.getByRole("region", { name: /Ad günü xatırlatması/ }).getByLabel("Tarix sahəsi")).toHaveValue("");
});
