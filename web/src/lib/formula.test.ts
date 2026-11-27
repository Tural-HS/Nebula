import { describe, expect, it } from "vitest";

import type { FieldDefinition, FormulaOptions } from "../api/types";
import { fieldSortOptions, isFormulaError, isNumeric, isSortable, parseSort, sortFieldKey, splitAt, valueType } from "./formula";
import { emptyState, formatDecimal, formatFieldValue, readState, stateFromView, sumFields, viewFromState, writeState } from "./views";

const field = (key: string, type: FieldDefinition["type"], extra: Partial<FieldDefinition> = {}): FieldDefinition => ({
  id: key, key, name: key.toUpperCase(), type, options: {}, archived: false, ...extra,
});
const formula = (key: string, result: FormulaOptions["result_type"], expression = "amount - paid", extra: Partial<FieldDefinition> = {}) =>
  field(key, "formula", { options: { formula: { expression, result_type: result, refs: [], scale: 2 } }, ...extra });

const fields = new Map<string, FieldDefinition>([
  ["amount", field("amount", "number")],
  ["remaining", formula("remaining", "number")],
  ["next_call", formula("next_call", "date", "due + 7")],
  ["late", formula("late", "boolean", "@due_date < due")],
  ["due", field("due", "date")],
  ["note", field("note", "text")],
  ["gone", formula("gone", "number", "1", { archived: true })],
]);

describe("formula fields", () => {
  it("behave like their result type", () => {
    expect(valueType(fields.get("remaining"))).toBe("number");
    expect(valueType(fields.get("next_call"))).toBe("date");
    expect(valueType(fields.get("late"))).toBe("boolean");
    expect(valueType(fields.get("note"))).toBe("text");
    expect(valueType(undefined)).toBeUndefined();
    expect(isNumeric(fields.get("remaining"))).toBe(true);
    expect(isNumeric(fields.get("next_call"))).toBe(false);
  });

  it("are totalled when the result is a number", () => {
    expect(sumFields(["title", "field.remaining", "field.next_call", "field.amount", "field.gone"], fields)).toEqual(["remaining", "amount"]);
  });

  it("format values and error markers", () => {
    const members = new Map();
    expect(formatFieldValue(fields.get("remaining"), 1234567.5, members)).toBe("1\u00a0234\u00a0567,5");
    expect(formatFieldValue(fields.get("next_call"), "2026-11-20", members)).toMatch(/2026/);
    expect(formatFieldValue(fields.get("late"), true, members)).not.toBe("true");
    expect(formatFieldValue(fields.get("remaining"), { error: "division_by_zero" }, members)).toBe("#SIFIRA BÖLMƏ");
    expect(formatFieldValue(fields.get("remaining"), { error: "something_new" }, members)).toBe("#something_new");
    expect(formatFieldValue(fields.get("remaining"), null, members)).toBe("");
    expect(isFormulaError({ error: "overflow" })).toBe(true);
    expect(isFormulaError(["a"])).toBe(false);
    expect(isFormulaError(null)).toBe(false);
    expect(isFormulaError(3)).toBe(false);
  });

  it("can be sorted when the result is a number or a date", () => {
    expect(isSortable(fields.get("remaining"))).toBe(true);
    expect(isSortable(fields.get("next_call"))).toBe(true);
    expect(isSortable(fields.get("late"))).toBe(false);
    expect(isSortable(fields.get("note"))).toBe(false);
    expect(isSortable(fields.get("gone"))).toBe(false);
    expect(fieldSortOptions(["title", "field.note", "field.remaining", "field.due", "field.late"], fields).map((o) => o.key)).toEqual(["remaining", "due"]);
  });
});

describe("field sorts in the page state", () => {
  it("parses only valid sorts", () => {
    expect(parseSort("due")).toBe("due");
    expect(parseSort("field.remaining")).toBe("field.remaining");
    expect(parseSort("-field.remaining")).toBe("-field.remaining");
    expect(parseSort("title")).toBe("title"); // ADR-0063
    expect(parseSort("-title")).toBe("-title");
    for (const bad of ["", null, undefined, "field.", "field.Bad", "--field.x", "Title", "--title", "-due", "field.a b"]) expect(parseSort(bad)).toBe("number");
    expect(sortFieldKey("-field.remaining")).toBe("remaining");
    expect(sortFieldKey("due")).toBe("");
  });

  it("round-trips through the URL and saved views", () => {
    const s = { ...emptyState(), sort: "-field.remaining" as const };
    const p = writeState(s);
    expect(p.get("sort")).toBe("-field.remaining");
    expect(readState(p).sort).toBe("-field.remaining");
    const v = viewFromState(s);
    expect(v.config.sort).toBe("-field.remaining");
    expect(stateFromView({ kind: "table", config: { ...v.config } }).sort).toBe("-field.remaining");
    expect(readState(new URLSearchParams("sort=field.EVIL;drop")).sort).toBe("number");
  });
});

describe("number formatting per UI language", () => {
  it.each([
    ["az", "1\u00a0234\u00a0567,5"],
    ["ru", "1\u00a0234\u00a0567,5"],
    ["tr", "1.234.567,5"],
    ["en", "1,234,567.5"],
  ] as const)("%s", (locale, want) => {
    expect(formatDecimal("1234567.5", locale)).toBe(want);
    expect(formatDecimal("-0.25", locale)).toBe(locale === "en" ? "-0.25" : "-0,25");
  });
});

describe("splitAt", () => {
  it("marks the character at a 1-based position", () => {
    expect(splitAt("amout - paid", 1)).toEqual({ before: "", at: "a", after: "mout - paid" });
    expect(splitAt("amount +", 9)).toEqual({ before: "amount +", at: " ", after: "" });
    expect(splitAt("ə + x", 5)).toEqual({ before: "ə + ", at: "x", after: "" }); // code points, like the server
    expect(splitAt("x", 0)).toEqual({ before: "", at: "x", after: "" });
  });
});
