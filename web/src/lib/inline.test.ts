import { describe, expect, it } from "vitest";

import type { FieldDefinition } from "../api/types";
import { draftOf, inlineKind, parseDraft } from "./inline";

const def = (key: string, type: FieldDefinition["type"], archived = false): FieldDefinition => ({ id: key, key, name: key, type, options: {}, archived });
const fields = new Map([
  ["plan", def("plan", "select")],
  ["points", def("points", "number")],
  ["notes", def("notes", "long_text")],
  ["tags", def("tags", "multi_select")],
  ["paid", def("paid", "boolean")],
  ["old", def("old", "text", true)],
]);

describe("inline editing", () => {
  it("edits only the supported columns", () => {
    expect(inlineKind("status", fields)).toBe("status");
    expect(inlineKind("assignee", fields)).toBe("assignee");
    expect(inlineKind("due_date", fields)).toBe("due_date");
    expect(inlineKind("field.plan", fields)).toBe("select");
    expect(inlineKind("field.points", fields)).toBe("number");
    expect(inlineKind("field.paid", fields)).toBe("boolean");
    for (const c of ["key", "title", "type", "created_at", "field.notes", "field.tags", "field.old", "field.missing"]) expect(inlineKind(c, fields)).toBeNull();
  });

  it("parses drafts", () => {
    expect(parseDraft("number", "1,5")).toBe(1.5);
    expect(parseDraft("number", " 42 ")).toBe(42);
    expect(parseDraft("number", "many")).toBeUndefined();
    expect(parseDraft("number", "")).toBeNull();
    expect(parseDraft("text", "  hi ")).toBe("hi");
    expect(draftOf(null)).toBe("");
    expect(draftOf(3)).toBe("3");
  });
});
