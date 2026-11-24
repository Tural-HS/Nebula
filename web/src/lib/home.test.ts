import { describe, expect, it } from "vitest";

import type { HomeItem } from "../api/home";
import { counterHref, firstName, formatToday, homeListHref, isCalm, itemHref, orderCards, rowDate } from "./home";

// Synthetic data only (ADR-0037).
const item = (patch: Partial<HomeItem> = {}): HomeItem => ({
  reason: "overdue",
  workspace: { slug: "acme mmc", name: "Acme", accent: null },
  project: { key: "FIN", name: "Maliyyə" },
  key: "FIN-7",
  number: 7,
  title: "Akt-üzləşmə",
  status: { id: "s1", name: "İcrada", category: "in_progress" },
  due_date: "2031-03-10",
  assigned: true,
  approval: null,
  ...patch,
});

describe("link targets", () => {
  it("opens the single workspace's inbox for approvals, the home list otherwise", () => {
    expect(counterHref("approval", [{ slug: "acme" }])).toBe("/w/acme/approvals");
    expect(counterHref("approval", [{ slug: "a" }, { slug: "b" }])).toBe("/?show=approval");
    expect(counterHref("approval", [])).toBe("/?show=approval");
    for (const show of ["overdue", "week", "assigned"] as const) {
      expect(counterHref(show, [{ slug: "acme" }])).toBe(homeListHref(show));
    }
  });

  it("opens the item drawer in its project, at the approval panel for approvals", () => {
    expect(itemHref(item())).toBe("/w/acme%20mmc/p/FIN?item=7");
    expect(itemHref(item({ reason: "approval" }))).toBe("/w/acme%20mmc/p/FIN?item=7&focus=approval");
  });
});

describe("ordering", () => {
  it("puts workspaces that need me first, then by name", () => {
    const ws = (name: string, overdue = 0, approvals = 0) => ({ name, overdue, approvals });
    expect(orderCards([ws("Zəfər"), ws("Beta"), ws("Xəzinə", 0, 2), ws("Acme", 1)]).map((w) => w.name)).toEqual(["Acme", "Xəzinə", "Beta", "Zəfər"]);
  });
});

describe("row dates", () => {
  it("shows the approval's age and deadline for approval rows", () => {
    const a = { requested_at: "2031-03-01T08:00:00Z", due_on: "2031-03-05", overdue: true };
    expect(rowDate(item({ reason: "approval", approval: a }))).toEqual({ kind: "approval", since: a.requested_at, dueOn: a.due_on, overdue: true });
  });
  it("shows the due date, red only when overdue", () => {
    expect(rowDate(item())).toEqual({ kind: "due", date: "2031-03-10", overdue: true });
    expect(rowDate(item({ reason: "week" }))).toEqual({ kind: "due", date: "2031-03-10", overdue: false });
    expect(rowDate(item({ reason: "assigned", due_date: null }))).toBeNull();
  });
});

describe("greeting", () => {
  it("uses the first name", () => {
    expect(firstName("Alice Developer")).toBe("Alice");
    expect(firstName("  Ləman   Həsənli ")).toBe("Ləman");
    expect(firstName("carol@example.com")).toBe("carol");
    expect(firstName(undefined)).toBe("");
  });
  it("writes today's date in the UI language", () => {
    const d = new Date(2026, 10, 24); // a Tuesday
    expect(formatToday(d, "az")).toBe("24 noyabr 2026, çərşənbə axşamı");
    expect(formatToday(d, "en")).toMatch(/Tuesday.*24 November 2026/);
    expect(formatToday(d, "ru")).toMatch(/вторник/);
  });
});

it("is calm only when nothing asks for action", () => {
  expect(isCalm({ overdue: 0, approvals: 0, week: 0, assigned: 12 })).toBe(true);
  expect(isCalm({ overdue: 0, approvals: 1, week: 0, assigned: 0 })).toBe(false);
});
