import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Item links and watchers (ADR-0028): add a link from the drawer, the
// "blocked" badge, closed linked items, the watch toggle, watcher
// notifications and hidden private items.
test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace(); // bob is a member
  const alice = await apiAs("alice");
  for (const title of ["Ay sonu bağlanışı", "Bank üzləşməsi", "Hesabatı göndər"]) {
    const res = await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title } }); // QA-1..3
    expect(res.status(), await res.text()).toBe(201);
  }
  await alice.dispose();
});

test("add a link from the drawer; both sides and the history show it", async ({ page }) => {
  await page.goto(seed.url + "?item=1");
  const panel = page.getByRole("dialog", { name: "QA-1" });
  const section = panel.getByRole("region", { name: "Əlaqələr" });
  await expect(section.getByText("Əlaqə yoxdur.")).toBeVisible();

  await section.getByRole("button", { name: /Əlaqə əlavə et/ }).click();
  await section.getByLabel("Əlaqə növü").selectOption({ label: "Bloklanır" });
  await section.getByLabel("Tapşırıq axtar").first().fill("QA-2");
  await section.getByRole("button", { name: /QA-2.*Bank üzləşməsi/ }).click();
  await expect(page.getByText("Əlaqə əlavə edildi")).toBeVisible();
  await expect(section.getByRole("heading", { name: "Bloklanır" })).toBeVisible();
  await expect(section.getByTestId("item-link").filter({ hasText: "QA-2" })).toBeVisible();

  await panel.getByRole("tab", { name: "Aktivlik" }).click();
  await expect(panel.getByText("əlaqə əlavə etdi: bloklanır QA-2")).toBeVisible();

  // The other side, reached through the link.
  await section.getByRole("link", { name: /QA-2/ }).click();
  const other = page.getByRole("dialog", { name: "QA-2" });
  await expect(other.getByRole("region", { name: "Əlaqələr" }).getByRole("heading", { name: "Bloklayır" })).toBeVisible();
});

test("an open blocker shows a badge; a done one is struck through", async ({ page }) => {
  const alice = await apiAs("alice");
  // Idempotent with the previous test: a second identical link is a 409.
  const add = await alice.post(`workspaces/${seed.ws}/projects/QA/items/2/links`, { data: { type: "blocks", target: "QA-1" } });
  expect([201, 409]).toContain(add.status());

  await page.goto(seed.url);
  const row = page.getByRole("row").filter({ hasText: "Ay sonu bağlanışı" });
  await expect(row.getByTestId("blocked-badge")).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "Hesabatı göndər" }).getByTestId("blocked-badge")).toHaveCount(0);

  const types = (await (await alice.get(`workspaces/${seed.ws}/item-types`)).json()) as {
    item_types: { key: string; statuses: { id: string; name: string }[] }[];
  };
  const done = types.item_types.find((t) => t.key === "task")?.statuses.find((s) => s.name === "Done");
  const blocker = (await (await alice.get(`workspaces/${seed.ws}/projects/QA/items/2`)).json()) as { version: number };
  const res = await alice.patch(`workspaces/${seed.ws}/projects/QA/items/2`, { data: { version: blocker.version, status_id: done?.id } });
  expect(res.status(), await res.text()).toBe(200);
  await alice.dispose();

  await page.goto(seed.url + "?item=1");
  await expect(page.getByRole("row").filter({ hasText: "Ay sonu bağlanışı" }).getByTestId("blocked-badge")).toHaveCount(0);
  const link = page.getByRole("dialog", { name: "QA-1" }).getByTestId("item-link").filter({ hasText: "QA-2" });
  await expect(link.getByText("(bağlıdır)")).toBeAttached();
  await expect(link.locator(".line-through")).toBeVisible();
});

test("watch toggle, watcher list and a watcher's notification", async ({ page }) => {
  await page.goto(seed.url + "?item=3");
  const panel = page.getByRole("dialog", { name: "QA-3" });
  // alice reported QA-3, so she watches it.
  const toggle = panel.getByTestId("watch-toggle");
  await expect(toggle).toHaveAttribute("aria-pressed", "true");
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-pressed", "false");
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-pressed", "true");

  // As an admin, alice adds bob from the watcher panel.
  await panel.getByTestId("watcher-count").click();
  const list = panel.getByRole("dialog", { name: "İzləyənlər" });
  await list.getByLabel("İzləyən əlavə et").selectOption({ label: "Bob Developer" });
  await expect(list.getByText("Bob Developer")).toBeVisible();
  await expect(panel.getByTestId("watcher-count")).toHaveText("2");

  // alice changes the due date: bob, a watcher, is notified; alice is not.
  await page.keyboard.press("Escape");
  await panel.getByLabel("Son tarix").fill("2030-03-31");
  await panel.getByLabel("Son tarix").blur();
  const bob = await apiAs("bob");
  await expect
    .poll(async () => {
      const res = await bob.get("me/notifications");
      const body = (await res.json()) as { notifications: { kind: string; item_key: string; workspace_slug: string }[] };
      return body.notifications.some((n) => n.workspace_slug === seed.ws && n.kind === "due_changed" && n.item_key === "QA-3");
    }, { timeout: 30_000 })
    .toBe(true);
  await bob.dispose();
});

test("links to a private item stay invisible to people outside it", async () => {
  const alice = await apiAs("alice");
  const bob = await apiAs("bob");
  expect((await alice.post(`workspaces/${seed.ws}/projects`, { data: { key: "SEC", name: "Gizli", visibility: "private" } })).status()).toBe(201);
  expect((await alice.post(`workspaces/${seed.ws}/projects/SEC/items`, { data: { title: "Məxfi iş" } })).status()).toBe(201);
  expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items/3/links`, { data: { type: "relates", target: "SEC-1" } })).status()).toBe(201);

  const own = await (await alice.get(`workspaces/${seed.ws}/projects/QA/items/3/links`)).text();
  expect(own).toContain("SEC-1");
  for (const p of ["links", "activity"]) {
    const text = await (await bob.get(`workspaces/${seed.ws}/projects/QA/items/3/${p}`)).text();
    expect(text).not.toContain("SEC");
    expect(text).not.toContain("Məxfi");
  }
  const refused = await bob.post(`workspaces/${seed.ws}/projects/QA/items/3/links`, { data: { type: "blocks", target: "SEC-1" } });
  const missing = await bob.post(`workspaces/${seed.ws}/projects/QA/items/3/links`, { data: { type: "blocks", target: "SEC-99" } });
  expect(refused.status()).toBe(422);
  expect(await refused.text()).toBe(await missing.text());
  await alice.dispose();
  await bob.dispose();
});
