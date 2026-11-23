import { describe, expect, it } from "vitest";

import type { FieldDefinition } from "../api/types";
import { conditionText, conditionValue, fieldValue, recipients } from "./rules";

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
    expect(recipients("assignee, ops@kredagro.az ,")).toEqual(["assignee", "ops@kredagro.az"]);
  });
});
