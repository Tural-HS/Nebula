import { describe, expect, it } from "vitest";

import type { ReportAssignee } from "../api/reports";
import {
  canDrillDown, durationParts, itemsHref, niceTicks, presetRange, rangeDays, rangeError, readFilters, reportsQuery, sortWorkload,
  todayIn, writeFilters, type ReportFilters,
} from "./reports";

const base: ReportFilters = { preset: "this_month", from: "2026-11-01", to: "2026-11-25", projects: [], assignee: "", type: "" };

describe("todayIn", () => {
  it("uses the workspace's zone, not the browser's", () => {
    const now = new Date("2026-11-25T21:30:00Z");
    expect(todayIn("Asia/Baku", now)).toBe("2026-11-26"); // UTC+4
    expect(todayIn("UTC", now)).toBe("2026-11-25");
    expect(todayIn("Pacific/Pago_Pago", new Date("2026-11-25T05:00:00Z"))).toBe("2026-11-24");
  });
  it("falls back to UTC for an unknown zone", () => {
    expect(todayIn("Mars/Olympus", new Date("2026-11-25T21:30:00Z"))).toBe("2026-11-25");
  });
});

describe("presetRange", () => {
  it.each([
    ["this_month", "2026-11-25", "2026-11-01", "2026-11-25"],
    ["last_month", "2026-11-25", "2026-10-01", "2026-10-31"],
    ["last_month", "2026-01-15", "2025-12-01", "2025-12-31"],
    ["last_month", "2026-03-31", "2026-02-01", "2026-02-28"],
    ["last_month", "2028-03-10", "2028-02-01", "2028-02-29"],
    ["last_30", "2026-11-25", "2026-10-27", "2026-11-25"],
    ["last_90", "2026-11-25", "2026-08-28", "2026-11-25"],
    ["last_30", "2026-03-01", "2026-01-31", "2026-03-01"],
    ["this_quarter", "2026-11-25", "2026-10-01", "2026-11-25"],
    ["this_quarter", "2026-01-01", "2026-01-01", "2026-01-01"],
    ["this_quarter", "2026-06-30", "2026-04-01", "2026-06-30"],
  ] as const)("%s on %s is %s..%s", (preset, today, from, to) => {
    expect(presetRange(preset, today)).toEqual({ from, to });
    expect(rangeDays(from, to)).toBeGreaterThan(0);
  });
  it("last 30 and last 90 days are exactly that many days", () => {
    const r30 = presetRange("last_30", "2026-11-25");
    const r90 = presetRange("last_90", "2026-11-25");
    expect(rangeDays(r30.from, r30.to)).toBe(30);
    expect(rangeDays(r90.from, r90.to)).toBe(90);
  });
});

describe("rangeError", () => {
  it("accepts up to two years", () => {
    expect(rangeError("2025-01-01", "2026-12-31")).toBeNull(); // 730 days
    expect(rangeError("2024-01-01", "2025-12-31")).toBeNull(); // 731 (leap year)
    expect(rangeError("2024-01-01", "2026-01-01")).toBe("tooLong");
  });
  it("rejects bad and reversed dates", () => {
    expect(rangeError("", "2026-01-01")).toBe("invalid");
    expect(rangeError("2026-02-30", "2026-03-01")).toBe("invalid");
    expect(rangeError("2026-02-02", "2026-02-01")).toBe("order");
    expect(rangeError("2026-02-01", "2026-02-01")).toBeNull();
  });
});

describe("URL state", () => {
  it("defaults to this month in the workspace's today", () => {
    expect(readFilters(new URLSearchParams(), "2026-11-25")).toEqual(base);
    expect(writeFilters(base).toString()).toBe("");
  });
  it("round-trips a custom range and filters", () => {
    const f: ReportFilters = { preset: "custom", from: "2026-01-05", to: "2026-01-25", projects: ["FIN", "AP"], assignee: "none", type: "invoice" };
    const p = writeFilters(f);
    expect(p.toString()).toBe("range=custom&from=2026-01-05&to=2026-01-25&projects=AP%2CFIN&assignee=none&type=invoice");
    expect(readFilters(p, "2026-11-25")).toEqual({ ...f, projects: ["AP", "FIN"] });
  });
  it("a preset ignores stale from/to; a bad custom range falls back", () => {
    expect(readFilters(new URLSearchParams("range=last_month&from=2020-01-01&to=2020-01-02"), "2026-11-25")).toMatchObject({ from: "2026-10-01", to: "2026-10-31" });
    expect(readFilters(new URLSearchParams("range=custom&from=2026-02-01&to=2026-01-01"), "2026-11-25")).toMatchObject({ preset: "this_month" });
  });
  it("drops malformed values", () => {
    const f = readFilters(new URLSearchParams("range=year&projects=fin,OK,OK,x-y&assignee=bob&type=Bad"), "2026-11-25");
    expect(f).toMatchObject({ preset: "this_month", projects: ["OK"], assignee: "", type: "" });
  });
  it("the API query always carries explicit dates", () => {
    expect(reportsQuery(base)).toBe("from=2026-11-01&to=2026-11-25");
    expect(reportsQuery({ ...base, projects: ["B", "A"], assignee: "me", type: "bug" })).toBe("from=2026-11-01&to=2026-11-25&projects=A%2CB&assignee=me&type=bug");
  });
});

describe("drill-down links", () => {
  it("use the item list's own parameters", () => {
    expect(itemsHref("acme", "ACME", base, { due: "overdue" })).toBe("/w/acme/p/ACME?due=overdue");
    expect(itemsHref("acme", "ACME", { ...base, assignee: "me" }, { status: ["b", "a", "a"] })).toBe("/w/acme/p/ACME?assignee=me&status=a%2Cb");
    expect(itemsHref("acme", "ACME", base, { assignee: "none", approval: "pending" })).toBe("/w/acme/p/ACME?assignee=none&approval=pending");
    expect(itemsHref("a b", "ACME", base, {})).toBe("/w/a%20b/p/ACME");
  });
  it("are off with a type filter (the list cannot filter by type)", () => {
    expect(canDrillDown(base)).toBe(true);
    expect(canDrillDown({ ...base, type: "bug" })).toBe(false);
  });
});

describe("workload sort", () => {
  const row = (name: string | null, open: number, overdue: number, completed: number): ReportAssignee => ({
    assignee_id: name ? `id-${name}` : null, name, open, overdue, completed, projects: [],
  });
  const rows = [row("Zaur", 1, 0, 5), row(null, 3, 2, 0), row("Aysel", 4, 1, 2)];
  const names = (rs: ReportAssignee[]) => rs.map((r) => r.name ?? "—");
  it("numbers descend, names ascend with unassigned last", () => {
    expect(names(sortWorkload(rows, "open", false, "—", "az"))).toEqual(["Aysel", "—", "Zaur"]);
    expect(names(sortWorkload(rows, "completed", false, "—", "az"))).toEqual(["Zaur", "Aysel", "—"]);
    expect(names(sortWorkload(rows, "name", true, "—", "az"))).toEqual(["Aysel", "Zaur", "—"]);
    expect(names(sortWorkload(rows, "overdue", true, "—", "az"))).toEqual(["Zaur", "Aysel", "—"]);
  });
});

describe("helpers", () => {
  it("durations pick a readable unit", () => {
    expect(durationParts(90)).toEqual({ unit: "minutes", value: 2 });
    expect(durationParts(6 * 3600)).toEqual({ unit: "hours", value: 6 });
    expect(durationParts(20.4 * 3600)).toEqual({ unit: "hours", value: 20.4 });
    expect(durationParts(3 * 86400 + 3600)).toEqual({ unit: "days", value: 3 });
  });
  it("ticks are round and reach the maximum", () => {
    expect(niceTicks(0)).toEqual([0]);
    expect(niceTicks(1)).toEqual([0, 1]);
    expect(niceTicks(10)).toEqual([0, 5, 10]);
    expect(niceTicks(37)).toEqual([0, 10, 20, 30, 40]);
  });
});
