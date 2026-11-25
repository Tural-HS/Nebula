import { execFileSync } from "node:child_process";
import { resolve } from "node:path";

import type { APIRequestContext } from "@playwright/test";

import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

test.use({ storageState: authFile("alice") });

/**
 * Moves an item's SLA clock into the past (the dues are computed from the
 * creation time, so a breach needs a backdated clock). Goes through psql in
 * the compose postgres container, like global-teardown.ts; the slug is a
 * psql variable, never spliced into SQL.
 */
function backdate(ws: string, number: number) {
  execFileSync(
    "docker",
    ["compose", "exec", "-T", "postgres", "psql", "-q", "-v", "ON_ERROR_STOP=1", "-U", "nebula", "-d", "nebula",
      "-v", `slug=${ws}`, "-v", `num=${String(number)}`],
    {
      cwd: resolve(import.meta.dirname, "../.."),
      input: `UPDATE item_sla SET started_at = now() - interval '30 days', response_due = now() - interval '20 days',
        resolution_due = now() - interval '1 hour'
        WHERE item_id = (SELECT i.id FROM items i JOIN workspaces w ON w.id = i.workspace_id
          WHERE w.slug = :'slug' AND i.number = :num);\n`,
      stdio: ["pipe", "ignore", "pipe"],
    },
  );
}

async function create(api: APIRequestContext, ws: string, title: string) {
  expect((await api.post(`workspaces/${ws}/projects/QA/items`, { data: { title } })).status()).toBe(201);
}

test.describe("SLA targets", () => {
  let seed: Seed;
  let alice: APIRequestContext;
  test.beforeAll(async () => {
    seed = await seedWorkspace();
    alice = await apiAs("alice");
  });
  test.afterAll(async () => { await alice.dispose(); });

  test("policy, badge and the breached queue", async ({ page }) => {
    await page.goto(seed.url + "/settings");
    const section = page.getByRole("region", { name: "SLA hədəfləri" });
    await section.getByLabel("İlk cavab (iş günü)").fill("1");
    await section.getByLabel("Həll (iş günü)").fill("3");
    await section.getByRole("button", { name: "Saxla" }).click();
    await expect(page.getByText("SLA hədəfləri saxlanıldı")).toBeVisible();

    await create(alice, seed.ws, "Ödəniş sorğusu"); // QA-1, will be breached
    await create(alice, seed.ws, "Yeni müraciət"); // QA-2, on track
    backdate(seed.ws, 1);

    await page.goto(seed.url);
    const row = (key: string) => page.getByRole("row").filter({ hasText: key });
    await expect(row("QA-1").getByRole("img", { name: /^SLA pozulub/ })).toBeVisible();
    await expect(row("QA-2").getByRole("img", { name: /^SLA qaydasındadır/ })).toBeVisible();

    await page.getByRole("group", { name: "Tez filtrlər" }).getByRole("button", { name: /SLA pozulub/ }).click();
    await expect(page).toHaveURL(/sla=breached/);
    await expect(row("QA-1")).toBeVisible();
    await expect(page.getByRole("row").filter({ hasText: "QA-" })).toHaveCount(1);

    // The drawer spells the state and the dues out.
    await row("QA-1").getByText("Ödəniş sorğusu").click();
    await expect(page.getByTestId("sla-panel")).toContainText("SLA pozulub");
    await expect(page.getByTestId("sla-panel")).toContainText("Həll:");
  });
});
