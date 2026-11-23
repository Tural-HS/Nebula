import { afterEach, describe, expect, it, vi } from "vitest";

import { BulkFailedError, bulkErrors, postBulk } from "./bulk";

afterEach(() => { vi.unstubAllGlobals(); });

describe("bulk client", () => {
  it("reads per-item errors defensively", () => {
    expect(bulkErrors({ error: { items: [{ number: 3, code: "version_conflict", message: "stale" }, { number: "x" }, null] } })).toEqual([
      { number: 3, code: "version_conflict", message: "stale" },
    ]);
    expect(bulkErrors(null)).toEqual([]);
    expect(bulkErrors({ error: { items: "nope" } })).toEqual([]);
  });

  it("sends number and version only, and surfaces a rolled-back change", async () => {
    const fetchMock = vi.fn<(url: string, init: RequestInit) => Promise<Response>>(() =>
      Promise.resolve(
        new Response(JSON.stringify({ error: { code: "bulk_failed", message: "bulk change failed", items: [{ number: 2, code: "version_conflict", message: "stale" }] } }), {
          status: 409,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
    const err = await postBulk("acme", "QA", [{ number: 2, version: 4 }], { status_id: "s1" }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(BulkFailedError);
    expect((err as BulkFailedError).isConflict).toBe(true);
    expect((err as BulkFailedError).items).toEqual([{ number: 2, code: "version_conflict", message: "stale" }]);
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(url).toBe("/api/v1/workspaces/acme/projects/QA/items/bulk");
    expect(JSON.parse(init?.body as string)).toEqual({ items: [{ number: 2, version: 4 }], status_id: "s1" });
  });
});
