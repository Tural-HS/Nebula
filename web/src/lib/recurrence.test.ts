import { describe, expect, it } from "vitest";

import { dayWithSuffix } from "../locales/az/recur";
import { describeSchedule, renderTitle } from "./recurrence";

const base = { every: 1, weekdays: [], month_day: null, month: null };

describe("describeSchedule", () => {
  it.each([
    [{ ...base, freq: "daily" as const }, "Hər gün"],
    [{ ...base, freq: "daily" as const, every: 3 }, "Hər 3 gündən bir"],
    [{ ...base, freq: "weekly" as const, weekdays: [4, 1] }, "Hər həftə: bazar ertəsi, cümə axşamı"],
    [{ ...base, freq: "weekly" as const, every: 2, weekdays: [0] }, "Hər 2 həftədən bir: bazar"],
    [{ ...base, freq: "monthly" as const, month_day: 9 }, "Hər ayın 9-u"],
    [{ ...base, freq: "monthly" as const, month_day: -1 }, "Hər ayın son günü"],
    [{ ...base, freq: "monthly" as const, every: 3, month_day: 15 }, "Hər 3 aydan bir, ayın 15-i"],
    [{ ...base, freq: "yearly" as const, month: 3, month_day: 30 }, "Hər il, martın 30-u"],
  ])("%j", (s, want) => {
    expect(describeSchedule(s)).toBe(want);
  });
});

describe("dayWithSuffix", () => {
  it("follows Azerbaijani vowel harmony of the number's reading", () => {
    expect([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20, 26, 30, 31].map(dayWithSuffix)).toEqual([
      "1-i", "2-si", "3-ü", "4-ü", "5-i", "6-sı", "7-si", "8-i", "9-u", "10-u", "15-i", "20-si", "26-sı", "30-u", "31-i",
    ]);
  });
});

describe("renderTitle", () => {
  it("matches the server's placeholders", () => {
    expect(renderTitle("Ofis icarəsi — {{month}} {{year}} ({{date}})", "2026-12-09")).toBe("Ofis icarəsi — dekabr 2026 (09.12.2026)");
    expect(renderTitle("Sabit", "2026-12-09")).toBe("Sabit");
  });
});
