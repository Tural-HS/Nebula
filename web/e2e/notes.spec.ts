import { apiAs, authFile, expect, test } from "./fixtures";

// Sticky notes on the home page (ADR-0052): add, colour, pin, edit, delete
// with undo; another user never sees them.
test.use({ storageState: authFile("alice") });

test.beforeAll(async () => {
  // Start from no notes (the dev database is shared between runs).
  const alice = await apiAs("alice");
  const { notes } = (await (await alice.get("me/notes")).json()) as { notes: { id: string }[] };
  for (const n of notes) await alice.delete(`me/notes/${n.id}`);
  await alice.dispose();
});

test("add, pin, edit and delete a note; undo brings it back", async ({ page }) => {
  await page.goto("/");
  const section = page.getByRole("region", { name: "Qeydlərim" });
  await expect(section.getByText(/Hələ qeyd yoxdur/)).toBeVisible();

  await section.getByRole("button", { name: "+ Qeyd" }).click();
  await section.getByLabel("Qeyd mətni").fill("Elmara zəng et — MAH hesabatı");
  await section.getByRole("radio", { name: "Yaşıl" }).check({ force: true });
  await section.getByRole("button", { name: "Saxla" }).click();
  const note = section.getByRole("article", { name: "Elmara zəng et — MAH hesabatı" });
  await expect(note).toBeVisible();
  await expect(note).toHaveClass(/bg-emerald-100/);

  await section.getByRole("button", { name: "+ Qeyd" }).click();
  await section.getByLabel("Qeyd mətni").fill("İkinci qeyd");
  await section.getByLabel("Qeyd mətni").press("Control+Enter");
  await expect(section.getByRole("article")).toHaveCount(2);
  await expect(section.getByRole("article").first()).toHaveAccessibleName("İkinci qeyd"); // newest first

  // Pinned goes first.
  await note.getByRole("button", { name: "Yuxarıda saxla" }).click();
  await expect(section.getByRole("article").first()).toHaveAccessibleName("Elmara zəng et — MAH hesabatı");
  await expect(section.getByRole("article").first().getByRole("button", { name: "Sancağı götür" })).toHaveAttribute("aria-pressed", "true");

  await section.getByRole("article", { name: "İkinci qeyd" }).getByRole("button", { name: "Redaktə et" }).click();
  await section.getByLabel("Qeyd mətni").fill("İkinci qeyd, dəyişdi");
  await section.getByRole("button", { name: "Saxla" }).click();
  const second = section.getByRole("article", { name: "İkinci qeyd, dəyişdi" });
  await expect(second).toBeVisible();

  await second.getByRole("button", { name: "Sil" }).click();
  await expect(section.getByRole("article")).toHaveCount(1);
  await page.getByRole("button", { name: "Geri qaytar" }).click();
  await expect(section.getByRole("article", { name: "İkinci qeyd, dəyişdi" })).toBeVisible();

  // Private: bob sees none of them.
  const bob = await apiAs("bob");
  const { notes } = (await (await bob.get("me/notes")).json()) as { notes: unknown[] };
  expect(notes.filter((n) => JSON.stringify(n).includes("Elmara"))).toHaveLength(0);
  await bob.dispose();
});
