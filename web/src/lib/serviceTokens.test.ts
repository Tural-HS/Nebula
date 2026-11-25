import { describe, expect, it } from "vitest";

import { curlExample, scopesForRole, tokenProblems, tokenState } from "./serviceTokens";

describe("service tokens", () => {
  const now = Date.parse("2026-11-25T12:00:00Z");
  it("derives the state", () => {
    expect(tokenState({ revoked_at: null, expires_at: "2026-12-25T00:00:00Z" }, now)).toBe("active");
    expect(tokenState({ revoked_at: null, expires_at: "2026-11-25T12:00:00Z" }, now)).toBe("expired");
    expect(tokenState({ revoked_at: "2026-11-01T00:00:00Z", expires_at: "2026-10-01T00:00:00Z" }, now)).toBe("revoked");
  });
  it("drops write scopes for guests and keeps a stable order", () => {
    expect(scopesForRole("guest", ["items:write", "reports:read", "items:read", "comments:write"])).toEqual(["items:read", "reports:read"]);
    expect(scopesForRole("member", ["export:read", "items:write"])).toEqual(["items:write", "export:read"]);
  });
  it("validates the form", () => {
    const base = { name: "1C", role: "member" as const, scopes: ["items:read" as const], project_ids: [], expires_in_days: 90 };
    expect(tokenProblems(base)).toEqual([]);
    expect(tokenProblems({ ...base, name: "  ", scopes: [] })).toEqual(["name", "scopes"]);
    expect(tokenProblems({ ...base, name: "x".repeat(81) })).toEqual(["name"]);
  });
  it("builds a curl example without a secret", () => {
    expect(curlExample("https://nebula.example.com", "acme", "FIN")).toBe(
      'curl -H "Authorization: Bearer $NEBULA_TOKEN" https://nebula.example.com/api/v1/workspaces/acme/projects/FIN/items',
    );
  });
});
