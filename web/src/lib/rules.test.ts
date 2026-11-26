import { describe, expect, it } from "vitest";

import type { FieldDefinition } from "../api/types";
import { conditionText, conditionValue, copyableFields, copyMatchFields, copySummary, fieldValue, recipients, toggleKey } from "./rules";

const def = (type: FieldDefinition["type"]): FieldDefinition => ({ id: "f", key: "k", name: "K", type, options: {}, archived: false });

describe("conditionValue", () => {
  it("omits values for emptiness checks", () => {
    expect(conditionValue("is_empty", "x")).toBeUndefined();
  });
  it("splits lists for in / not_in", () => {
    expect(conditionValue("in", " bug, task ,, ")).toEqual(["bug", "task"]);
  });
  it("trims scalars", () => {
    expect(conditionValue("eq", "  bug ")).toBe("bug");
  });
  it("round-trips through conditionText", () => {
    expect(conditionText(conditionValue("not_in", "a, b"))).toBe("a, b");
    expect(conditionText(undefined)).toBe("");
    expect(conditionText(5)).toBe("5");
  });
});

describe("fieldValue", () => {
  it("types values by field type", () => {
    expect(fieldValue(def("number"), " 42 ")).toBe(42);
    expect(fieldValue(def("number"), "abc")).toBe("abc"); // the API reports the error
    expect(fieldValue(def("boolean"), "true")).toBe(true);
    expect(fieldValue(def("boolean"), "no")).toBe(false);
    expect(fieldValue(def("multi_select"), "c1, c2")).toEqual(["c1", "c2"]);
    expect(fieldValue(def("text"), " hi ")).toBe("hi");
    expect(fieldValue(undefined, "x")).toBe("x");
  });
});

describe("recipients", () => {
  it("splits and trims", () => {
    expect(recipients("assignee, ops@example.com ,")).toEqual(["assignee", "ops@example.com"]);
  });
});

describe("copy_fields helpers", () => {
  const f = (key: string, type: FieldDefinition["type"], archived = false): FieldDefinition => ({ id: key, key, name: key.toUpperCase(), type, options: {}, archived });
  const all = [f("nv", "text"), f("bal", "number"), f("sum", "formula"), f("old", "text", true), f("due", "date")];

  it("matches on live text and number fields", () => {
    expect(copyMatchFields(all).map((x) => x.key)).toEqual(["nv", "bal"]);
  });
  it("copies live non-formula fields other than the match field", () => {
    expect(copyableFields(all, "nv").map((x) => x.key)).toEqual(["bal", "due"]);
  });
  it("toggles keys in selection order", () => {
    expect(toggleKey(["a"], "b")).toEqual(["a", "b"]);
    expect(toggleKey(["a", "b"], "a")).toEqual(["b"]);
  });
  it("summarizes the target and fields", () => {
    expect(copySummary("MAH", ["bal", "x"], (k) => (k === "bal" ? "Balance" : k))).toBe("MAH: Balance, x");
  });
});
