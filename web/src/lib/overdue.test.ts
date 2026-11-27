import { describe, expect, it } from "vitest";
import { daysLate, isDueToday, lateDays, lateLevel, localToday } from "./overdue";

describe("overdue bands", () => {
  it("counts whole days across months", () => {
    expect(daysLate("2026-11-25", "2026-11-26")).toBe(1);
    expect(daysLate("2026-10-31", "2026-11-26")).toBe(26);
    expect(daysLate("2026-11-26", "2026-11-26")).toBe(0);
  });
  it("grades 1–15 yellow, 16–30 orange, 31+ red", () => {
    expect(lateLevel(0)).toBe(0);
    expect(lateLevel(1)).toBe(1);
    expect(lateLevel(15)).toBe(1);
    expect(lateLevel(16)).toBe(2);
    expect(lateLevel(30)).toBe(2);
    expect(lateLevel(31)).toBe(3);
  });
  it("formats the local date", () => {
    expect(localToday(new Date(2026, 0, 5))).toBe("2026-01-05");
  });
});

describe("lateDays", () => {
  const it1 = { id: "a", due_date: "2026-11-03", overdue: true };
  it("marks each late day up to today with its band", () => {
    const m = lateDays([it1], "2026-10-26", "2026-12-06", "2026-11-26");
    expect(m.has("2026-11-03")).toBe(false);
    expect(m.get("2026-11-04")?.[0]).toMatchObject({ days: 1, level: 1 });
    expect(m.get("2026-11-18")?.[0]).toMatchObject({ days: 15, level: 1 });
    expect(m.get("2026-11-19")?.[0]).toMatchObject({ days: 16, level: 2 });
    expect(m.get("2026-11-26")?.[0]).toMatchObject({ days: 23, level: 2 });
    expect(m.has("2026-11-27")).toBe(false);
  });
  it("clips to the visible range and skips items that are not overdue", () => {
    const m = lateDays([it1, { id: "b", due_date: "2026-11-01", overdue: false }], "2026-11-20", "2026-11-22", "2026-11-26");
    expect([...m.keys()]).toEqual(["2026-11-20", "2026-11-21", "2026-11-22"]);
    expect(m.get("2026-11-20")).toHaveLength(1);
  });
});

describe("isDueToday", () => {
  const item = (due: string | null, category = "todo", overdue = false) => ({ due_date: due, overdue, status: { category } });
  it("marks open items due today", () => {
    expect(isDueToday(item("2026-11-27"), "2026-11-27")).toBe(true);
  });
  it("skips other days, overdue, done and canceled items", () => {
    expect(isDueToday(item("2026-11-28"), "2026-11-27")).toBe(false);
    expect(isDueToday(item(null), "2026-11-27")).toBe(false);
    expect(isDueToday(item("2026-11-27", "todo", true), "2026-11-27")).toBe(false);
    expect(isDueToday(item("2026-11-27", "done"), "2026-11-27")).toBe(false);
    expect(isDueToday(item("2026-11-27", "canceled"), "2026-11-27")).toBe(false);
  });
});
