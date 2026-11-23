import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Workspace-wide search (ADR-0024): Cmd/Ctrl+K, Azerbaijani folding,
// grouping, keyboard navigation, privacy and the phone layout. bob is a
// plain member who is not in the private project.

test.use({ storageState: authFile("bob") });

let seed: Seed;
const tag = Math.random().toString(36).slice(2, 7); // unique words: other specs' data never matches

test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const alice = await apiAs("alice");
  const post = async (path: string, data: object) => {
    const r = await alice.post(`workspaces/${seed.ws}/${path}`, { data });
    expect(r.status(), path).toBe(201);
  };
  await post("projects/QA/items", { title: `Ödəniş sistemi ${tag}`, description: "Müştəri kartla ödəyə bilmir." }); // QA-1
  await post("projects/QA/items", { title: `Giriş səhifəsi ${tag}` }); // QA-2
  await post("projects/QA/items/2/comments", { body: `Ödənişdən sonra yönləndirmə yoxdur ${tag}` });
  await post("projects", { key: "SEC", name: "Hüquq", visibility: "private" });
  await post("projects/SEC/items", { title: `Gizli ödəniş ${tag}` });
  await alice.dispose();
});

test("Ctrl/Cmd+K finds folded Azerbaijani text, groups it and opens the item from the keyboard", async ({ page }) => {
  await page.goto(`/w/${seed.ws}`);
  await page.keyboard.press("ControlOrMeta+k");
  const dialog = page.getByRole("dialog", { name: "Workspace-də axtarış" });
  const input = dialog.getByRole("combobox", { name: "Axtarış sorğusu" });
  await expect(input).toBeFocused();

  // ASCII query, Azerbaijani text: ö/ə/ş fold to o/e/s.
  await input.fill(`odenis ${tag}`);
  const list = dialog.getByRole("listbox");
  const items = list.getByRole("group", { name: /Tapşırıqlar/ });
  const comments = list.getByRole("group", { name: /Şərhlər/ });
  await expect(items.getByRole("option")).toHaveCount(1);
  await expect(items.getByRole("option").first()).toContainText("QA-1");
  await expect(items.locator("mark").first()).toHaveText("Ödəniş");
  await expect(comments.getByRole("option")).toHaveCount(1);
  await expect(comments.getByRole("option").first()).toContainText("Alice Developer şərhi");
  // The private project's item is not there, not even in the counts.
  await expect(dialog).not.toContainText("SEC-");
  await expect(dialog.getByRole("status")).toHaveText("Tapşırıqlar: 1, Şərhlər: 1");

  // First row active; ArrowDown moves to the comment; Enter opens its item.
  await expect(items.getByRole("option").first()).toHaveAttribute("aria-selected", "true");
  await input.press("ArrowDown");
  await expect(comments.getByRole("option").first()).toHaveAttribute("aria-selected", "true");
  await input.press("Enter");
  await expect(page).toHaveURL(new RegExp(`/w/${seed.ws}/p/QA\\?item=2$`));
  await expect(page.getByRole("dialog", { name: "QA-2" })).toBeVisible();
  await expect(dialog).toBeHidden();
});

test("an exact key comes first and Escape closes the dialog", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/p/QA`);
  const button = page.getByRole("button", { name: "Axtar" });
  await button.click();
  const dialog = page.getByRole("dialog", { name: "Workspace-də axtarış" });
  await dialog.getByRole("combobox").fill("qa-2");
  const first = dialog.getByRole("option").first();
  await expect(first).toContainText("QA-2");
  await expect(first).toContainText("Açar");
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(button).toBeFocused();

  // Bob cannot reach the private item by its key either.
  await button.click();
  await dialog.getByRole("combobox").fill("SEC-1");
  await expect(dialog.getByText("Heç nə tapılmadı.")).toBeVisible();
});

test("private items never come back from the API", async () => {
  const bob = await apiAs("bob");
  for (const q of [`gizli ${tag}`, `odenis ${tag}`, "SEC-1"]) {
    const r = await bob.get(`workspaces/${seed.ws}/search?q=${encodeURIComponent(q)}`);
    expect(r.ok()).toBeTruthy();
    const body = (await r.json()) as { hits: { item_key: string }[]; counts: { items: number; comments: number } };
    expect(body.hits.filter((h) => h.item_key.startsWith("SEC-"))).toEqual([]);
    if (q.startsWith("gizli")) expect(body.counts).toEqual({ items: 0, comments: 0 });
  }
  await bob.dispose();
});

test.describe("phone", () => {
  test.use({ viewport: { width: 375, height: 812 }, hasTouch: true, isMobile: true });

  test("the search fills the screen and still opens items", async ({ page }) => {
    await page.goto(`/w/${seed.ws}`);
    const button = page.getByRole("button", { name: "Axtar" });
    const box = await button.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44); // touch target (ADR-0019)
    await button.tap();
    const dialog = page.getByRole("dialog", { name: "Workspace-də axtarış" });
    const size = await dialog.boundingBox();
    expect(size?.width).toBe(375);
    await dialog.getByRole("combobox").fill(`giris ${tag}`);
    await dialog.getByRole("option").filter({ hasText: "QA-2" }).tap();
    await expect(page.getByRole("dialog", { name: "QA-2" })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375); // no sideways scroll
  });
});
