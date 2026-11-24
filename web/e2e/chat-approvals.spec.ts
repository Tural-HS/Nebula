import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Approval cards in chat channels (ADR-0031, step 1). What the card looks
// like and who receives it (private projects) is covered by Go tests against
// a fake receiver; here: the per-webhook toggle and the "Open in Nebula"
// deep link landing on the approval panel.
test.describe.configure({ mode: "serial" });
test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace("member");
});

type Hook = { id: string; url: string; events: string[] };
async function hooks(ws: string): Promise<Hook[]> {
  const alice = await apiAs("alice");
  const r = (await (await alice.get(`workspaces/${ws}/webhooks`)).json()) as { webhooks: Hook[] };
  await alice.dispose();
  return r.webhooks;
}

test("chat webhooks send approval events by default; the toggle edits the events list", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/settings/webhooks`);
  const form = page.locator("form").filter({ hasText: "URL" });
  await form.getByLabel("Format").selectOption("slack");
  const toggle = form.getByLabel("Təsdiq hadisələrini göndər");
  await expect(toggle).toBeChecked();
  await expect(form.getByLabel(/^Hadisələr/)).toHaveValue("item.*, comment.created, approval.*");
  await expect(page.getByText(/«Nebula-da aç» keçidi gedir/)).toBeVisible();

  await toggle.uncheck();
  await expect(form.getByLabel(/^Hadisələr/)).toHaveValue("item.*, comment.created");
  await form.getByLabel("URL").fill("https://chat.example.com/hooks/abcdefghijklmnopqrstuvwxyz");
  await form.getByRole("button", { name: "Yarat" }).click();
  await expect.poll(async () => (await hooks(seed.ws)).map((h) => h.events)).toEqual([["comment.created", "item.*"]]);

  // Ticking it on the saved webhook's card saves at once.
  const card = page.getByText("https://chat.example.com/…wxyz").locator("xpath=ancestor::*[.//button[normalize-space()='Test göndər']][1]");
  const cardToggle = card.getByLabel("Təsdiq hadisələrini göndər");
  await expect(cardToggle).not.toBeChecked();
  await cardToggle.check();
  await expect.poll(async () => (await hooks(seed.ws))[0]?.events).toEqual(["approval.*", "comment.created", "item.*"]);
  await expect(card.getByLabel("Hadisələr")).toHaveValue("comment.created, item.*, approval.*");
});

test("a '*' subscription already includes approvals and cannot untick them", async ({ page }) => {
  await page.goto(`/w/${seed.ws}/settings/webhooks`);
  const form = page.locator("form").filter({ hasText: "URL" });
  await form.getByLabel(/^Hadisələr/).fill("*");
  const toggle = form.getByLabel("Təsdiq hadisələrini göndər");
  await expect(toggle).toBeChecked();
  await expect(toggle).toBeDisabled();
});

test("the card's link opens the item on its approval panel", async ({ page }) => {
  const alice = await apiAs("alice");
  const wfs = (await (await alice.get(`workspaces/${seed.ws}/workflows`)).json()) as {
    workflows: { id: string; name: string; statuses: { id: string; name: string }[] }[];
  };
  const wf = wfs.workflows.find((w) => w.name === "Default") ?? wfs.workflows[0];
  const done = wf.statuses.find((s) => s.name === "Done")?.id ?? "";
  const pol = await alice.put(`workspaces/${seed.ws}/workflows/${wf.id}/statuses/${done}/approval-policy`, {
    data: { min_approvals: 1, approver_roles: ["owner"], approver_user_ids: [] },
  });
  expect(pol.status(), await pol.text()).toBe(200);
  await alice.dispose();
  const bob = await apiAs("bob");
  for (let i = 0; i < 30; i++) await bob.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: `Sətir ${String(i)}` } });
  const it = await bob.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Hesab-faktura 2001", description: "x\n".repeat(80) } });
  const n = ((await it.json()) as { number: number }).number;
  const req = await bob.post(`workspaces/${seed.ws}/projects/QA/items/${String(n)}/approvals`, { data: { version: 1, to_status_id: done } });
  expect(req.status(), await req.text()).toBe(201);
  await bob.dispose();

  // Exactly the link the chat card carries (engine.ItemURL + "&focus=approval").
  await page.goto(`${seed.url}?item=${String(n)}&focus=approval`);
  const panel = page.getByRole("dialog", { name: `QA-${String(n)}` });
  const approval = panel.getByRole("region", { name: "Təsdiq" });
  await expect(approval).toBeFocused();
  await expect(approval).toBeInViewport();
  await expect(approval.getByRole("button", { name: "Təsdiqlə" })).toBeVisible();
  // The parameter is consumed: the drawer stays open, a reload does not jump.
  await expect(page).not.toHaveURL(/focus=/);
  await expect(page).toHaveURL(new RegExp(`item=${String(n)}`));
});
