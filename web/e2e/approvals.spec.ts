import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Four-eyes approvals (ADR-0026): bob (member) prepares and requests, alice
// (owner) approves or rejects. The requester never gets approve buttons, the
// item stays locked while pending, lists show a badge and a filter.
test.describe.configure({ mode: "serial" });

let seed: Seed;
let workflowId = "";
let doneId = "";

test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const alice = await apiAs("alice");
  const wfs = (await (await alice.get(`workspaces/${seed.ws}/workflows`)).json()) as {
    workflows: { id: string; name: string; statuses: { id: string; name: string }[] }[];
  };
  const wf = wfs.workflows.find((w) => w.name === "Default") ?? wfs.workflows[0];
  workflowId = wf.id;
  doneId = wf.statuses.find((s) => s.name === "Done")?.id ?? "";
  const pol = await alice.put(`workspaces/${seed.ws}/workflows/${workflowId}/statuses/${doneId}/approval-policy`, {
    data: { min_approvals: 1, approver_roles: ["owner"], approver_user_ids: [] },
  });
  expect(pol.status(), await pol.text()).toBe(200);
  await alice.dispose();

  const bob = await apiAs("bob");
  for (const title of ["Hesab-faktura 1001", "Hesab-faktura 1002"]) {
    const res = await bob.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title } });
    expect(res.status(), await res.text()).toBe(201);
  }
  // QA-2 is requested through the API; QA-1 through the UI below.
  const req = await bob.post(`workspaces/${seed.ws}/projects/QA/items/2/approvals`, { data: { version: 1, to_status_id: doneId } });
  expect(req.status(), await req.text()).toBe(201);
  await bob.dispose();
});

test.describe("as the preparer", () => {
  test.use({ storageState: authFile("bob") });

  test("moving into a guarded status asks for approval and locks the item", async ({ page }) => {
    await page.goto(seed.url + "?item=1");
    const panel = page.getByRole("dialog", { name: "QA-1" });
    await panel.getByLabel("Status").selectOption({ label: "Done (təsdiq tələb edir)" });

    const ask = page.getByRole("dialog", { name: '"Done" statusu üçün təsdiq istə' });
    await ask.getByLabel("Şərh (istəyə bağlı)").fill("Qəbz əlavə olunub");
    await ask.getByRole("button", { name: "Təsdiq istə" }).click();

    const approval = panel.getByRole("region", { name: "Təsdiq" });
    await expect(approval.getByTestId("approval-open")).toContainText("Gözləyir");
    await expect(approval.getByText("Qəbz əlavə olunub")).toBeVisible();
    // The requester can cancel but never approve their own request.
    await expect(approval.getByRole("button", { name: "Sorğunu ləğv et" })).toBeVisible();
    await expect(approval.getByRole("button", { name: "Təsdiqlə" })).toHaveCount(0);
    // Locked content: title is read-only and the status is not editable.
    await expect(panel.getByLabel("Başlıq")).toBeDisabled();
    await expect(panel.getByRole("combobox", { name: "Status" })).toHaveCount(0);
    await expect(panel.getByRole("status").filter({ hasText: "kilidlidir" })).toBeVisible();
  });
});

test.describe("as the approver", () => {
  test.use({ storageState: authFile("alice") });

  test("lists show the badge and the 'pending my approval' filter", async ({ page }) => {
    await page.goto(seed.url);
    await expect(page.getByRole("row").filter({ hasText: "Hesab-faktura 1001" }).getByLabel("Təsdiq gözləyir: 0/1")).toBeVisible();
    await page.getByLabel("Təsdiq", { exact: true }).selectOption({ label: "Mənim təsdiqimi gözləyənlər" });
    await expect(page).toHaveURL(/approval=mine/);
    await expect(page.getByRole("row").filter({ hasText: "Hesab-faktura" })).toHaveCount(2);

    await page.goto(seed.url + "?view=board");
    await expect(page.getByRole("region", { name: "To Do" }).getByLabel("Təsdiq gözləyir: 0/1").first()).toBeVisible();
  });

  test("approving applies the status change and records it", async ({ page }) => {
    await page.goto(seed.url + "?item=1");
    const panel = page.getByRole("dialog", { name: "QA-1" });
    const approval = panel.getByRole("region", { name: "Təsdiq" });
    await approval.getByRole("button", { name: "Təsdiqlə" }).click();
    await expect(panel.getByLabel("Status")).toHaveValue(doneId);
    await expect(approval.getByText("Təsdiqlənib")).toBeVisible();

    await panel.getByRole("tab", { name: "Aktivlik" }).click();
    await expect(panel.getByText("təsdiq tamamlandı, status dəyişdi")).toBeVisible();
    await expect(panel.getByText("təsdiq istədi")).toBeVisible();
  });

  test("rejecting needs a reason", async ({ page }) => {
    await page.goto(seed.url + "?item=2");
    const panel = page.getByRole("dialog", { name: "QA-2" });
    await panel.getByRole("region", { name: "Təsdiq" }).getByRole("button", { name: "Rədd et" }).click();
    const dialog = page.getByRole("dialog", { name: "Rədd etmə səbəbi" });
    await dialog.getByLabel("Səbəb").fill("   ");
    await dialog.getByRole("button", { name: "Rədd et" }).click();
    await expect(dialog.getByRole("alert")).toHaveText("Səbəb məcburidir.");
    await dialog.getByLabel("Səbəb").fill("Məbləğ müqavilə ilə uyğun deyil");
    await dialog.getByRole("button", { name: "Rədd et" }).click();
    await expect(panel.getByText("Rədd edilib")).toBeVisible();
    await expect(panel.getByText("“Məbləğ müqavilə ilə uyğun deyil”")).toBeVisible();
    // Unlocked again.
    await expect(panel.getByLabel("Başlıq")).toBeEnabled();
  });

  test("workflow settings show the policy", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/settings/workflows`);
    await expect(page.getByRole("region", { name: /Default: Təsdiq siyasətləri/ }).getByText("1 təsdiq lazımdır")).toBeVisible();
  });
});
