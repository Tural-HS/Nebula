import { describe, expect, it } from "vitest";

import type { FieldDefinition } from "../api/types";
import { choicesValue, containsValue, filterKind, operatorText, parseFilter, pickedChoices, rangeValue } from "./columnFilter";

const base: FieldDefinition = { id: "f", key: "x", name: "X", type: "text", options: {}, archived: false } as unknown as FieldDefinition;
const def = (patch: Partial<FieldDefinition>): FieldDefinition => ({ ...base, ...patch });
const words = { contains: (v: string) => `içində "${v}"`, from: (v: string) => `≥ ${v}`, to: (v: string) => `≤ ${v}` };

describe("filterKind", () => {
  it("maps field types to filter editors", () => {
    expect(filterKind(def({ type: "text" }))).toBe("contains");
    expect(filterKind(def({ type: "email" }))).toBe("contains");
    expect(filterKind(def({ type: "number" }))).toBe("number");
    expect(filterKind(def({ type: "date" }))).toBe("date");
    expect(filterKind(def({ type: "select" }))).toBe("choice");
    expect(filterKind(def({ type: "boolean" }))).toBe("boolean");
    expect(filterKind(def({ type: "money" }))).toBe("money");
    expect(filterKind(def({ type: "user" }))).toBeNull();
    expect(filterKind(def({ type: "text", archived: true }))).toBeNull();
    expect(filterKind(undefined)).toBeNull();
  });
});

describe("values", () => {
  it("builds and parses operator values", () => {
    expect(containsValue("  055 ")).toBe("~055");
    expect(containsValue("   ")).toBe("");
    expect(containsValue("x".repeat(150))).toHaveLength(101);
    expect(rangeValue("500", "")).toBe("500..");
    expect(rangeValue("", "2026-12-31")).toBe("..2026-12-31");
    expect(rangeValue(" ", "")).toBe("");
    expect(parseFilter("~055")).toEqual({ contains: "055", from: "", to: "", exact: "" });
    expect(parseFilter("10..20")).toEqual({ contains: "", from: "10", to: "20", exact: "" });
    expect(parseFilter("..20")).toEqual({ contains: "", from: "", to: "20", exact: "" });
    expect(parseFilter("abc")).toEqual({ contains: "", from: "", to: "", exact: "abc" });
    expect(parseFilter(undefined).exact).toBe("");
  });

  it("describes operator values for chips", () => {
    const fmt = (b: string) => `<${b}>`;
    expect(operatorText("~055", fmt, words)).toBe('içində "055"');
    expect(operatorText("10..20", fmt, words)).toBe("<10> – <20>");
    expect(operatorText("10..", fmt, words)).toBe("≥ <10>");
    expect(operatorText("..20", fmt, words)).toBe("≤ <20>");
    expect(operatorText("choice-id", fmt, words)).toBeNull();
  });
});

describe("empty, set and several choices", () => {
  it("builds and reads choice lists", () => {
    expect(choicesValue([])).toBe("");
    expect(choicesValue(["a"])).toBe("a");
    expect(choicesValue(["a", "b"])).toBe("in:a,b");
    expect(pickedChoices("in:a,b")).toEqual(["a", "b"]);
    expect(pickedChoices("a")).toEqual(["a"]);
    expect(pickedChoices("is:empty")).toEqual([]);
    expect(pickedChoices(undefined)).toEqual([]);
  });
  it("describes empty and set, and keeps them out of the range inputs", () => {
    const w = { ...words, empty: "boşdur", set: "doludur" };
    expect(operatorText("is:empty", (b) => b, w)).toBe("boşdur");
    expect(operatorText("is:set", (b) => b, w)).toBe("doludur");
    expect(parseFilter("is:empty")).toEqual({ contains: "", from: "", to: "", exact: "" });
    expect(parseFilter("in:a,b").exact).toBe("");
  });
});
