import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// The in-app notification center (ADR-0017). bob's notifications span every
// workspace other specs create, so assertions are scoped to this seed's
// workspace and item titles, never to global counts.

test.use({ storageState: authFile("bob") });
test.describe.configure({ mode: "serial" }); // the second test marks everything read

interface Note {
  id: number;
  kind: string;
  read: boolean;
  workspace_slug: string;
  item_title: string;
}

let seed: Seed;
const title = `Bildiriş testi ${Math.random().toString(36).slice(2, 8)}`;

async function bobsNotes(unread = false): Promise<Note[]> {
  const bob = await apiAs("bob");
  const r = await bob.get(`me/notifications?limit=100${unread ? "&unread=true" : ""}`);
  expect(r.ok()).toBeTruthy();
  const body = (await r.json()) as { notifications: Note[] };
  await bob.dispose();
  return body.notifications.filter((n) => n.workspace_slug === seed.ws);
}

test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const alice = await apiAs("alice");
  expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title, assignee_id: seed.bobId } })).status()).toBe(201);
  expect(
    (await alice.post(`workspaces/${seed.ws}/projects/QA/items/1/comments`, { data: { body: `Baxın <@${seed.bobId}>, zəhmət olmasa` } })).status(),
  ).toBe(201);
  await alice.dispose();
  // Created asynchronously by the worker (outbox → notifications job).
  await expect.poll(async () => (await bobsNotes()).map((n) => n.kind).sort(), { timeout: 15_000 }).toEqual(["assigned", "mention"]);
});

test("bell shows unread notifications, opens the item and marks it read", async ({ page }) => {
  await page.goto(`/w/${seed.ws}`);
  const bell = page.getByRole("button", { name: /^Bildirişlər, (\d+|99-dan çox) oxunmamış$/ });
  await expect(bell).toBeVisible();

  // Keyboard: open, focus lands in the list, Escape closes and returns focus.
  await bell.focus();
  await page.keyboard.press("Enter");
  const panel = page.getByRole("dialog", { name: "Bildirişlər" });
  await expect(panel).toBeVisible();
  const mention = panel.getByRole("link").filter({ hasText: "Alice Developer sizi qeyd etdi" }).filter({ hasText: title });
  const assigned = panel.getByRole("link").filter({ hasText: "Alice Developer tərəfindən sizə təyin edildi" }).filter({ hasText: title });
  await expect(mention).toBeVisible();
  await expect(assigned).toBeVisible();
  await expect(mention).toContainText("oxunmamış");
  await page.keyboard.press("Escape");
  await expect(panel).toBeHidden();
  await expect(bell).toBeFocused();

  // Click: deep link to the item drawer; the notification becomes read.
  await bell.click();
  await mention.click();
  await expect(page).toHaveURL(new RegExp(`/w/${seed.ws}/p/QA\\?item=1$`));
  await expect(page.getByRole("dialog", { name: "QA-1" })).toBeVisible();
  await expect.poll(async () => (await bobsNotes(true)).map((n) => n.kind)).toEqual(["assigned"]);
});

test("Hamısını oxunmuş et clears the unread state", async ({ page }) => {
  await page.goto(`/w/${seed.ws}`);
  await page.getByRole("button", { name: /^Bildirişlər/ }).click();
  const panel = page.getByRole("dialog", { name: "Bildirişlər" });
  await panel.getByRole("button", { name: "Hamısını oxunmuş et" }).click();
  await expect.poll(async () => (await bobsNotes(true)).length).toBe(0);
  await expect(panel.getByRole("link").filter({ hasText: title }).first()).not.toContainText("oxunmamış");
});
