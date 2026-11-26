import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Ledger records (imported customer cards): number units, the key facts card,
// tap-to-call phone numbers, folded empty sections and the pinned title.

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("guest");
  const alice = await apiAs("alice");
  const ws = `workspaces/${seed.ws}`;
  const field = async (data: Record<string, unknown>, position: number) => {
    expect((await alice.post(`${ws}/fields`, { data })).status()).toBe(201);
    expect((await alice.put(`${ws}/item-types/task/fields/${String(data.key)}`, { data: { position } })).status()).toBe(200);
  };
  await field({ key: "balance", name: "Qalıq", type: "number", options: { unit: "₼" } }, 1);
  await field({ key: "phone", name: "Əlaqə nömrəsi", type: "text" }, 2);
  await field({ key: "nv", name: "NV nömrəsi", type: "text" }, 3);
  await field({ key: "paid", name: "Ödədi", type: "boolean" }, 4);
  for (const [title, balance] of [["Rəşad Quliyev Hikmət oğlu", 1200], ["Kullinan MMC", 800]] as const) {
    const data = { title, fields: { balance, phone: "515614145", nv: "10YX930" } };
    expect((await alice.post(`${ws}/projects/QA/items`, { data })).status()).toBe(201);
  }
  await alice.dispose();
});

test.describe("as admin", () => {
  test.use({ storageState: authFile("alice") });

  test("units in cells and totals, key facts, folded sections", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,status,field.balance,field.phone,field.nv`);
    await expect(page.getByRole("row").filter({ hasText: "Kullinan MMC" })).toContainText("800,00 ₼");
    await expect(page.getByRole("row", { name: "Cəmi" })).toContainText("2 000,00 ₼");
    // No row has a signal: no empty column.
    await expect(page.getByRole("columnheader", { name: "Siqnal" })).toHaveCount(0);

    await page.getByRole("button", { name: "Kullinan MMC" }).click();
    const panel = page.getByRole("dialog", { name: "QA-2" });
    const facts = panel.getByRole("region", { name: "Əsas məlumat" });
    await expect(facts).toContainText("800,00 ₼");
    await expect(facts.getByRole("link", { name: "+994 51 561 41 45" })).toHaveAttribute("href", "tel:+994515614145");
    // An account-like number is not a phone link.
    await expect(facts.getByRole("link", { name: "10YX930" })).toHaveCount(0);

    // Empty sections fold into one row and open on demand.
    await expect(panel.getByRole("heading", { name: "Fayllar" })).toHaveCount(0);
    await panel.getByRole("button", { name: "+ Fayl" }).click();
    await expect(panel.getByRole("heading", { name: /Fayllar/ })).toBeVisible();
    await expect(panel.getByRole("button", { name: "+ Fayl" })).toHaveCount(0);
  });

  test("a yes/no cell toggles in one click and can be undone", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,field.paid`);
    const cell = page.getByRole("checkbox", { name: "Ödədi, QA-2" });
    await expect(cell).toHaveAttribute("aria-checked", "false");
    await cell.click();
    await expect(cell).toHaveAttribute("aria-checked", "true");
    const toast = page.getByRole("status").filter({ hasText: "QA-2: Ödədi — Bəli" });
    await expect(toast).toBeVisible();
    // Stored: a reload shows it.
    await page.reload();
    await expect(page.getByRole("checkbox", { name: "Ödədi, QA-2" })).toHaveAttribute("aria-checked", "true");
    await page.getByRole("checkbox", { name: "Ödədi, QA-2" }).click(); // back to no
    await expect(page.getByRole("checkbox", { name: "Ödədi, QA-2" })).toHaveAttribute("aria-checked", "false");
    await page.getByRole("status").filter({ hasText: "QA-2: Ödədi — Xeyr" }).getByRole("button", { name: "Geri qaytar" }).click();
    await expect(page.getByRole("checkbox", { name: "Ödədi, QA-2" })).toHaveAttribute("aria-checked", "true");
    await page.reload();
    await expect(page.getByRole("checkbox", { name: "Ödədi, QA-2" })).toHaveAttribute("aria-checked", "true");
  });

  test("on a phone each row shows the phone (tap to call) and amounts under the name", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`${seed.url}?cols=key,title,status,field.phone,field.balance`);
    const row = page.getByRole("row").filter({ hasText: "Kullinan MMC" });
    await expect(row.getByRole("link", { name: "+994 51 561 41 45" }).first()).toHaveAttribute("href", "tel:+994515614145");
    await expect(row).toContainText("Qalıq: 800,00 ₼");
    const link = await row.getByRole("link", { name: "+994 51 561 41 45" }).first().boundingBox();
    expect((link?.x ?? 999) + (link?.width ?? 0)).toBeLessThan(390); // visible without scrolling
  });

  test("move through the list from the panel with buttons and J/K", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title&sort=number`);
    await page.getByRole("button", { name: "Kullinan MMC" }).click();
    const nav = page.getByRole("group", { name: "Siyahıda keçid" });
    await expect(nav).toContainText("1 / 2");
    await nav.getByRole("button", { name: "Növbəti tapşırıq" }).click();
    await expect(page.getByRole("dialog", { name: "QA-1" })).toBeVisible();
    await expect(nav).toContainText("2 / 2");
    await expect(nav.getByRole("button", { name: "Növbəti tapşırıq" })).toBeDisabled();
    await page.keyboard.press("k");
    await expect(page.getByRole("dialog", { name: "QA-2" })).toBeVisible();
    // Not while typing.
    await page.getByRole("dialog", { name: "QA-2" }).getByLabel("Başlıq").focus();
    await page.keyboard.press("j");
    await expect(page.getByRole("dialog", { name: "QA-2" })).toBeVisible();
  });

  test("call results write the history; a promise moves the due date", async ({ page }) => {
    await page.goto(`${seed.url}?item=1`);
    const panel = page.getByRole("dialog", { name: "QA-1" });
    const calls = panel.getByRole("region", { name: "Zəng nəticəsi" });
    await calls.getByRole("button", { name: /Cavab vermədi/ }).click();
    await expect(panel.getByText("📞 Cavab vermədi")).toBeVisible();
    await calls.getByRole("button", { name: /Ödəməyə söz verdi/ }).click();
    await calls.getByLabel("Hansı tarixə").fill("2030-12-05");
    await calls.getByRole("button", { name: /Saxla/ }).click();
    await expect(panel.getByText("📞 Ödəməyə söz verdi: 5 dek 2030")).toBeVisible();
    await expect(panel.getByLabel("Son tarix")).toHaveValue("2030-12-05");
    await page.reload();
    await expect(page.getByRole("dialog", { name: "QA-1" }).getByLabel("Son tarix")).toHaveValue("2030-12-05");
  });

  test("the title stays while the table scrolls sideways", async ({ page }) => {
    const many = ["key", "title", "status", "assignee", "due_date", "created_at", "updated_at", "field.balance", "field.phone", "field.nv"];
    await page.setViewportSize({ width: 900, height: 700 });
    await page.goto(`${seed.url}?cols=${many.join(",")}`);
    const title = page.getByRole("row").filter({ hasText: "Kullinan MMC" }).getByRole("cell").nth(1);
    const before = await title.boundingBox();
    await page.getByRole("table").evaluate((el) => { el.scrollLeft = 600; });
    await expect.poll(async () => (await page.getByRole("table").evaluate((el) => el.scrollLeft))).toBeGreaterThan(100);
    const after = await title.boundingBox();
    expect(Math.abs((after?.x ?? 0) - (before?.x ?? 0))).toBeLessThan(20);
  });

  test("an admin sets a number field's unit in the settings", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/settings/fields`);
    const unit = page.getByRole("textbox", { name: /^Vahid/ }).first();
    await unit.fill("AZN");
    await page.getByRole("button", { name: "Vahidi saxla" }).click();
    // Saved: the button goes once the field comes back with the new unit.
    await expect(page.getByRole("button", { name: "Vahidi saxla" })).toHaveCount(0);
    await page.goto(`${seed.url}?cols=key,title,field.balance`);
    await expect(page.getByRole("row").filter({ hasText: "Kullinan MMC" })).toContainText("800,00 AZN");
  });
});

test.describe("item menu and skip link", () => {
  test.use({ storageState: authFile("alice") });

  test("delete sits in the panel menu and asks first; the skip link reaches the page", async ({ page }) => {
    const alice = await apiAs("alice");
    const r = await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Silinəcək qeyd" } });
    const n = ((await r.json()) as { number: number }).number;
    await alice.dispose();
    await page.goto(`${seed.url}?item=${String(n)}`);
    const panel = page.getByRole("dialog", { name: `QA-${String(n)}` });
    await expect(panel.getByRole("button", { name: "Sil", exact: true })).toHaveCount(0);
    await panel.getByLabel("Digər əməliyyatlar").click();
    await panel.getByRole("button", { name: "Sil", exact: true }).click();
    await page.getByRole("dialog").getByRole("button", { name: /QA-\d+.* sil|Sil/ }).last().click();
    await expect(page.getByRole("row").filter({ hasText: "Silinəcək qeyd" })).toHaveCount(0);

    await page.goto(seed.url);
    await expect(page.getByRole("button", { name: "Yeni tapşırıq" })).toBeVisible();
    await page.locator("body").focus();
    await page.keyboard.press("Tab");
    const skip = page.getByRole("link", { name: "Məzmuna keç" });
    await expect(skip).toBeFocused();
    await skip.press("Enter");
    await expect(page.locator("main#main")).toBeFocused();
  });
});

test.describe("report amounts", () => {
  test.use({ storageState: authFile("alice") });

  test("a one-project report sums amount fields over open and closed items", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/reports?projects=QA`);
    const w = page.getByTestId("w-amounts");
    // The unit is ₼ or AZN (another test in this file renames it).
    const row = w.getByRole("row", { name: /Qalıq/ });
    await expect(row.getByRole("cell").nth(0)).toHaveText(/2\s000,00\s(₼|AZN)/);
    await expect(row.getByRole("cell").nth(1)).toHaveText("—"); // nothing closed yet
  });
});

test.describe("column filters and density", () => {
  test.use({ storageState: authFile("alice") });

  test("a column menu filters by range and by contained text; chips describe them", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,field.balance,field.phone`);
    await page.getByRole("button", { name: "Qalıq: filtr və sıralama" }).click();
    const menu = page.getByRole("dialog", { name: "Qalıq: filtr və sıralama" });
    await menu.getByLabel("Başlanğıc").fill("1000");
    await menu.getByRole("button", { name: "Tətbiq et" }).click();
    await expect(page).toHaveURL(/field\.balance=1000\.\./);
    await expect(page.getByRole("row").filter({ hasText: "Kullinan MMC" })).toHaveCount(0);
    await expect(page.getByRole("row").filter({ hasText: "Rəşad Quliyev" })).toHaveCount(1);
    await expect(page.getByRole("list", { name: "Aktiv filtrlər" })).toContainText("Qalıq: ≥ 1000");
    // Focus returns to the column button (WCAG 2.4.3).
    await expect(page.getByRole("button", { name: /Qalıq: filtr və sıralama, filtr var/ })).toBeFocused();

    await page.getByRole("button", { name: /Əlaqə nömrəsi: filtr və sıralama/ }).click();
    const phone = page.getByRole("dialog", { name: "Əlaqə nömrəsi: filtr və sıralama" });
    await phone.getByLabel("İçində").fill("5614");
    await phone.getByLabel("İçində").press("Enter");
    await expect(page.getByRole("list", { name: "Aktiv filtrlər" })).toContainText('Əlaqə nömrəsi: içində "5614"');
    await expect(page.getByRole("row").filter({ hasText: "Rəşad Quliyev" })).toHaveCount(1);

    // Empty / not empty: every row has a phone.
    await page.getByRole("button", { name: /Əlaqə nömrəsi: filtr və sıralama/ }).click();
    await page.getByRole("dialog", { name: "Əlaqə nömrəsi: filtr və sıralama" }).getByRole("button", { name: "Boş olanlar" }).click();
    await expect(page.getByRole("list", { name: "Aktiv filtrlər" })).toContainText("Əlaqə nömrəsi: boşdur");
    await expect(page.getByRole("row").filter({ hasText: "Rəşad Quliyev" })).toHaveCount(0);
    // No rows left: the table header stays (with a "no matching items" note); the chip removes the filter.
    await page.getByRole("button", { name: "Filtri götür: Əlaqə nömrəsi: boşdur" }).click();
    await page.getByRole("button", { name: /Əlaqə nömrəsi: filtr və sıralama/ }).click();
    await page.getByRole("dialog", { name: "Əlaqə nömrəsi: filtr və sıralama" }).getByRole("button", { name: "Dolu olanlar" }).click();
    await expect(page.getByRole("row").filter({ hasText: "Rəşad Quliyev" })).toHaveCount(1);

    // Clear from the menu; Escape closes without changes.
    await page.getByRole("button", { name: /Qalıq: filtr və sıralama/ }).click();
    await page.getByRole("dialog", { name: "Qalıq: filtr və sıralama" }).getByRole("button", { name: "Təmizlə" }).click();
    await expect(page).not.toHaveURL(/field\.balance=/);
    await page.getByRole("button", { name: /Əlaqə nömrəsi: filtr və sıralama/ }).click();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", { name: "Əlaqə nömrəsi: filtr və sıralama" })).toHaveCount(0);
  });

  test("a text column filter follows what is typed, no button needed; the menu stays open", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,field.balance,field.phone`);
    await page.getByRole("button", { name: /Əlaqə nömrəsi: filtr və sıralama/ }).click();
    const phone = page.getByRole("dialog", { name: "Əlaqə nömrəsi: filtr və sıralama" });
    await phone.getByLabel("İçində").pressSequentially("5614");
    await expect(page).toHaveURL(/field\.phone=(~|%7E)5614/);
    await expect(page.getByRole("row").filter({ hasText: "Rəşad Quliyev" })).toHaveCount(1);
    await expect(phone).toBeVisible();
    // Nothing matches: the menu stays open over the empty table and the text can be fixed.
    await phone.getByLabel("İçində").pressSequentially("zz");
    await expect(page.getByRole("status").filter({ hasText: "Bu filtrlərə uyğun tapşırıq yoxdur." })).toBeVisible();
    await expect(phone).toBeVisible();
    await phone.getByLabel("İçində").press("Backspace");
    await phone.getByLabel("İçində").press("Backspace");
    await expect(page.getByRole("row").filter({ hasText: "Rəşad Quliyev" })).toHaveCount(1);
    await expect(phone.getByLabel("İçində")).toHaveValue("5614");
    // Enter closes the menu and keeps the filter.
    await phone.getByLabel("İçində").press("Enter");
    await expect(phone).toHaveCount(0);
    await expect(page).toHaveURL(/field\.phone=(~|%7E)5614/);
  });

  test("drag or arrow-key a column wider; the width is remembered, Home resets it", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,field.balance,field.phone`);
    const header = page.getByRole("columnheader", { name: /Qalıq/ });
    const handle = page.getByRole("separator", { name: /Qalıq: sütun eni/ });
    const w0 = (await header.boundingBox())?.width ?? 0;
    const box = await handle.boundingBox();
    if (!box) throw new Error("no resize handle");
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width / 2 + 80, box.y + box.height / 2, { steps: 4 });
    await page.mouse.up();
    await expect.poll(async () => (await header.boundingBox())?.width ?? 0).toBeGreaterThan(w0 + 60);

    await handle.focus();
    const w1 = Number(await handle.getAttribute("aria-valuenow"));
    await page.keyboard.press("ArrowRight");
    await expect(handle).toHaveAttribute("aria-valuenow", String(w1 + 0.5));
    await page.reload();
    await expect(page.getByRole("separator", { name: /Qalıq: sütun eni/ })).toHaveAttribute("aria-valuenow", String(w1 + 0.5));
    await page.getByRole("separator", { name: /Qalıq: sütun eni/ }).focus();
    await page.keyboard.press("Home");
    await expect.poll(async () => Math.round((await header.boundingBox())?.width ?? 0)).toBe(Math.round(w0));
  });

  test("sort from the column menu, and the compact density is remembered", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,field.balance`);
    await page.getByRole("button", { name: "Qalıq: filtr və sıralama" }).click();
    await page.getByRole("dialog", { name: "Qalıq: filtr və sıralama" }).getByRole("button", { name: /Azalan sırala/ }).click();
    await expect(page).toHaveURL(/sort=-field\.balance/);
    await expect(page.getByRole("row").nth(1)).toContainText("Rəşad Quliyev");

    const toggle = page.getByRole("button", { name: "Sıx" });
    await expect(toggle).toHaveAttribute("aria-pressed", "false");
    const before = (await page.getByRole("row").nth(1).boundingBox())?.height ?? 0;
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await page.reload();
    await expect(page.getByRole("button", { name: "Sıx" })).toHaveAttribute("aria-pressed", "true");
    const after = (await page.getByRole("row").nth(1).boundingBox())?.height ?? 0;
    expect(after).toBeLessThan(before);
    await page.getByRole("button", { name: "Sıx" }).click(); // leave the browser state as found
  });
});

test.describe("as guest", () => {
  test.use({ storageState: authFile("bob") });

  test("read-only rows link phone numbers and show no empty-section buttons", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,field.phone`);
    await expect(page.getByRole("row").filter({ hasText: "Kullinan MMC" }).getByRole("link", { name: "+994 51 561 41 45" })).toHaveAttribute("href", "tel:+994515614145");
    await page.getByRole("button", { name: "Kullinan MMC" }).click();
    await expect(page.getByRole("dialog", { name: "QA-2" }).getByRole("button", { name: "+ Fayl" })).toHaveCount(0);
  });
});
