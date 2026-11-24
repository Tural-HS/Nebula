import { describe, expect, it } from "vitest";

import { ApiError } from "../api/client";
import { exportErrorText, exportFileName, exportQuery } from "./itemExport";
import { tv } from "../i18n-views";

describe("exportQuery", () => {
  it("carries the list's filters and the visible columns, without paging", () => {
    const q = new URLSearchParams(
      exportQuery(
        { q: " Ödəniş ", assignee: "me", due: "overdue", status: ["s2", "s1"], fields: { stage: "c1" }, sort: "-field.amount", approval: "pending", blocked: true },
        ["key", "title", "field.amount"],
        "xlsx",
        "az",
        true,
      ),
    );
    expect(q.get("limit")).toBeNull();
    expect(q.get("cursor")).toBeNull();
    expect(q.get("q")).toBe("Ödəniş");
    expect(q.get("assignee")).toBe("me");
    expect(q.get("due")).toBe("overdue");
    expect(q.get("status")).toBe("s2,s1");
    expect(q.get("field.stage")).toBe("c1");
    expect(q.get("sort")).toBe("-field.amount");
    expect(q.get("approval")).toBe("pending");
    expect(q.get("blocked")).toBe("true");
    expect(q.get("cols")).toBe("key,title,field.amount");
    expect(q.get("format")).toBe("xlsx");
    expect(q.get("lang")).toBe("az");
    expect(q.get("delimiter")).toBeNull(); // CSV only
  });

  it("sets the semicolon delimiter for CSV", () => {
    const q = new URLSearchParams(exportQuery({ q: "", assignee: "" }, ["title"], "csv", "ru", true));
    expect(q.get("delimiter")).toBe("semicolon");
    expect(q.get("format")).toBe("csv");
    expect(new URLSearchParams(exportQuery({ q: "", assignee: "" }, [], "csv", "en", false)).has("cols")).toBe(false);
  });
});

describe("exportFileName", () => {
  const fallback = "ACME.xlsx";
  it("prefers the UTF-8 name", () => {
    expect(exportFileName(`attachment; filename="SEC-2026-11-25.xlsx"; filename*=UTF-8''H%C3%BCquq%20%C5%9F%C3%B6b%C9%99si-2026-11-25.xlsx`, fallback, "xlsx")).toBe(
      "Hüquq şöbəsi-2026-11-25.xlsx",
    );
  });
  it("falls back to the ASCII name, then to the fallback", () => {
    expect(exportFileName(`attachment; filename="SEC-2026-11-25.xlsx"; filename*=UTF-8''%E0%A4%A`, fallback, "xlsx")).toBe("SEC-2026-11-25.xlsx");
    expect(exportFileName(`attachment; filename="SEC.csv"`, fallback, "xlsx")).toBe(fallback);
    expect(exportFileName(null, fallback, "xlsx")).toBe(fallback);
  });
  it("never keeps a path or control characters", () => {
    expect(exportFileName(`attachment; filename*=UTF-8''..%2F..%2Fetc%2Fx.csv`, "x.csv", "csv")).toBe("x.csv");
    expect(exportFileName(`attachment; filename*=UTF-8''a%0Ab%2Fc.csv`, "x.csv", "csv")).toBe("a_b_c.csv");
  });
});

describe("exportErrorText", () => {
  it("says what to do next", () => {
    expect(exportErrorText(new ApiError(422, "export_too_large", "x", "rows"))).toContain("50");
    expect(exportErrorText(new ApiError(422, "export_too_large", "x", "bytes"))).toBe(tv.export.tooLarge);
    expect(exportErrorText(new ApiError(429, "rate_limited", "x"))).toBe(tv.export.rateLimited);
    expect(exportErrorText(new ApiError(429, "export_busy", "x"))).toBe(tv.export.busy);
    expect(exportErrorText(new TypeError("network"))).toBe(tv.export.failed);
  });
});
