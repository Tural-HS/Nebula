import { describe, expect, it } from "vitest";

import { formatDate, formatDateTime, formatRelative } from "./format";

describe("formatRelative", () => {
  const now = Date.parse("2026-11-23T12:00:00Z");
  it.each([
    ["2026-11-23T11:59:30Z", "indicə"],
    ["2026-11-23T11:29:00Z", "31 dəqiqə əvvəl"],
    ["2026-11-23T09:00:00Z", "3 saat əvvəl"],
    ["2026-11-21T12:00:00Z", "2 gün əvvəl"],
    ["2026-09-23T12:00:00Z", "2 ay əvvəl"],
    ["2024-11-23T12:00:00Z", "2 il əvvəl"],
    ["2026-11-23T14:00:00Z", "2 saat sonra"],
    ["not a date", "—"],
  ])("%s -> %s", (iso, want) => {
    expect(formatRelative(iso, now)).toBe(want);
  });
});

describe("formatDateTime", () => {
  it("uses Azerbaijani month names", () => {
    const d = new Date(2026, 10, 23, 9, 5);
    expect(formatDateTime(d.toISOString())).toBe("23 noy 2026, 09:05");
  });
  it("handles invalid input", () => {
    expect(formatDateTime("x")).toBe("—");
  });
});

describe("formatDate", () => {
  it("formats a calendar date without time zone shifts", () => {
    expect(formatDate("2026-11-20")).toBe("20 noy 2026");
    expect(formatDate("2027-01-01")).toBe("1 yan 2027");
  });
  it("rejects anything else", () => {
    expect(formatDate("2026-13-01")).toBe("—");
    expect(formatDate("20.11.2026")).toBe("—");
  });
});
