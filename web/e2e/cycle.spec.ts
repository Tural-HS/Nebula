import { apiAs, authFile, expect, projectLink, seedWorkspace, test, type Seed } from "./fixtures";

// Monthly period close (ADR-0051): set it up, preview and close a month by
// hand, read the saved month; members see it read-only.

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const alice = await apiAs("alice");
  const ws = `workspaces/${seed.ws}`;
  const field = async (data: Record<string, unknown>, position: number) => {
    expect((await alice.post(`${ws}/fields`, { data })).status()).toBe(201);
    expect((await alice.put(`${ws}/item-types/task/fields/${String(data.key)}`, { data: { position } })).status()).toBe(200);
  };
  await field({ key: "monthly", name: "Aylıq ödəniş", type: "number", options: { unit: "₼" } }, 1);
  await field({ key: "balance", name: "Qalıq", type: "number", options: { unit: "₼" } }, 2);
  await field({ key: "paid", name: "Ödədi", type: "boolean" }, 3);
  for (const [title, monthly, balance, paid] of [["Rəşad Quliyev", 150, 0, true], ["Kullinan MMC", 200, 50, false]] as const) {
    expect((await alice.post(`${ws}/projects/QA/items`, { data: { title, fields: { monthly, balance, paid } } })).status()).toBe(201);
  }
  await alice.dispose();
});

const monthName = (offset: number) => {
  const names = ["Yanvar", "Fevral", "Mart", "Aprel", "May", "İyun", "İyul", "Avqust", "Sentyabr", "Oktyabr", "Noyabr", "Dekabr"];
  const d = new Date();
  d.setDate(1);
  d.setMonth(d.getMonth() + offset);
  return `${names[d.getMonth()] ?? ""} ${String(d.getFullYear())}`;
};

test("admin sets up the cycle, closes last month and reads it back", async ({ page }) => {
  await page.goto(seed.url);
  await projectLink(page, "Aylıq dövr");
  await expect(page.getByRole("heading", { name: "Aylıq dövr", level: 1 })).toBeVisible();
  await expect(page.getByText("Avtomatik bağlanış söndürülüb.")).toBeVisible();

  const form = page.getByRole("form", { name: "Ayarlar" });
  await form.getByLabel("Hər ay avtomatik bağla").check();
  await form.getByLabel("Ödədi").check();
  await form.getByRole("button", { name: "+ Sahə əlavə et" }).click();
  await form.getByLabel("Hədəf").selectOption({ label: "Qalıq" });
  await form.getByLabel("Mənbə").selectOption({ label: "Aylıq ödəniş" });
  await form.getByRole("button", { name: "Yadda saxla" }).click();
  await expect(page.getByText("Aylıq dövr yadda saxlanıldı")).toBeVisible();
  await expect(page.getByText(new RegExp(`Növbəti bağlanış: 1 \\S{3} \\d{4} — ${monthName(0)} bağlanacaq\\.`))).toBeVisible();

  const close = page.getByRole("region", { name: "Ayı əl ilə bağlamaq" });
  await expect(close.getByText("2 tapşırıq tarixçəyə yazılacaq; 2 açıq tapşırıqdan 2-nin dəyərləri dəyişəcək.")).toBeVisible();
  await close.getByRole("button", { name: "İndi bağla" }).click();
  const dialog = page.getByRole("dialog", { name: `${monthName(-1)} bağlansın?` });
  await dialog.getByRole("button", { name: "Ayı bağla" }).click();
  await expect(page.getByText(`${monthName(-1)} bağlandı: 2 tapşırıq yeniləndi`)).toBeVisible();
  await expect(close.getByText("Bu ay artıq bağlanıb.")).toBeVisible();
  await expect(close.getByRole("button", { name: "İndi bağla" })).toBeDisabled();

  // The saved month keeps the old values, with totals.
  await page.getByRole("button", { name: `Bax: ${monthName(-1)}` }).click();
  await expect(page.getByRole("heading", { name: `${monthName(-1)}: ay bağlananda` })).toBeVisible();
  const row = page.getByRole("row", { name: /Rəşad Quliyev/ });
  await expect(row).toContainText("✓");
  await expect(row).toContainText(/0,00\s₼/);
  await expect(page.getByRole("row", { name: /Cəmi/ })).toContainText(/350,00\s₼/);
  await page.getByRole("link", { name: "← Bütün aylar" }).click();
  await expect(page.getByRole("heading", { name: "Bağlanmış aylar" })).toBeVisible();

  // The items themselves were reset: balance = monthly payment, paid cleared.
  const alice = await apiAs("alice");
  const items = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/items`)).json()) as { items: { title: string; fields: Record<string, unknown> }[] };
  const r = items.items.find((i) => i.title === "Rəşad Quliyev");
  expect(r?.fields.balance).toBe(150);
  expect(r?.fields.paid ?? null).toBeNull();
  await alice.dispose();
});

test("members read the cycle and history but cannot change it", async ({ browser }) => {
  const ctx = await browser.newContext({ storageState: authFile("bob") });
  const page = await ctx.newPage();
  await page.goto(seed.url + "/cycle");
  await expect(page.getByRole("heading", { name: "Bağlanmış aylar" })).toBeVisible();
  await expect(page.getByRole("form", { name: "Ayarlar" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "İndi bağla" })).toHaveCount(0);
  await ctx.close();
});
