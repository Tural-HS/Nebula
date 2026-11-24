import { describe, expect, it } from "vitest";

import type { CalendarDay } from "../api/calendar";
import { ordinal } from "../locales/az/workdays";
import { tw as en } from "../locales/en/workdays";
import { tw as ru } from "../locales/ru/workdays";
import { tw as trk } from "../locales/tr/workdays";
import { describeSchedule } from "./recurrence";
import { daysOfYear, dayLookup, entryProblem, jsWeekday, sortWeekdays, toggleWeekend } from "./workdays";

const cal = {
  weekend_days: [5, 6], // Friday + Saturday
  days: [
    { date: "2027-01-01", kind: "holiday" as const, name: "Yeni il" },
    { date: "2027-01-02", kind: "workday" as const, name: "Köçürmə" }, // a Saturday
  ],
};

describe("dayLookup", () => {
  const l = dayLookup(cal);
  it("uses the workspace weekend, not a hardcoded Sat/Sun", () => {
    expect(l.info("2027-01-08")).toEqual({ kind: "weekend", name: null, working: false }); // Friday
    expect(l.info("2027-01-10")).toEqual({ kind: null, name: null, working: true }); // Sunday
    expect([0, 1, 5, 6].map(l.isWeekend)).toEqual([false, false, true, true]);
  });
  it("marks holidays and transfers with their names", () => {
    expect(l.info("2027-01-01")).toEqual({ kind: "holiday", name: "Yeni il", working: false });
    expect(l.info("2027-01-02")).toEqual({ kind: "workday", name: "Köçürmə", working: true });
  });
  it("defaults to Saturday and Sunday before the calendar loads", () => {
    const d = dayLookup(undefined);
    expect(d.info("2027-01-09").kind).toBe("weekend"); // Saturday
    expect(d.info("2027-01-08").working).toBe(true); // Friday
  });
});

describe("weekday helpers", () => {
  it("reads weekdays on UTC dates", () => {
    expect(jsWeekday("2028-02-29")).toBe(2); // Tuesday, leap day
  });
  it("sorts Monday first and caps the weekend", () => {
    expect(sortWeekdays([0, 6, 1, 6])).toEqual([1, 6, 0]);
    expect(toggleWeekend([6, 0], 5, 3)).toEqual([5, 6, 0]);
    expect(toggleWeekend([5, 6, 0], 1, 3)).toEqual([5, 6, 0]);
    expect(toggleWeekend([6, 0], 6, 3)).toEqual([0]);
  });
  it("picks one year's days, sorted", () => {
    const extra: CalendarDay = { date: "2026-12-31", kind: "holiday", name: "x" };
    expect(daysOfYear([...cal.days, extra].reverse(), 2027).map((d) => d.date)).toEqual(["2027-01-01", "2027-01-02"]);
  });
});

describe("entryProblem", () => {
  const ok = { date: "2027-03-08", kind: "holiday" as const, name: "Qadınlar günü" };
  it("accepts a valid entry", () => {
    expect(entryProblem(ok, [6, 0], 0, 60, false)).toBeNull();
  });
  it("rejects what the server rejects", () => {
    expect(entryProblem({ ...ok, date: "2027-02-30x" }, [6, 0], 0, 60, false)).toBe("date");
    expect(entryProblem({ ...ok, date: "1999-12-31" }, [6, 0], 0, 60, false)).toBe("date");
    expect(entryProblem({ ...ok, name: "  " }, [6, 0], 0, 60, false)).toBe("name");
    expect(entryProblem({ ...ok, name: "ə".repeat(101) }, [6, 0], 0, 60, false)).toBe("name");
    expect(entryProblem({ ...ok, kind: "workday" }, [6, 0], 0, 60, false)).toBe("workdayNeedsWeekend"); // a Monday
    expect(entryProblem({ ...ok, date: "2027-03-06", kind: "workday" }, [6, 0], 0, 60, false)).toBeNull(); // Saturday
    expect(entryProblem(ok, [6, 0], 60, 60, false)).toBe("limit");
    expect(entryProblem(ok, [6, 0], 60, 60, true)).toBeNull(); // replacing an existing date
  });
});

const base = { every: 1, weekdays: [], month_day: null, month: null };

describe("describeSchedule with working days (az)", () => {
  it.each([
    [{ ...base, freq: "monthly" as const, work_day: -1 }, "Hər ayın son iş günü"],
    [{ ...base, freq: "monthly" as const, work_day: 3 }, "Hər ayın 3-cü iş günü"],
    [{ ...base, freq: "monthly" as const, every: 3, work_day: 10 }, "Hər 3 aydan bir, ayın 10-cu iş günü"],
    [{ ...base, freq: "yearly" as const, month: 12, work_day: -1 }, "Hər il, dekabrın son iş günü"],
    [{ ...base, freq: "monthly" as const, month_day: 15, shift: "next" as const }, "Hər ayın 15-i (qeyri-iş günüdürsə, növbəti iş günü)"],
    [{ ...base, freq: "monthly" as const, month_day: -1, shift: "previous" as const }, "Hər ayın son günü (qeyri-iş günüdürsə, əvvəlki iş günü)"],
    [{ ...base, freq: "monthly" as const, month_day: 9, shift: "none" as const }, "Hər ayın 9-u"],
    [{ ...base, freq: "weekly" as const, weekdays: [1], shift: "next" as const }, "Hər həftə: bazar ertəsi (qeyri-iş günüdürsə, növbəti iş günü)"],
  ])("%j", (s, want) => {
    expect(describeSchedule(s)).toBe(want);
  });

  it("Azerbaijani ordinals follow vowel harmony", () => {
    expect([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 16, 19, 20, 21, 23].map(ordinal)).toEqual([
      "1-ci", "2-ci", "3-cü", "4-cü", "5-ci", "6-cı", "7-ci", "8-ci", "9-cu", "10-cu", "11-ci", "13-cü", "16-cı", "19-cu", "20-ci", "21-ci", "23-cü",
    ]);
  });
});

describe("working-day descriptions in en/tr/ru", () => {
  it("English", () => {
    expect(en.describe.monthly(1, -1)).toBe("Monthly on the last working day");
    expect(en.describe.monthly(1, 2)).toBe("Monthly on the 2nd working day");
    expect(en.describe.monthly(2, 11)).toBe("Every 2 months on the 11th working day");
    expect(en.describe.yearly(1, 12, -1)).toBe("Every year on the last working day of December");
    expect(en.describe.shift.next).toContain("next working day");
    expect(en.presetAdded(1)).toBe("1 date added");
    expect(en.presetAdded(3)).toBe("3 dates added");
  });
  it("Turkish", () => {
    expect(trk.describe.monthly(1, -1)).toBe("Her ayın son iş günü");
    expect(trk.describe.monthly(3, 5)).toBe("3 ayda bir, ayın 5. iş günü");
    expect(trk.describe.yearly(1, 3, 1)).toBe("Her yıl, mart ayının 1. iş günü");
  });
  it("Russian with plural forms", () => {
    expect(ru.describe.monthly(1, -1)).toBe("Каждый месяц, в последний рабочий день");
    expect(ru.describe.monthly(2, 3)).toBe("Каждые 2 месяца, в 3-й рабочий день");
    expect(ru.describe.monthly(5, 3)).toBe("Каждые 5 месяцев, в 3-й рабочий день");
    expect(ru.describe.yearly(1, 12, -1)).toBe("Каждый год, в последний рабочий день декабря");
    expect(ru.presetAdded(1)).toBe("Добавлена 1 дата");
    expect(ru.presetAdded(2)).toBe("Добавлены 2 даты");
    expect(ru.presetAdded(5)).toBe("Добавлено 5 дат");
  });
});
