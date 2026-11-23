import { authFile, expect, signIn, test, type User } from "./fixtures";

// Signs each user in once through Keycloak. The saved state holds only the
// IdP's SSO cookies (Nebula keeps tokens in memory), so every test page still
// performs a real, silent PKCE redirect.
for (const user of ["alice", "bob"] as User[]) {
  test(`sign in as ${user}`, async ({ page }) => {
    const health = await page.request.get("/api/v1/version");
    expect(health.ok(), "API not reachable: run `make up` first").toBeTruthy();

    await page.goto("/");
    await expect(page).toHaveURL(/localhost:8081\/realms\/nebula\/protocol\/openid-connect\/auth/);
    expect(page.url()).toContain("code_challenge_method=S256");
    await signIn(page, user);
    await expect(page.getByRole("heading", { level: 1 }).or(page.getByText("Workspace-lər"))).toBeVisible();
    await page.context().storageState({ path: authFile(user) });
  });
}
