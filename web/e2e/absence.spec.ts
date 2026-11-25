import { apiAs, apiAsCarol, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Approval delegation for absences (ADR-0042). Only bob may approve the
// "Done" move; bob records an absence with alice as his delegate. carol
// (member) asks for QA-1, so alice approves it on bob's behalf; alice asks
// for QA-2 herself, so she cannot approve it, also not through bob.
// Synthetic data only.
test.describe.configure({ mode: "serial" });

let seed: Seed;
let doneId = "";

test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const ws = `workspaces/${seed.ws}`;
  const alice = await apiAs("alice");
  const carol = await apiAsCarol();
  expect((await alice.post(`${ws}/members`, { data: { email: carol.email, role: "member" } })).status()).toBe(201);
  const wfs = (await (await alice.get(`${ws}/workflows`)).json()) as {
    workflows: { id: string; name: string; statuses: { id: string; name: string }[] }[];
  };
  const wf = wfs.workflows.find((w) => w.name === "Default") ?? wfs.workflows[0];
  doneId = wf.statuses.find((s) => s.name === "Done")?.id ?? "";
  const pol = await alice.put(`${ws}/workflows/${wf.id}/statuses/${doneId}/approval-policy`, {
    data: { min_approvals: 1, approver_roles: [], approver_user_ids: [seed.bobId] },
  });
  expect(pol.status(), await pol.text()).toBe(200);
  for (const [who, title] of [[carol.api, "Ödəniş 3001"], [alice, "Ödəniş 3002"]] as const) {
    const res = await who.post(`${ws}/projects/QA/items`, { data: { title } });
    expect(res.status(), await res.text()).toBe(201);
    const n = ((await res.json()) as { number: number }).number;
    const req = await who.post(`${ws}/projects/QA/items/${String(n)}/approvals`, { data: { version: 1, to_status_id: doneId } });
    expect(req.status(), await req.text()).toBe(201);
  }
  await alice.dispose();
  await carol.api.dispose();
});

test.describe("the absent approver", () => {
  test.use({ storageState: authFile("bob") });

  test("bob records an absence with alice as his delegate", async ({ page }) => {
    await page.goto(`/w/${seed.ws}`);
    // The user menu leads to "Məzuniyyət və əvəzləmə".
    await page.locator("summary[aria-label='Hesab: Bob Developer']").click();
    await page.getByRole("link", { name: "Məzuniyyət və əvəzləmə" }).click();
    await expect(page.getByRole("heading", { name: "Məzuniyyət və əvəzləmə", exact: true })).toBeVisible();
    await expect(page.getByText("Qeyd olunmuş məzuniyyətiniz yoxdur.")).toBeVisible();

    await page.getByRole("button", { name: "Məzuniyyət əlavə et" }).click();
    const form = page.getByRole("dialog", { name: "Yeni məzuniyyət" });
    await expect(form.getByText(/Tarixlər workspace-in saat qurşağındadır/)).toBeVisible();
    const to = await form.getByLabel("Son gün").inputValue();
    const later = new Date(to + "T00:00:00Z");
    later.setUTCDate(later.getUTCDate() + 3);
    await form.getByLabel("Son gün").fill(later.toISOString().slice(0, 10));
    await form.getByLabel("Əvəzləyən").selectOption({ label: "Alice Developer" });
    await form.getByLabel("Qeyd (istəyə bağlı)").fill("Ezamiyyət");
    await form.getByRole("button", { name: "Yadda saxla" }).click();

    const list = page.getByTestId("absence-list");
    await expect(list).toContainText("İndi");
    await expect(list).toContainText("Əvəzləyən: Alice Developer");
    await expect(list).toContainText("Ezamiyyət");
    // The banner tells bob who stands in for him.
    await expect(page.getByTestId("absence-banner")).toContainText("Təsdiqlərinizi Alice Developer əvəzləyir.");
  });
});

test.describe("the delegate", () => {
  test.use({ storageState: authFile("alice") });

  test("alice sees the delegation and approves on bob's behalf", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/approvals`);
    await expect(page.getByTestId("absence-banner")).toContainText("Bu gün Bob Developer adına təsdiq qərarı verə bilərsiniz.");
    const row = page.getByTestId("approval-inbox-row").filter({ hasText: "QA-1" });
    await expect(row.getByTestId("inbox-on-behalf")).toHaveText(/Bob Developer adına/);
    // Her own request is not hers to decide.
    await expect(page.getByTestId("approval-inbox-row").filter({ hasText: "QA-2" })).toHaveCount(0);
    await row.click();

    const approval = page.getByRole("dialog", { name: "QA-1" }).getByRole("region", { name: "Təsdiq", exact: true });
    await expect(approval.getByTestId("approval-absences")).toContainText("Bob Developer");
    await expect(approval.getByTestId("approval-absences")).toContainText("əvəzləyən: Alice Developer");
    await expect(approval.getByRole("button", { name: "Təsdiqlə", exact: true })).toHaveCount(0);
    await approval.getByRole("button", { name: "Bob Developer adına təsdiqlə" }).click();
    await expect(approval.getByTestId("approval-closed")).toContainText("Təsdiqlənib");
    await expect(approval.getByTestId("decision-on-behalf")).toHaveText("(Bob Developer adına)");
  });

  test("alice cannot approve her own request through the delegation", async ({ page }) => {
    await page.goto(seed.url + "?item=2");
    const approval = page.getByRole("dialog", { name: "QA-2" }).getByRole("region", { name: "Təsdiq", exact: true });
    await expect(approval.getByTestId("approval-open")).toBeVisible();
    await expect(approval.getByTestId("approval-absences")).toContainText("əvəzləyən yoxdur");
    await expect(approval.getByRole("button", { name: /adına təsdiqlə/ })).toHaveCount(0);
    await expect(approval.getByRole("button", { name: "Təsdiqlə", exact: true })).toHaveCount(0);
    // The server refuses it too.
    const alice = await apiAs("alice");
    const base = `workspaces/${seed.ws}/projects/QA/items/2/approvals`;
    const list = (await (await alice.get(base)).json()) as { approvals: { id: string }[] };
    const res = await alice.post(`${base}/${list.approvals[0]?.id ?? ""}/approve`, { data: { on_behalf_of: seed.bobId } });
    expect(res.status()).toBe(403);
    expect(((await res.json()) as { error: { code: string } }).error.code).toBe("self_approval");
    await alice.dispose();
  });

  test("the admin view lists the absence; the page fits 320 px", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/settings/members`);
    const card = page.getByRole("region", { name: "Məzuniyyətlər" });
    await expect(card.getByTestId("absence-list")).toContainText("Bob Developer");
    await expect(page.getByTestId("member-away")).toContainText("əvəzləyən: Alice Developer");
    await page.setViewportSize({ width: 320, height: 720 });
    await page.goto(`/w/${seed.ws}/absence`);
    await expect(page.getByRole("heading", { name: "Məzuniyyət və əvəzləmə", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Məzuniyyət əlavə et" }).click();
    await expect(page.getByRole("dialog", { name: "Yeni məzuniyyət" })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320);
  });
});
