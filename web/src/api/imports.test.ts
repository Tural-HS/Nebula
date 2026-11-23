import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError, apiUpload, setTokenProvider } from "./client";
import { runImport } from "./imports";

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("import API", () => {
  it("uploads multipart with the bearer token and lets the browser set the boundary", async () => {
    setTokenProvider(() => Promise.resolve("tok"));
    const f = vi.fn<typeof fetch>(() => Promise.resolve(json(200, { ok: true })));
    const fd = new FormData();
    fd.append("file", new File(["a"], "a.csv"));
    await apiUpload("/x", fd, [], f);
    const [url, init] = f.mock.calls[0] ?? [];
    expect(url).toBe("/api/v1/x");
    const headers = init?.headers as Record<string, string>;
    expect(headers.Authorization).toBe("Bearer tok");
    expect(headers["Content-Type"]).toBeUndefined();
    expect(init?.credentials).toBe("omit");
    expect(init?.body).toBe(fd);
  });

  it("returns the row report of a rejected import but throws for a rejected mapping", async () => {
    const report = { dry_run: false, row_count: 2, valid: 1, created: 0, error_count: 1, errors: [], truncated: false, file_name: "a.csv" };
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(json(422, report))));
    const file = new File(["x"], "a.csv");
    await expect(runImport("acme", "ACME", file, "", { columns: [], default_type: "task" }, false)).resolves.toMatchObject({ error_count: 1 });

    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(json(422, { error: { code: "validation_failed", message: "bad", field: "mapping.columns" } }))));
    const err = await runImport("acme", "ACME", file, "", { columns: [], default_type: "task" }, true).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 422, field: "mapping.columns" });

    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(json(413, { error: { code: "too_many_rows", message: "x" } }))));
    await expect(runImport("acme", "ACME", file, "", { columns: [], default_type: "" }, true)).rejects.toMatchObject({ code: "too_many_rows" });
  });
});
