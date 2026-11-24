import { describe, expect, it } from "vitest";

import { ageDays, inboxHref, overdueFirst, parseDue, policyExtrasFrom, toggleWatched, watchTokens } from "./approvalInbox";

describe("approval inbox helpers", () => {
  it("counts whole days and never goes negative", () => {
    const now = new Date(2031, 2, 20, 9, 0);
    expect(ageDays(new Date(2031, 2, 20, 8, 0).toISOString(), now)).toBe(0);
    expect(ageDays(new Date(2031, 2, 19, 23, 59).toISOString(), now)).toBe(1);
    expect(ageDays(new Date(2031, 2, 10).toISOString(), now)).toBe(10);
    expect(ageDays(new Date(2031, 2, 21).toISOString(), now)).toBe(0);
  });

  it("links into the drawer's approval panel", () => {
    expect(inboxHref("acme co", { item: { key: "FIN-7", number: 7, project_key: "FIN", title: "x" } }))
      .toBe("/w/acme%20co/p/FIN?item=7&focus=approval");
  });

  it("puts overdue requests first and keeps the order otherwise", () => {
    const list = [{ id: "a", overdue: false }, { id: "b", overdue: true }, { id: "c", overdue: false }, { id: "d", overdue: true }];
    expect(overdueFirst(list).map((e) => e.id)).toEqual(["b", "d", "a", "c"]);
  });

  it("parses the deadline input", () => {
    expect(parseDue("")).toBeNull();
    expect(parseDue(" 3 ")).toBe(3);
    expect(parseDue("30")).toBe(30);
    for (const bad of ["0", "31", "1.5", "abc", "-2"]) expect(parseDue(bad)).toBeNaN();
  });

  it("builds and toggles watched tokens", () => {
    expect(watchTokens(["amount"])).toEqual(["title", "description", "fields.amount"]);
    expect(toggleWatched([], "fields.amount", true)).toEqual(["fields.amount"]);
    expect(toggleWatched(["title", "fields.amount"], "title", false)).toEqual(["fields.amount"]);
    expect(toggleWatched(["title"], "title", true)).toEqual(["title"]);
  });

  it("reads a policy into the form, defaulting to off / everything / no deadline", () => {
    expect(policyExtrasFrom(undefined)).toEqual({ reapprove_on_change: false, reapprove_fields: [], due: "" });
    expect(policyExtrasFrom({ reapprove_on_change: true, reapprove_fields: ["fields.amount"], due_in_working_days: 3 }))
      .toEqual({ reapprove_on_change: true, reapprove_fields: ["fields.amount"], due: "3" });
  });
});
