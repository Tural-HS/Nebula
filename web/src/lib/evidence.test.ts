import { describe, expect, it } from "vitest";

import { ApiError } from "../api/client";
import type { Activity } from "../api/types";
import { te as en } from "../locales/en/evidence";
import { te as ru } from "../locales/ru/evidence";
import { te as trk } from "../locales/tr/evidence";
import { defaultRange, evidenceErrorText, exportRecords, fileNameFrom, rangeDays, rangeProblem, verifyCommand } from "./evidence";

describe("defaultRange", () => {
  it("is the previous calendar month", () => {
    expect(defaultRange("2026-11-24")).toEqual({ from: "2026-10-01", to: "2026-10-31" });
    expect(defaultRange("2026-01-05")).toEqual({ from: "2025-12-01", to: "2025-12-31" });
    expect(defaultRange("2028-03-01")).toEqual({ from: "2028-02-01", to: "2028-02-29" }); // leap year
  });
  it("keeps an unparsable date", () => {
    expect(defaultRange("x")).toEqual({ from: "x", to: "x" });
  });
});

describe("rangeProblem", () => {
  it("validates like the server", () => {
    expect(rangeDays("2026-01-01", "2026-01-01")).toBe(1);
    expect(rangeProblem("2026-01-01", "2026-12-31", 366)).toBeNull();
    expect(rangeProblem("2024-01-01", "2024-12-31", 366)).toBeNull(); // 366 days in a leap year
    expect(rangeProblem("2025-01-01", "2026-01-02", 366)).toBe("tooLong");
    expect(rangeProblem("2026-02-01", "2026-01-31", 366)).toBe("order");
    expect(rangeProblem("", "2026-01-31", 366)).toBe("required");
    expect(rangeProblem("2026-1-1", "2026-01-31", 366)).toBe("required");
  });
});

describe("fileNameFrom", () => {
  it("keeps the server's name and strips anything unsafe", () => {
    expect(fileNameFrom('attachment; filename="nebula-evidence-acme-FIN-1.zip"', "x.zip")).toBe("nebula-evidence-acme-FIN-1.zip");
    expect(fileNameFrom('attachment; filename="../../evil name.zip"', "x.zip")).toBe("x.zip"); // starts with a dot
    expect(fileNameFrom('attachment; filename="a b/c.zip"', "x.zip")).toBe("a_b_c.zip");
    expect(fileNameFrom('attachment; filename="report.exe"', "x.zip")).toBe("x.zip");
    expect(fileNameFrom(null, "fallback.zip")).toBe("fallback.zip");
  });
});

describe("evidenceErrorText", () => {
  it("maps API error codes", () => {
    expect(evidenceErrorText(new ApiError(422, "export_too_large", "m", "events"))).toMatch(/hadisə/);
    expect(evidenceErrorText(new ApiError(429, "rate_limited", "m"))).toBeTruthy();
    expect(evidenceErrorText(new ApiError(429, "export_busy", "m"))).toBeTruthy();
    expect(evidenceErrorText(new ApiError(503, "evidence_disabled", "m"))).toMatch(/NEBULA_EVIDENCE_SIGNING_KEY/);
    expect(evidenceErrorText(new ApiError(422, "validation_failed", "unknown project \"X\"", "project_keys"))).toBe('unknown project "X"');
    expect(evidenceErrorText(new ApiError(500, "internal", "m"))).toBeNull();
    expect(evidenceErrorText(new Error("x"))).toBeNull();
  });
});

const act = (id: number, data: Record<string, unknown>): Activity => ({
  id, action: "evidence.exported", entity_type: data.kind === "item" ? "item" : "workspace", entity_id: "e", source: "user",
  automation_rule_id: null, occurred_at: "2026-11-24T10:00:00Z", actor: { id: "u", display_name: "Alice" }, data,
});

describe("exportRecords", () => {
  const keys = new Map([["p1", "FIN"], ["p2", "HR"]]);
  it("describes item and scope exports", () => {
    const rows = exportRecords([
      act(3, { kind: "item", item_key: "FIN-7", counts: { items: 1, events: 12 }, manifest_sha256: "ab".repeat(32) }),
      act(2, { kind: "scope", from: "2026-10-01", to: "2026-10-31", project_ids: ["p1", "p2"], all_projects: false, approvals_only: true, counts: { items: 4, events: 90 } }),
      act(1, { kind: "scope", from: "2026-09-01", to: "2026-09-30", project_ids: ["p1", "gone"], all_projects: true }),
      { ...act(0, {}), action: "item.updated" },
    ], keys);
    expect(rows).toHaveLength(3);
    expect(rows[0]).toMatchObject({ id: 3, who: "Alice", what: "Tapşırıq FIN-7", items: 1, events: 12, approvalsOnly: false });
    expect(rows[1]?.what).toBe("2026-10-01 … 2026-10-31 · FIN, HR");
    expect(rows[1]?.approvalsOnly).toBe(true);
    expect(rows[2]?.what).toContain("bütün layihələr");
  });
  it("does not name projects it cannot resolve", () => {
    const rows = exportRecords([act(1, { kind: "scope", from: "a", to: "b", project_ids: ["hidden"], all_projects: false })], keys);
    expect(rows[0]?.what).toBe("a … b · 1 layihə");
  });
});

describe("translations", () => {
  it("inflect counts", () => {
    expect(en.counts(1, 2)).toBe("1 item, 2 events");
    expect(ru.counts(1, 5)).toBe("1 задача, 5 событий");
    expect(ru.counts(3, 21)).toBe("3 задачи, 21 событие");
    expect(ru.projectCount(2)).toBe("2 проекта");
    expect(trk.counts(5, 1)).toBe("5 görev, 1 olay");
    expect(en.errors.tooLarge("events")).toContain("events");
    expect(ru.errors.tooLarge("nope")).toContain("записей");
  });
  it("builds the verify command", () => {
    expect(verifyCommand("SHA256:abc")).toBe("nebula evidence verify --key SHA256:abc <zip>");
  });
});
