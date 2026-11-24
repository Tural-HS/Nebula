import { describe, expect, it } from "vitest";

import { formatBytes, formatDate, formatDateTime, formatRelative } from "./format";

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

describe("other UI languages (ADR-0025)", () => {
  const now = Date.parse("2026-11-23T12:00:00Z");
  it("formats calendar dates with Intl", () => {
    expect(formatDate("2026-11-20", "en")).toBe("20 Nov 2026");
    expect(formatDate("2026-11-20", "tr")).toBe("20 Kas 2026");
    expect(formatDate("2026-11-20", "ru")).toBe("20 нояб. 2026 г.");
  });
  it("formats relative times with Intl", () => {
    expect(formatRelative("2026-11-21T12:00:00Z", now, "en")).toBe("2 days ago");
    expect(formatRelative("2026-11-23T11:59:30Z", now, "en")).toBe("now");
    expect(formatRelative("2026-11-23T07:00:00Z", now, "ru")).toBe("5 часов назад");
    expect(formatRelative("2026-11-23T14:00:00Z", now, "tr")).toBe("2 saat sonra");
  });
  it("uses the language's decimal separator for sizes", () => {
    expect(formatBytes(1.5 * 1024 * 1024, "en")).toBe("1.5 MB");
    expect(formatBytes(1.5 * 1024 * 1024, "ru")).toBe("1,5 MB");
  });
});
