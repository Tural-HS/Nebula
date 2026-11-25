// Workspace service tokens (ADR-0043): pure helpers for the settings screen.

export const TOKEN_SCOPES = ["items:read", "items:write", "comments:write", "reports:read", "export:read"] as const;
export type TokenScope = (typeof TOKEN_SCOPES)[number];
export type TokenRole = "guest" | "member" | "admin";
export const TOKEN_ROLES: readonly TokenRole[] = ["member", "guest", "admin"];
/** Expiry presets in days; the server allows 1..365. */
export const EXPIRY_PRESETS = [30, 90, 365] as const;

export interface ServiceToken {
  id: string;
  name: string;
  display_name: string;
  role: TokenRole;
  scopes: TokenScope[];
  projects: { id: string; key: string; name: string }[];
  created_by: string | null;
  created_by_name: string | null;
  created_at: string;
  expires_at: string;
  revoked_at: string | null;
  last_used_at: string | null;
  last_used_ip: string | null;
}

export interface CreatedToken extends ServiceToken {
  /** The secret token string: returned once, never again. */
  token: string;
}

export interface TokenInput {
  name: string;
  role: TokenRole;
  scopes: TokenScope[];
  project_ids: string[];
  expires_in_days: number;
}

export type TokenState = "active" | "expired" | "revoked";

export function tokenState(t: Pick<ServiceToken, "revoked_at" | "expires_at">, now: number = Date.now()): TokenState {
  if (t.revoked_at) return "revoked";
  return Date.parse(t.expires_at) <= now ? "expired" : "active";
}

const WRITE: readonly TokenScope[] = ["items:write", "comments:write"];

/** Guests cannot write: switching to guest drops write scopes. */
export function scopesForRole(role: TokenRole, scopes: readonly TokenScope[]): TokenScope[] {
  return TOKEN_SCOPES.filter((s) => scopes.includes(s) && (role !== "guest" || !WRITE.includes(s)));
}

export const isWriteScope = (s: TokenScope) => WRITE.includes(s);

/** Client-side problems of the create form, in the order the form shows them. */
export function tokenProblems(input: TokenInput): ("name" | "scopes")[] {
  const out: ("name" | "scopes")[] = [];
  const name = input.name.trim();
  // Code points, as the server counts runes.
  if (name.length === 0 || Array.from(name).length > 80) out.push("name");
  if (input.scopes.length === 0) out.push("scopes");
  return out;
}

/** A curl line for the README-style hint; the token is a placeholder. */
export function curlExample(origin: string, ws: string, projectKey: string): string {
  return `curl -H "Authorization: Bearer $NEBULA_TOKEN" ${origin}/api/v1/workspaces/${encodeURIComponent(ws)}/projects/${encodeURIComponent(projectKey)}/items`;
}
