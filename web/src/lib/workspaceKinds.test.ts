import { describe, expect, it } from "vitest";

import { createWorkspaceBody, isWorkspaceKind, kindNotes, WORKSPACE_KINDS } from "./workspaceKinds";

describe("workspace kinds", () => {
  it("lists general first (the default)", () => {
    expect(WORKSPACE_KINDS[0]).toBe("general");
    expect(WORKSPACE_KINDS).toEqual(["general", "finance", "it", "hr"]);
  });

  it("recognises only known kinds", () => {
    expect(isWorkspaceKind("finance")).toBe(true);
    expect(isWorkspaceKind("sales")).toBe(false);
    expect(isWorkspaceKind(undefined)).toBe(false);
  });

  it("leaves general out of the request body", () => {
    expect(createWorkspaceBody({ slug: " Fin-Team ", name: " Maliyyə ", template: "az", kind: "general" })).toEqual({
      slug: "fin-team", name: "Maliyyə", template: "az",
    });
    expect(createWorkspaceBody({ slug: "fin", name: "F", template: "ru", kind: "finance" })).toEqual({
      slug: "fin", name: "F", template: "ru", kind: "finance",
    });
  });

  it("notes the calendar only for az finance/hr and the approval only for finance", () => {
    expect(kindNotes("finance", "az")).toEqual({ calendar: true, approval: true });
    expect(kindNotes("finance", "en")).toEqual({ calendar: false, approval: true });
    expect(kindNotes("hr", "az")).toEqual({ calendar: true, approval: false });
    expect(kindNotes("it", "az")).toEqual({ calendar: false, approval: false });
    expect(kindNotes("general", "az")).toEqual({ calendar: false, approval: false });
  });
});
