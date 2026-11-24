import { describe, expect, it } from "vitest";

import { ApiError } from "../api/client";
import { archivedDate, confirmMatches, deleteOutcome, projectActions, projectGone, workspaceActions } from "./lifecycle";

describe("confirmMatches", () => {
  it("needs the exact key, spaces trimmed", () => {
    expect(confirmMatches(" ACME ", "ACME")).toBe(true);
    expect(confirmMatches("acme", "ACME")).toBe(false);
    expect(confirmMatches("", "ACME")).toBe(false);
    expect(confirmMatches("ACM", "ACME")).toBe(false);
  });
  it("compares slugs case-insensitively when asked", () => {
    expect(confirmMatches("Acme-Ops", "acme-ops", true)).toBe(true);
    expect(confirmMatches("acme", "acme-ops", true)).toBe(false);
    expect(confirmMatches("  ", "", true)).toBe(false);
  });
});

describe("actions by role", () => {
  it("workspace: owner archives and deletes, admins restore", () => {
    expect(workspaceActions("owner", false)).toEqual({ archive: true, restore: false, delete: false });
    expect(workspaceActions("owner", true)).toEqual({ archive: false, restore: true, delete: true });
    expect(workspaceActions("admin", false)).toEqual({ archive: false, restore: false, delete: false });
    expect(workspaceActions("admin", true)).toEqual({ archive: false, restore: true, delete: false });
    expect(workspaceActions("member", true)).toEqual({ archive: false, restore: false, delete: false });
    expect(workspaceActions(undefined, true)).toEqual({ archive: false, restore: false, delete: false });
  });
  it("project: admins archive and restore, only the owner deletes", () => {
    expect(projectActions("admin", false)).toEqual({ archive: true, restore: false, delete: false });
    expect(projectActions("admin", true)).toEqual({ archive: false, restore: true, delete: false });
    expect(projectActions("owner", true)).toEqual({ archive: false, restore: true, delete: true });
    expect(projectActions("member", false)).toEqual({ archive: false, restore: false, delete: false });
    expect(projectActions("guest", true)).toEqual({ archive: false, restore: false, delete: false });
  });
});

describe("projectGone", () => {
  it("reads the 410 codes", () => {
    expect(projectGone(new ApiError(410, "project_archived", "x"))).toBe("archived");
    expect(projectGone(new ApiError(410, "project_deleting", "x"))).toBe("deleting");
    expect(projectGone(new ApiError(404, "not_found", "x"))).toBe("missing");
    expect(projectGone(new Error("network"))).toBe("missing");
    expect(projectGone(undefined)).toBe("missing");
  });
});

describe("deleteOutcome and archivedDate", () => {
  it("tells finished from continuing deletions", () => {
    expect(deleteOutcome({ status: "deleted" })).toBe("deleted");
    expect(deleteOutcome({ status: "deleting" })).toBe("deleting");
    expect(deleteOutcome(undefined)).toBe("deleting");
  });
  it("takes the local date of a timestamp", () => {
    expect(archivedDate("2026-11-25T12:00:00")).toBe("2026-11-25"); // no zone: local time
    expect(archivedDate(new Date(2026, 0, 2, 0, 30).toISOString())).toBe("2026-01-02");
    expect(archivedDate(null)).toBeNull();
    expect(archivedDate("garbage")).toBeNull();
  });
});
