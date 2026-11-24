import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Working-day calendar (ADR-0027): settings, recurrence options, calendar view.
// Synthetic data only. A year far ahead keeps the dates stable whatever today is.

test.use({ storageState: authFile("alice") });

const YEAR = 2031; // 1 Jan 2031 is a Wednesday
let seed: Seed;

test.beforeAll(async () => {
  seed = await seedWorkspace("member");
});

test("admin manages weekend, holidays and the Azerbaijan fixed-date preset", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/settings/general`);
  await page.getByRole("link", { name: "İş təqvimi" }).click();
  await expect(page.getByRole("heading", { name: "İş təqvimi", level: 1 })).toBeVisible();

  // Year navigation up to 2031.
  const days = page.getByRole("region", { name: "Bayramlar və köçürmələr" });
  const current = Number(await days.locator("[aria-live]").innerText());
  for (let y = current; y < YEAR; y++) await days.getByRole("button", { name: "Növbəti il" }).click();
  await expect(days.locator("[aria-live]")).toHaveText(String(YEAR));
  await expect(days.getByText(`${String(YEAR)} ili üçün hələ tarix əlavə edilməyib.`)).toBeVisible();

  // The preset says which dates must be confirmed every year.
  const preset = page.getByRole("region", { name: "Azərbaycanın sabit tarixli bayramları" });
  await expect(preset.getByRole("note")).toContainText("Novruz, Ramazan və Qurban");
  await preset.getByRole("button", { name: `${String(YEAR)} ilinə əlavə et` }).click();
  await expect(page.getByText("10 tarix əlavə edildi")).toBeVisible();
  await expect(days.getByRole("listitem", { name: /Yeni il bayramı/ })).toHaveCount(2);
  await expect(days.getByText("10 / 60 tarix")).toBeVisible();
  await preset.getByRole("button", { name: `${String(YEAR)} ilinə əlavə et` }).click();
  await expect(page.getByText("Yeni tarix yoxdur: hamısı artıq əlavə edilib.")).toBeVisible();

  // A lunar holiday is added by hand.
  await days.getByLabel("Tarix").fill(`${String(YEAR)}-03-20`);
  await days.getByLabel("Ad", { exact: true }).fill("Sınaq bayramı");
  await days.getByRole("button", { name: "Əlavə et" }).click();
  await expect(days.getByRole("listitem", { name: /Sınaq bayramı/ })).toBeVisible();

  // A working-day transfer must be on a weekend date: 3 Mar 2031 is a Monday.
  await days.getByLabel("Tarix").fill(`${String(YEAR)}-03-03`);
  await days.getByLabel("Növ").selectOption("workday");
  await days.getByRole("button", { name: "Əlavə et" }).click();
  await expect(days.getByRole("alert")).toHaveText("İş günü kimi yalnız həftə sonu günü elan etmək olar.");
  await days.getByLabel("Tarix").fill(`${String(YEAR)}-03-08`); // Saturday (also 8 March: replaces it)
  await days.getByRole("button", { name: "Əlavə et" }).click();
  await expect(days.getByRole("listitem", { name: /Köçürülmüş iş günü/ })).toBeVisible();

  // Removing a date.
  await days.getByRole("button", { name: "20 mar 2031 tarixini sil" }).click();
  await expect(days.getByRole("listitem", { name: /Sınaq bayramı/ })).toHaveCount(0);

  // Weekend: Friday + Saturday.
  const weekend = page.getByRole("region", { name: "Həftə sonu günləri" });
  await weekend.getByLabel("cümə", { exact: true }).check({ force: true });
  await weekend.getByLabel("bazar", { exact: true }).uncheck({ force: true });
  await weekend.getByRole("button", { name: "Yadda saxla" }).click();
  await expect(page.getByText("Yadda saxlanıldı")).toBeVisible();

  // Restore the default weekend for the other tests.
  const alice = await apiAs("alice");
  expect((await alice.put(`workspaces/${seed.ws}/calendar/weekend`, { data: { weekend_days: [6, 0] } })).status()).toBe(200);
  await alice.dispose();
});

test("members cannot change the calendar", async ({ browser }) => {
  const ctx = await browser.newContext({ storageState: authFile("bob") });
  const page = await ctx.newPage();
  await page.goto(`/w/${seed.ws}/settings/calendar`);
  await expect(page.getByRole("region", { name: "Həftə sonu günləri" })).toHaveCount(0);
  const bob = await apiAs("bob");
  expect((await bob.put(`workspaces/${seed.ws}/calendar/days/${String(YEAR)}-05-05`, { data: { kind: "holiday", name: "x" } })).status()).toBe(403);
  await bob.dispose();
  await ctx.close();
});

test("a 'last working day' rule skips a holiday and the calendar view shades it", async ({ page }) => {
  const alice = await apiAs("alice");
  // 31 Jan 2031 is a Friday; make it a holiday: the last working day is Thu 30 Jan.
  expect((await alice.put(`workspaces/${seed.ws}/calendar/days/${String(YEAR)}-01-31`, { data: { kind: "holiday", name: "Sınaq qeyri-iş günü" } })).status()).toBe(200);
  const r = await alice.post(`workspaces/${seed.ws}/projects/QA/recurrences`, {
    data: { title: "Aylıq hesabat", freq: "monthly", every: 1, work_day: -1, start_date: `${String(YEAR)}-01-01` },
  });
  expect(r.status()).toBe(201);
  expect(((await r.json()) as { next_due: string }).next_due).toBe(`${String(YEAR)}-01-30`);
  expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Bayramda son tarix", due_date: `${String(YEAR)}-01-31` } })).status()).toBe(201);
  await alice.dispose();

  await page.goto(`${seed.url}/recurring`);
  const rule = page.getByRole("listitem", { name: "Aylıq hesabat" });
  await expect(rule).toContainText("Hər ayın son iş günü");
  await expect(rule).toContainText("30 yan 2031");

  // The form shows the working-day option and the shift policy.
  await rule.getByRole("button", { name: "Redaktə et" }).click();
  const dialog = page.getByRole("dialog", { name: "Qaydanı redaktə et" });
  await expect(dialog.getByLabel("Gün", { exact: true })).toHaveValue("workday");
  await expect(dialog.getByLabel("Neçənci iş günü")).toHaveValue("-1");
  await dialog.getByLabel("Gün", { exact: true }).selectOption("date");
  await dialog.getByLabel("Ayın günü").selectOption("-1");
  await dialog.getByLabel("Qeyri-iş gününə düşərsə").selectOption("previous");
  await expect(dialog.getByText(/Hər ayın son günü \(qeyri-iş günüdürsə, əvvəlki iş günü\)/)).toBeVisible();
  await dialog.getByRole("button", { name: "Ləğv et" }).click();

  // Calendar view: the holiday cell carries its name; weekends come from the workspace calendar.
  await page.goto(`${seed.url}?view=calendar&cal=${String(YEAR)}-01`);
  const holiday = page.locator(`[data-day="${String(YEAR)}-01-31"]`);
  await expect(holiday).toHaveAttribute("data-nonworking", "holiday");
  await expect(holiday).toHaveAttribute("aria-label", /Qeyri-iş günü: Sınaq qeyri-iş günü/);
  await expect(holiday).toContainText("Bayramda son tarix");
  await expect(page.locator(`[data-day="${String(YEAR)}-01-04"]`)).toHaveAttribute("data-nonworking", "weekend"); // Saturday
  await expect(page.locator(`[data-day="${String(YEAR)}-01-06"]`)).not.toHaveAttribute("data-nonworking", /.+/); // Monday
});

test("settings stay usable on a phone (ADR-0019)", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto(`/w/${seed.ws}/settings/calendar`);
  await expect(page.getByRole("region", { name: "Bayramlar və köçürmələr" })).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => {
      const root = document.scrollingElement ?? document.documentElement;
      return Math.max(root.scrollWidth, document.body.scrollWidth) - window.innerWidth;
    }))
    .toBeLessThanOrEqual(0);
});
