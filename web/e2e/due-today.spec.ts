import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Items due today glow green in the table (ADR-0067); later ones do not.
test.use({ storageState: authFile("alice") });

const day = (n: number) => {
  const d = new Date();
  d.setDate(d.getDate() + n);
  return `${String(d.getFullYear())}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  for (const [title, n] of [["Bu gün ödəyir", 0], ["Sabah ödəyir", 1]] as const) {
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title, due_date: day(n) } })).status()).toBe(201);
  }
  await alice.dispose();
});

test("the title of an item due today glows", async ({ page }) => {
  await page.goto(seed.url);
  const row = page.getByRole("row", { name: /Bu gün ödəyir/ });
  const glow = row.locator("[data-due-today]");
  // The default columns have no due column; the title cell glows.
  await expect(glow).toHaveCount(1);
  await expect(glow.first()).toHaveCSS("animation-name", "due-today-pulse");
  await expect(page.getByRole("row", { name: /Sabah ödəyir/ }).locator("[data-due-today]")).toHaveCount(0);
});

test("reduced motion keeps the glow still", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(seed.url);
  const glow = page.getByRole("row", { name: /Bu gün ödəyir/ }).locator("[data-due-today]").first();
  await expect(glow).toHaveCSS("animation-name", "none");
});
