import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Overdue bands (ADR-0057): yellow up to 10 days late, orange up to 20, red after.
test.use({ storageState: authFile("alice") });

const ago = (n: number) => {
  const d = new Date();
  d.setDate(d.getDate() - n);
  return `${String(d.getFullYear())}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  for (const [title, n] of [["Beş gün", 5], ["On beş gün", 15], ["Otuz gün", 30]] as const) {
    expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title, due_date: ago(n) } })).status()).toBe(201);
  }
  await alice.dispose();
});

test("cards and due chips take the band colour", async ({ page }) => {
  await page.goto(seed.url + "?view=board");
  const chip = (title: string) => page.locator("[data-late]", { hasText: new RegExp("^" + title) }).first();
  await expect(chip("Beş gün")).toHaveAttribute("data-late", "1");
  await expect(chip("On beş gün")).toHaveAttribute("data-late", "2");
  await expect(chip("Otuz gün")).toHaveAttribute("data-late", "3");
  await expect(page.getByLabel(/\(15 gün gecikir\)$/)).toBeVisible();
});

test("the calendar marks every late day", async ({ page }) => {
  await page.goto(seed.url + "?view=calendar");
  const marks = page.getByTestId("calendar-late").filter({ hasText: /^Beş gün/ });
  // Five late days, all in view unless the month turned in between.
  await expect.poll(() => marks.count()).toBeGreaterThan(0);
  await expect(marks.first()).toHaveAttribute("data-late", "1");
  await expect(page.getByTestId("calendar-late").filter({ hasText: /^Otuz gün/ }).locator("text=/^2[1-9]$|^30$/").first()).toBeVisible();
});
