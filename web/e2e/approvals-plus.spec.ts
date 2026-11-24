import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Approval improvements (ADR-0034): the workspace inbox with its badge, the
// deadline chip, and re-approval when approved content changes. bob (member)
// prepares, alice (owner) approves. Synthetic data only.
test.describe.configure({ mode: "serial" });

let seed: Seed;
let workflowId = "";
let doneId = "";

test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const ws = `workspaces/${seed.ws}`;
  const alice = await apiAs("alice");
  expect((await alice.post(`${ws}/fields`, { data: { key: "amount", name: "Məbləğ", type: "number" } })).status()).toBe(201);
  expect((await alice.put(`${ws}/item-types/task/fields/amount`, { data: { position: 1 } })).status()).toBe(200);
  const wfs = (await (await alice.get(`${ws}/workflows`)).json()) as {
    workflows: { id: string; name: string; statuses: { id: string; name: string }[] }[];
  };
  const wf = wfs.workflows.find((w) => w.name === "Default") ?? wfs.workflows[0];
  workflowId = wf.id;
  doneId = wf.statuses.find((s) => s.name === "Done")?.id ?? "";
  const pol = await alice.put(`${ws}/workflows/${workflowId}/statuses/${doneId}/approval-policy`, {
    data: {
      min_approvals: 1, approver_roles: ["owner"], approver_user_ids: [],
      reapprove_on_change: true, reapprove_fields: ["fields.amount"], due_in_working_days: 2,
    },
  });
  expect(pol.status(), await pol.text()).toBe(200);

  const bob = await apiAs("bob");
  for (const [title, amount] of [["Hesab-faktura 2001", 1200], ["Hesab-faktura 2002", 800]] as const) {
    const res = await bob.post(`${ws}/projects/QA/items`, { data: { title, fields: { amount } } });
    expect(res.status(), await res.text()).toBe(201);
  }
  for (const n of [1, 2]) {
    const req = await bob.post(`${ws}/projects/QA/items/${String(n)}/approvals`, { data: { version: 1, to_status_id: doneId } });
    expect(req.status(), await req.text()).toBe(201);
  }
  // QA-2 is approved (and so in Done); QA-1 stays pending.
  const list = (await (await alice.get(`${ws}/projects/QA/items/2/approvals`)).json()) as { approvals: { id: string }[] };
  const ok = await alice.post(`${ws}/projects/QA/items/2/approvals/${list.approvals[0]?.id ?? ""}/approve`, { data: {} });
  expect(ok.status(), await ok.text()).toBe(200);
  await alice.dispose();
  await bob.dispose();
});

test.describe("as the approver", () => {
  test.use({ storageState: authFile("alice") });

  test("the workspace page links to the inbox with a badge", async ({ page }) => {
    await page.goto(`/w/${seed.ws}`);
    const link = page.getByRole("link", { name: /^Təsdiq gözləyənlər: 1 sorğu qərarınızı gözləyir/ });
    await expect(link).toBeVisible();
    await link.click();
    await expect(page).toHaveURL(new RegExp(`/w/${seed.ws}/approvals$`));
    await expect(page.getByRole("heading", { name: "Təsdiq gözləyənlər", exact: true })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Mənim qərarımı gözləyənlər", exact: true })).toHaveAttribute("aria-selected", "true");
  });

  test("an inbox row shows the request and opens the drawer at the approval panel", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/approvals`);
    const row = page.getByTestId("approval-inbox-row").filter({ hasText: "QA-1" });
    await expect(row).toContainText("Hesab-faktura 2001");
    await expect(row).toContainText("İstədi:");
    await expect(row).toContainText("Qərarınızı gözləyir");
    await expect(row.getByTestId("approval-due")).toContainText("Son gün:");
    await row.click();
    await expect(page).toHaveURL(/item=1/);
    const panel = page.getByRole("dialog", { name: "QA-1" });
    const approval = panel.getByRole("region", { name: "Təsdiq", exact: true });
    await expect(approval).toBeFocused();
    await expect(approval.getByTestId("approval-due")).toContainText("Son gün:");
    await expect(page).not.toHaveURL(/focus=approval/);
  });

  test("all pending lists the same request; after deciding the inbox is empty", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/approvals?view=all`);
    await expect(page.getByTestId("approval-inbox-row")).toHaveCount(1);
    await page.getByTestId("approval-inbox-row").click();
    const approval = page.getByRole("dialog", { name: "QA-1" }).getByRole("region", { name: "Təsdiq", exact: true });
    await approval.getByRole("button", { name: "Təsdiqlə", exact: true }).click();
    await expect(approval.getByTestId("approval-closed")).toContainText("Təsdiqlənib");
    await page.goto(`/w/${seed.ws}/approvals`);
    await expect(page.getByText("Qərarınızı gözləyən sorğu yoxdur.")).toBeVisible();
  });

  test("the policy editor shows the deadline and re-approval options", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/settings/workflows`);
    const summary = page.getByText(/1 təsdiq lazımdır · ⏰ 2 iş günü · ♻️/);
    await expect(summary).toBeVisible();
    await summary.click();
    // Every status has its own (collapsed) policy form: stay inside this one.
    const row = page.locator("details").filter({ has: summary });
    await expect(row.getByLabel("Təsdiqdən sonrakı dəyişiklik yenidən təsdiq tələb edir")).toBeChecked();
    await expect(row.getByLabel("Məbləğ", { exact: true })).toBeChecked();
    await expect(row.getByLabel("Qərar müddəti, iş günü")).toHaveValue("2");
  });
});

test.describe("re-approval", () => {
  test.use({ storageState: authFile("bob") });

  test("the approved item warns, and changing the amount sends it back for approval", async ({ page }) => {
    await page.goto(seed.url + "?item=2");
    const panel = page.getByRole("dialog", { name: "QA-2" });
    const approval = panel.getByRole("region", { name: "Təsdiq", exact: true });
    await expect(approval.getByTestId("reapprove-warning")).toContainText("Məbləğ");

    // Change the watched amount through the API (the drawer's field editor is
    // being redesigned in another round; the rule is enforced server-side).
    const bob = await apiAs("bob");
    const ws = `workspaces/${seed.ws}/projects/QA/items/2`;
    const item = (await (await bob.get(ws)).json()) as { version: number };
    const res = await bob.patch(ws, { data: { version: item.version, fields: { amount: 950 } } });
    expect(res.status(), await res.text()).toBe(200);
    const body = (await res.json()) as { status: { name: string }; approval: { requested_by: string } | null };
    expect(body.status.name).toBe("To Do");
    expect(body.approval?.requested_by).toBe(seed.bobId);
    await bob.dispose();

    await page.reload();
    await expect(approval.getByTestId("approval-open")).toContainText("yenidən təsdiq istənilir");
    await approval.getByText("Əvvəlki sorğular").click();
    await expect(approval.getByTestId("approval-invalidated")).toBeVisible();
    // bob changed it, so bob cannot approve it.
    await expect(approval.getByRole("button", { name: "Təsdiqlə", exact: true })).toHaveCount(0);
  });
});
