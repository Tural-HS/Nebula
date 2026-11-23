import { expect, request, test as base, type APIRequestContext, type Page } from "@playwright/test";

// Dev-only users and passwords from deploy/keycloak/nebula-realm.json.
export const USERS = {
  alice: { password: "alice-dev-only", name: "Alice Developer" },
  bob: { password: "bob-dev-only", name: "Bob Developer" },
} as const;
export type User = keyof typeof USERS;

const KEYCLOAK = "http://localhost:8081/realms/nebula/protocol/openid-connect/token";
// E2E_API_URL: run against an API other than the compose one (e.g. a branch build on another port).
const API = process.env.E2E_API_URL ?? "http://localhost:8080/api/v1";
export const MAILPIT = "http://localhost:8025/api/v1";

export const authFile = (user: User) => `e2e/.auth/${user}.json`;

/** Signs in through the real Keycloak form (Authorization Code + PKCE). */
export async function signIn(page: Page, user: User): Promise<void> {
  await page.getByRole("textbox", { name: /username or email/i }).fill(user);
  await page.getByRole("textbox", { name: /^password$/i }).fill(USERS[user].password);
  await page.getByRole("button", { name: /sign in/i }).click();
}

/** API client for test data setup, authenticated with a dev token. */
export async function apiAs(user: User): Promise<APIRequestContext> {
  const kc = await request.newContext();
  const res = await kc.post(KEYCLOAK, {
    form: { grant_type: "password", client_id: "nebula-cli", username: user, password: USERS[user].password, scope: "openid" },
  });
  expect(res.ok(), `token for ${user}`).toBeTruthy();
  const { access_token } = (await res.json()) as { access_token: string };
  await kc.dispose();
  return request.newContext({ baseURL: API + "/", extraHTTPHeaders: { Authorization: `Bearer ${access_token}` } });
}

export interface Seed {
  ws: string;
  project: string;
  bobId: string;
  aliceId: string;
  url: string; // project page
}

/**
 * Creates an isolated workspace (alice = owner, bob = given role) with one
 * project, so specs never depend on or disturb each other's data.
 */
export async function seedWorkspace(bobRole: "guest" | "member" = "member"): Promise<Seed> {
  const alice = await apiAs("alice");
  const bob = await apiAs("bob");
  const ws = `e2e-${process.env.E2E_RUN ?? "manual"}-${Math.random().toString(36).slice(2, 8)}`; // see global-teardown.ts
  const bobId = ((await (await bob.get("me")).json()) as { id: string }).id; // first sign-in creates bob
  const aliceId = ((await (await alice.get("me")).json()) as { id: string }).id;
  expect((await alice.post("workspaces", { data: { slug: ws, name: "E2E " + ws } })).status()).toBe(201);
  expect((await alice.post(`workspaces/${ws}/projects`, { data: { key: "QA", name: "Keyfiyyət" } })).status()).toBe(201);
  expect((await alice.post(`workspaces/${ws}/members`, { data: { email: "bob@example.com", role: bobRole } })).status()).toBe(201);
  await alice.dispose();
  await bob.dispose();
  return { ws, project: "QA", bobId, aliceId, url: `/w/${ws}/p/QA` };
}

export const test = base;
export { expect };
