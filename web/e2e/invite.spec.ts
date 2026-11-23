import { request } from "@playwright/test";

import { apiAs, authFile, expect, MAILPIT, test } from "./fixtures";

// A workspace Bob is not in yet; the slug follows the cleanup pattern.
const ws = `e2e-${process.env.E2E_RUN ?? "manual"}-inv${Math.random().toString(36).slice(2, 7)}`;

test.beforeAll(async () => {
  const alice = await apiAs("alice");
  expect((await alice.post("workspaces", { data: { slug: ws, name: "Dəvət testi" } })).status()).toBe(201);
  await alice.dispose();
});

/** The newest invitation link sent to an address, read from Mailpit. */
async function inviteLink(to: string, since: number): Promise<string> {
  const mail = await request.newContext();
  let link = "";
  await expect
    .poll(async () => {
      const q = encodeURIComponent(`to:${to} subject:"Dəvət testi"`);
      const found = (await (await mail.get(`${MAILPIT}/search?query=${q}`)).json()) as { messages: { ID: string; Created: string }[] };
      const fresh = found.messages.filter((m) => Date.parse(m.Created) >= since);
      const newest = fresh.at(0);
      if (!newest) return "";
      const msg = (await (await mail.get(`${MAILPIT}/message/${newest.ID}`)).json()) as { Text: string };
      link = /http:\/\/localhost:5173(\/invite\/[A-Za-z0-9_-]{43})/.exec(msg.Text)?.[1] ?? "";
      return link;
    }, { timeout: 20_000 })
    .not.toBe("");
  await mail.dispose();
  return link;
}

test("an admin invites someone by email; only that person can accept", async ({ browser }) => {
  const since = Date.now() - 1000;
  const alice = await (await browser.newContext({ storageState: authFile("alice") })).newPage();
  await alice.goto(`/w/${ws}/settings/members`);
  await alice.getByLabel("Email").fill("bob@example.com");
  await alice.getByRole("button", { name: "Dəvət et" }).click();
  await expect(alice.getByText("Dəvət göndərildi").first()).toBeVisible();
  const pending = alice.getByRole("region", { name: "Gözləyən dəvətlər" });
  await expect(pending.getByText("bob@example.com")).toBeVisible();

  const link = await inviteLink("bob@example.com", since);

  // A forwarded link does not work for another account.
  await alice.goto(link);
  await expect(alice.getByRole("alert")).toContainText("b***@example.com");
  await expect(alice.getByRole("button", { name: "Qəbul et" })).toHaveCount(0);

  // Bob accepts and lands in the workspace.
  const bob = await (await browser.newContext({ storageState: authFile("bob") })).newPage();
  await bob.goto(link);
  await expect(bob.getByText(/Alice Developer sizi "Dəvət testi" workspace-inə üzv kimi dəvət edib/)).toBeVisible();
  await bob.getByRole("button", { name: "Qəbul et" }).click();
  await expect(bob).toHaveURL(new RegExp(`/w/${ws}$`));

  // Used up: the link no longer works, and the pending list is empty.
  await bob.goto(link);
  await expect(bob.getByText(/artıq bu workspace-in üzvüsünüz|artıq istifadə olunub/)).toBeVisible();
  await alice.goto(`/w/${ws}/settings/members`);
  await expect(alice.getByRole("region", { name: "Gözləyən dəvətlər" })).toHaveCount(0);
  await expect(alice.getByText("Bob Developer").first()).toBeVisible();

  await alice.context().close();
  await bob.context().close();
});
