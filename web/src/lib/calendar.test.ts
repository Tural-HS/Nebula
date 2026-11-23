import { describe, expect, it } from "vitest";

import type { Item } from "../api/types";
import { addDays, byDay, monthDays, monthGrid, overdueOn, overflow, parseMonth, shiftMonth, todayIn, weekday } from "./calendar";

const item = (n: number, due: string | null, category: Item["status"]["category"] = "todo") =>
  ({ number: n, due_date: due, status: { id: "s", name: "S", category } }) as Item;

describe("calendar", () => {
  it("parses months strictly", () => {
    expect(parseMonth("2026-12")).toBe("2026-12");
    for (const bad of ["2026-13", "2026-1", "26-01", "2026-00", "1800-01", "", null, "2026-12-01"]) expect(parseMonth(bad)).toBeNull();
  });

  it("shifts months across years", () => {
    expect(shiftMonth("2026-12", 1)).toBe("2027-01");
    expect(shiftMonth("2026-01", -1)).toBe("2025-12");
    expect(shiftMonth("2026-03", -14)).toBe("2025-01");
  });

  it("builds Monday-first grids of whole weeks", () => {
    // December 2026 starts on a Tuesday and ends on a Thursday.
    const g = monthGrid("2026-12");
    expect(g.first).toBe("2026-11-30");
    expect(g.last).toBe("2027-01-03");
    expect(g.days).toHaveLength(35);
    expect(weekday(g.first)).toBe(0);
    // February 2027 starts on a Monday: no leading days.
    expect(monthGrid("2027-02").first).toBe("2027-02-01");
    // March 2026 needs six weeks.
    expect(monthGrid("2026-03").days).toHaveLength(42);
    expect(monthDays("2028-02")).toHaveLength(29);
  });

  it("does day arithmetic without DST drift", () => {
    expect(addDays("2026-03-29", 1)).toBe("2026-03-30");
    expect(addDays("2026-10-25", 1)).toBe("2026-10-26");
    expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
  });

  it("computes today in the workspace zone", () => {
    const now = new Date("2026-11-24T21:30:00Z");
    expect(todayIn("Asia/Baku", now)).toBe("2026-11-25");
    expect(todayIn("America/New_York", now)).toBe("2026-11-24");
    expect(todayIn("Not/AZone", now)).toBe("2026-11-24");
  });

  it("groups by day and caps a day with +N more", () => {
    const m = byDay([item(1, "2026-12-01"), item(2, null), item(3, "2026-12-01"), item(4, "2026-12-02")]);
    expect(m.get("2026-12-01")?.map((i) => i.number)).toEqual([1, 3]);
    expect(m.size).toBe(2);
    expect(overflow([1, 2, 3], 3)).toEqual({ shown: [1, 2, 3], more: 0 });
    expect(overflow([1, 2, 3, 4], 3)).toEqual({ shown: [1, 2], more: 2 });
  });

  it("predicts overdue like the server", () => {
    expect(overdueOn(item(1, null), "2026-11-01", "2026-11-24")).toBe(true);
    expect(overdueOn(item(1, null), "2026-11-24", "2026-11-24")).toBe(false);
    expect(overdueOn(item(1, null, "done"), "2026-11-01", "2026-11-24")).toBe(false);
    expect(overdueOn(item(1, null), null, "2026-11-24")).toBe(false);
  });
});
