import { request } from "@playwright/test";

import { apiAs, authFile, expect, MAILPIT, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Şərh testi" } });
  await alice.dispose();
});

test("comment with an @mention notifies the mentioned member by email", async ({ page }) => {
  await page.goto(seed.url + "?item=1");
  const panel = page.getByRole("dialog", { name: "QA-1" });
  const box = panel.getByRole("textbox", { name: /Şərh yazın/ });
  await box.fill("Zəhmət olmasa baxın @Bo");
  await panel.getByRole("listbox").getByRole("option", { name: /Bob Developer/ }).click();
  await box.pressSequentially("bu gün.");
  await panel.getByRole("button", { name: "Göndər" }).click();

  // Rendered as a mention chip, never as the raw <@id> token.
  const comment = panel.getByRole("listitem").filter({ hasText: "bu gün." });
  await expect(comment).toContainText("@Bob Developer");
  await expect(comment).not.toContainText("<@");

  // The notification arrives through the outbox + worker (LISTEN/NOTIFY).
  const mail = await request.newContext();
  await expect
    .poll(
      async () => {
        const r = await mail.get(`${MAILPIT}/search?query=${encodeURIComponent(`to:bob@example.com subject:"QA-1"`)}`);
        const body = (await r.json()) as { messages: { Subject: string }[] };
        return body.messages.map((m) => m.Subject);
      },
      { timeout: 15_000 },
    )
    .toContainEqual(expect.stringContaining("Alice Developer mentioned you"));
  await mail.dispose();
});
