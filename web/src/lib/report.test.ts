import { describe, expect, it } from "vitest";
import type { Item } from "../api/types";
import { categoryCounts, overdueBreakdown, reportNumbers } from "./report";

const item = (key: string, due: string, owed: number): Item => ({
  id: key, key, number: 1, project_id: "p", type: { id: "t", key: "task", name: "Task" },
  status: { id: "s", name: "Gecikir", category: "in_progress" }, title: key, description: null,
  assignee_id: null, reporter_id: null, fields: { balance: owed }, due_date: due, overdue: true, version: 1, created_at: "", updated_at: "",
});

describe("report panel (ADR-0066)", () => {
  it("computes paid, percent and forecast", () => {
    const r = reportNumbers("67425", "10902", 3361);
    expect(r).toMatchObject({ plan: 67425, remaining: 10902, paid: 56523, percent: 84, forecast: 64064, forecastPercent: 95 });
    expect(reportNumbers(undefined, undefined, 0).percent).toBe(0);
  });
  it("bins overdue items and finds the longest late", () => {
    const b = overdueBreakdown([item("A", "2026-10-09", 1109), item("B", "2026-11-01", 1302), item("C", "2026-11-14", 950)], "balance", "2026-11-27");
    expect(b.bands).toEqual({ 1: 1, 2: 1, 3: 1 });
    expect(b.owed).toBe(3361);
    expect(b.worst?.item.key).toBe("A");
    expect(b.worst?.days).toBe(49);
  });
  it("sums counts per category", () => {
    const cat = new Map([["a", "done" as const], ["b", "todo" as const], ["c", "in_progress" as const]]);
    expect(categoryCounts({ a: 46, b: 10, c: 4, x: 9 }, cat)).toEqual({ todo: 10, in_progress: 4, done: 46, canceled: 0 });
  });
});
