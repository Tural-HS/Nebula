import { describe, expect, it, vi } from "vitest";

import { api, ApiError, path, setTokenProvider, setUnauthorizedHandler } from "./client";
import { itemsQueryString } from "./hooks";

function fakeFetch(status: number, body?: unknown) {
  return vi.fn<typeof fetch>(() =>
    Promise.resolve(new Response(body === undefined ? null : JSON.stringify(body), { status })),
  );
}

describe("api", () => {
  it("sends the bearer token and JSON body, never cookies", async () => {
    setTokenProvider(() => Promise.resolve("tok-123"));
    const f = fakeFetch(200, { ok: true });
    await api("POST", "/workspaces", { slug: "x" }, f);
    const [url, init] = f.mock.calls[0] ?? [];
    expect(url).toBe("/api/v1/workspaces");
    const headers = init?.headers as Record<string, string>;
    expect(headers.Authorization).toBe("Bearer tok-123");
    expect(headers["Content-Type"]).toBe("application/json");
    expect(init?.credentials).toBe("omit");
    expect(init?.body).toBe('{"slug":"x"}');
  });

  it("maps the API error shape to ApiError", async () => {
    const f = fakeFetch(422, { error: { code: "validation_failed", message: "is required", field: "title" } });
    const err = await api("POST", "/x", {}, f).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 422, code: "validation_failed", field: "title", message: "is required" });
  });

  it("flags conflicts and handles non-JSON errors", async () => {
    const conflict = await api("PATCH", "/x", {}, fakeFetch(409, { error: { code: "version_conflict", message: "m" } })).catch((e: unknown) => e);
    expect((conflict as ApiError).isConflict).toBe(true);
    const gw = await api("GET", "/x", undefined, vi.fn<typeof fetch>(() => Promise.resolve(new Response("<html>", { status: 502 })))).catch((e: unknown) => e);
    expect(gw).toMatchObject({ status: 502, code: "http_502" });
  });

  it("calls the unauthorized handler on 401", async () => {
    const onUnauth = vi.fn();
    setUnauthorizedHandler(onUnauth);
    await api("GET", "/me", undefined, fakeFetch(401, { error: { code: "invalid_token", message: "m" } })).catch(() => undefined);
    expect(onUnauth).toHaveBeenCalledOnce();
  });

  it("returns undefined for 204", async () => {
    await expect(api("DELETE", "/x", undefined, fakeFetch(204))).resolves.toBeUndefined();
  });

  it("rejects relative paths", async () => {
    await expect(api("GET", "me", undefined, fakeFetch(200, {}))).rejects.toThrow();
  });
});

describe("path", () => {
  it("encodes every interpolated segment", () => {
    expect(path`/workspaces/${"a/../b"}/projects/${"K?x=1"}`).toBe("/workspaces/a%2F..%2Fb/projects/K%3Fx%3D1");
  });
});

describe("itemsQueryString", () => {
  it("builds filters and cursor", () => {
    const qs = new URLSearchParams(itemsQueryString({ q: " login ", assignee: "me" }, "abc"));
    expect(Object.fromEntries(qs)).toEqual({ limit: "100", q: "login", assignee: "me", cursor: "abc" });
    expect(itemsQueryString({ q: "", assignee: "" }, null)).toBe("limit=100");
  });
});
