import { expect, request, test as base, type APIRequestContext, type Page } from "@playwright/test";

// Dev-only users and passwords from deploy/keycloak/nebula-realm.json.
export const USERS = {
  alice: { password: "alice-dev-only", name: "Alice Developer" },
  bob: { password: "bob-dev-only", name: "Bob Developer" },
} as const;
export type User = keyof typeof USERS;

const KEYCLOAK = "http://localhost:8081/realms/nebula/protocol/openid-connect/token";
// E2E_API_URL: run against an API other than the compose one (e.g. a branch build on another port).
export const API = process.env.E2E_API_URL ?? "http://localhost:8080/api/v1";
export const MAILPIT = "http://localhost:8025/api/v1";

export const authFile = (user: User) => `e2e/.auth/${user}.json`;

/** Signs in through the real Keycloak form (Authorization Code + PKCE). */
export async function signIn(page: Page, user: User): Promise<void> {
  // Keycloak's field ids: the login page follows the UI language (ADR-0025).
  await page.locator("#username").fill(user);
  await page.locator("#password").fill(USERS[user].password);
  await page.locator("#kc-login").click();
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
  return withRateLimitRetry(await request.newContext({ baseURL: API + "/", extraHTTPHeaders: { Authorization: `Bearer ${access_token}` } }));
}

const RETRIED = ["get", "post", "put", "patch", "delete", "fetch", "head"] as const;

/**
 * Test setup runs many requests as one user in a few seconds, so it can hit
 * the per-user rate limit (ADR-0004). Like the web client (lib/retry), retry
 * 429 after Retry-After: the limiter rejects before any handler runs, so a
 * retry never applies a change twice.
 */
function withRateLimitRetry(ctx: APIRequestContext): APIRequestContext {
  for (const m of RETRIED) {
    const send = ctx[m].bind(ctx) as (...args: unknown[]) => ReturnType<APIRequestContext["get"]>;
    (ctx as unknown as Record<string, unknown>)[m] = async (...args: unknown[]) => {
      for (let attempt = 0; ; attempt++) {
        const res = await send(...args);
        if (res.status() !== 429 || attempt === 5) return res;
        const wait = Math.min(Number(res.headers()["retry-after"] ?? "1") || 1, 5);
        await new Promise((r) => setTimeout(r, wait * 1000));
      }
    };
  }
  return ctx;
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

const KEYCLOAK_ADMIN = "http://localhost:8081";

/**
 * A third person for flows that need someone other than alice and bob (a
 * requester who is neither the approver nor the delegate, ADR-0042). The
 * dev realm only ships alice and bob, so carol is created in the local dev
 * Keycloak through its admin API (bootstrap admin of docker-compose.yml,
 * dev only) with a fresh random password on every run, never stored.
 * Returns an API client signed in as carol (her first request creates her
 * Nebula user).
 */
export async function apiAsCarol(): Promise<{ api: APIRequestContext; email: string }> {
  const email = "carol@example.com";
  const kc = await request.newContext({ baseURL: KEYCLOAK_ADMIN });
  const tok = await kc.post("/realms/master/protocol/openid-connect/token", {
    form: { grant_type: "password", client_id: "admin-cli", username: "admin", password: "admin-dev-only" },
  });
  expect(tok.ok(), "Keycloak admin token (dev only)").toBeTruthy();
  const headers = { Authorization: `Bearer ${((await tok.json()) as { access_token: string }).access_token}` };
  const users = "/admin/realms/nebula/users";
  let found = (await (await kc.get(`${users}?username=carol&exact=true`, { headers })).json()) as { id: string }[];
  if (found.length === 0) {
    const created = await kc.post(users, {
      headers,
      data: { username: "carol", email, emailVerified: true, firstName: "Carol", lastName: "Developer", enabled: true, requiredActions: [] },
    });
    expect(created.status(), await created.text()).toBe(201);
    found = (await (await kc.get(`${users}?username=carol&exact=true`, { headers })).json()) as { id: string }[];
  }
  const password = `carol-${crypto.randomUUID()}`;
  const reset = await kc.put(`${users}/${found[0]?.id ?? ""}/reset-password`, { headers, data: { type: "password", value: password, temporary: false } });
  expect(reset.status(), await reset.text()).toBe(204);
  await kc.dispose();

  const kt = await request.newContext();
  const res = await kt.post(KEYCLOAK, { form: { grant_type: "password", client_id: "nebula-cli", username: "carol", password, scope: "openid" } });
  expect(res.ok(), "token for carol").toBeTruthy();
  const { access_token } = (await res.json()) as { access_token: string };
  await kt.dispose();
  const api = withRateLimitRetry(await request.newContext({ baseURL: API + "/", extraHTTPHeaders: { Authorization: `Bearer ${access_token}` } }));
  expect((await api.get("me")).status()).toBe(200);
  return { api, email };
}
