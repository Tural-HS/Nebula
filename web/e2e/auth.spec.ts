import { apiAs, authFile, expect, seedWorkspace, signIn, test } from "./fixtures";

test.describe("authentication", () => {
  test("deep link survives the Keycloak round trip (fresh browser)", async ({ browser }) => {
    const seed = await seedWorkspace();
    const alice = await apiAs("alice");
    await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Lövhədə görünür" } });
    await alice.dispose();
    const ctx = await browser.newContext(); // no SSO cookies: a real login
    const page = await ctx.newPage();
    await page.goto(seed.url + "?view=board");
    await expect(page).toHaveURL(/localhost:8081/);
    await signIn(page, "alice");
    // The single-use auth code is exchanged exactly once and the user lands
    // exactly where they started, including the query string.
    await expect(page).toHaveURL(new RegExp(`${seed.url}\\?view=board$`));
    await expect(page.getByRole("region", { name: "To Do" })).toBeVisible();
    await ctx.close();
  });

  test("a reload keeps the session through silent SSO, tokens are not in web storage", async ({ browser }) => {
    const ctx = await browser.newContext({ storageState: authFile("alice") });
    const page = await ctx.newPage();
    await page.goto("/");
    await expect(page.getByText("Workspace-lər").first()).toBeVisible();
    await page.reload();
    await expect(page.getByText("Workspace-lər").first()).toBeVisible();
    const stored = await page.evaluate(() => JSON.stringify([Object.entries(localStorage), Object.entries(sessionStorage)]));
    expect(stored).not.toMatch(/eyJ[A-Za-z0-9_-]{10,}\./); // no JWT persisted anywhere
    await ctx.close();
  });

  test("sign out ends the session", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await page.goto("/");
    await signIn(page, "alice");
    await page.getByRole("button", { name: "Çıxış" }).click();
    // RP-initiated logout ends the IdP session and comes back to Nebula, which
    // then has to show the login form again (no silent re-login).
    await expect(page.getByRole("textbox", { name: /username or email/i })).toBeVisible();
    await page.goto("/");
    await expect(page.getByRole("textbox", { name: /username or email/i })).toBeVisible();
    await ctx.close();
  });
});
