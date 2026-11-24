import { readFile } from "node:fs/promises";

import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Audit evidence export (ADR-0029): alice (owner) exports one item from the
// drawer and a scope from Settings → Audit log; bob (member) sees neither.
// The package is verified in Go tests (nebula evidence verify); here we check
// the download, the export history and the access rules. Synthetic data only.
test.describe.configure({ mode: "serial" });

let seed: Seed;

test.beforeAll(async () => {
  seed = await seedWorkspace("member");
  const bob = await apiAs("bob");
  const res = await bob.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Ödəniş tapşırığı 77" } });
  expect(res.status(), await res.text()).toBe(201);
  const c = await bob.post(`workspaces/${seed.ws}/projects/QA/items/1/comments`, { data: { body: "Qəbz yoxlanıldı" } });
  expect(c.status(), await c.text()).toBe(201);
  await bob.dispose();
});

test.describe("as an admin", () => {
  test.use({ storageState: authFile("alice") });

  test("exports one item from the drawer", async ({ page }) => {
    await page.goto(seed.url + "?item=1");
    const panel = page.getByRole("dialog", { name: "QA-1" });
    const download = page.waitForEvent("download");
    await panel.getByRole("button", { name: "Sübut ixracı" }).click();
    const file = await download;
    expect(file.suggestedFilename()).toBe(`nebula-evidence-${seed.ws}-QA-1.zip`);
    const zip = await readFile(await file.path());
    expect(zip.subarray(0, 2).toString()).toBe("PK");
    for (const name of ["report.html", "manifest.json", "manifest.sig", "data/events.csv"]) {
      expect(zip.includes(Buffer.from(name)), name).toBe(true);
    }
    await expect(page.getByText("Sübut paketi yükləndi")).toBeVisible();
    // The export itself is in the item's history.
    await panel.getByRole("tab", { name: "Aktivlik" }).click();
    await expect(panel.getByText("Sübut paketi ixrac edildi")).toBeVisible();
  });

  test("exports a scope from Settings → Audit log and lists it", async ({ page }) => {
    await page.goto(`/w/${seed.ws}/settings/audit`);
    const box = page.getByRole("region", { name: "Audit sübut paketi" });
    await expect(box.getByTestId("evidence-fingerprint")).toHaveText(/^SHA256:[A-Za-z0-9+/]{43}$/);

    // The form checks the range before asking the server.
    await box.getByLabel("Başlanğıc tarixi").fill("2026-02-01");
    await box.getByLabel("Son tarix").fill("2026-01-01");
    await box.getByRole("button", { name: "Sübut paketini ixrac et" }).click();
    await expect(box.getByRole("alert")).toHaveText("Son tarix başlanğıcdan əvvəl ola bilməz.");

    const today = new Date().toISOString().slice(0, 10);
    const weekAgo = new Date(Date.now() - 7 * 86_400_000).toISOString().slice(0, 10);
    await box.getByLabel("Başlanğıc tarixi").fill(weekAgo);
    await box.getByLabel("Son tarix").fill(today);
    await box.getByRole("checkbox", { name: /QA/ }).check();
    const download = page.waitForEvent("download");
    await box.getByRole("button", { name: "Sübut paketini ixrac et" }).click();
    const file = await download;
    expect(file.suggestedFilename()).toBe(`nebula-evidence-${seed.ws}-${weekAgo}_${today}.zip`);

    const history = box.getByRole("table", { name: "İxrac tarixçəsi" });
    await expect(history.getByRole("row")).toHaveCount(3); // header + scope + item export
    await expect(history.getByRole("row").nth(1)).toContainText(`${weekAgo} … ${today} · QA`);
    await expect(history.getByRole("row").nth(1)).toContainText("1 tapşırıq");
    await expect(history.getByRole("row").nth(2)).toContainText("Tapşırıq QA-1");
  });
});

test.describe("as a member", () => {
  test.use({ storageState: authFile("bob") });

  test("has no export action and the API refuses", async ({ page }) => {
    await page.goto(seed.url + "?item=1");
    const panel = page.getByRole("dialog", { name: "QA-1" });
    await expect(panel.getByText("Ödəniş tapşırığı 77")).toBeVisible();
    await expect(panel.getByRole("button", { name: "Sübut ixracı" })).toHaveCount(0);

    const bob = await apiAs("bob");
    const res = await bob.post(`workspaces/${seed.ws}/projects/QA/items/1/evidence`, { data: {} });
    expect(res.status()).toBe(403);
    const scope = await bob.post(`workspaces/${seed.ws}/evidence/exports`, { data: { from: "2026-01-01", to: "2026-01-31" } });
    expect(scope.status()).toBe(403);
    await bob.dispose();
  });
});
