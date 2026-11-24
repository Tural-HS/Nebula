import { apiAs, authFile, expect, test } from "./fixtures";

// Workspace kinds (ADR-0033): a finance team's workspace starts with the
// "Maliyyə" project, the payment request form and the four-eyes policy.
// Synthetic data only.

test.use({ storageState: authFile("alice") });

test("a finance workspace is created with its project, form and approval policy", async ({ page }) => {
  const slug = `e2e-${process.env.E2E_RUN ?? "manual"}-fin${Math.random().toString(36).slice(2, 6)}`; // cleaned by global-teardown
  await page.goto("/");
  await page.getByRole("button", { name: "Yeni workspace" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Ad", { exact: true }).fill("Maliyyə komandası");
  await dialog.getByLabel("Qısa ad (URL)").fill(slug);
  await expect(dialog.getByLabel("Başlanğıc şablonu")).toHaveValue("az");

  const kinds = dialog.getByRole("group", { name: "Komanda növü" });
  await expect(kinds.getByRole("radio", { name: /^Ümumi/ })).toBeChecked(); // the default keeps today's behaviour
  await kinds.getByRole("radio", { name: /^Maliyyə/ }).check();
  await expect(dialog.getByText(/başqa admin və ya sahibin təsdiqini/)).toBeVisible();
  await expect(dialog.getByText(/rəsmi iş təqvimi/)).toBeVisible();
  await dialog.getByRole("button", { name: "Yarat" }).click();
  await expect(page).toHaveURL(new RegExp(`/w/${slug}`));

  await page.getByRole("link", { name: /Maliyyə/ }).first().click();
  await expect(page).toHaveURL(new RegExp(`/w/${slug}/p/FIN`));

  const formSlug = `${slug}-payment-request`;
  await page.goto(`/w/${slug}/p/FIN/forms`);
  const row = page.getByRole("listitem").filter({ hasText: "Ödəniş tələbi" });
  await expect(row).toContainText(`/f/${formSlug}`);
  await expect(row).toContainText("Aktiv");

  await page.goto(`/f/${formSlug}`);
  await expect(page.getByRole("heading", { name: "Ödəniş tələbi" })).toBeVisible();
  await expect(page.getByLabel(/^Kontragent/)).toBeVisible();
  await expect(page.getByLabel(/^Məbləğ/)).toBeVisible();

  const alice = await apiAs("alice");
  const views = (await (await alice.get(`workspaces/${slug}/projects/FIN/views`)).json()) as { views: { name: string }[] };
  const wfs = (await (await alice.get(`workspaces/${slug}/workflows`)).json()) as {
    workflows: { statuses: { id: string; name: string }[]; approval_policies: { status_id: string }[] }[];
  };
  await alice.dispose();
  expect(views.views.map((v) => v.name).sort()).toEqual(["Gecikənlər", "Təsdiq gözləyir"]);
  expect(wfs.workflows).toHaveLength(1);
  const [wf] = wfs.workflows;
  expect(wf.statuses.map((s) => s.name)).toEqual(["Hazırlanır", "Yoxlamada", "Təsdiq gözləyir", "Ödənildi", "Ləğv edildi"]);
  expect(wf.approval_policies.map((p) => p.status_id)).toEqual([wf.statuses.find((s) => s.name === "Ödənildi")?.id]);
});
