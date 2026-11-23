import { describe, expect, it } from "vitest";

import { activeFilterCount, displayColumns, keyboardInset } from "./responsive";

describe("displayColumns", () => {
  const cols = ["key", "title", "type", "status", "field.cost"];

  it("keeps the order on wide screens", () => {
    expect(displayColumns(cols, false)).toEqual({ columns: cols, keyInTitle: false });
  });

  it("pins the title first and folds the key into it on narrow screens", () => {
    expect(displayColumns(cols, true)).toEqual({ columns: ["title", "type", "status", "field.cost"], keyInTitle: true });
  });

  it("moves the title first even when it was not second", () => {
    expect(displayColumns(["status", "key", "assignee", "title"], true)).toEqual({ columns: ["title", "status", "assignee"], keyInTitle: true });
  });

  it("does not invent a key the user hid", () => {
    expect(displayColumns(["title", "status"], true)).toEqual({ columns: ["title", "status"], keyInTitle: false });
  });

  it("leaves a table without a title alone", () => {
    expect(displayColumns(["key", "status"], true)).toEqual({ columns: ["key", "status"], keyInTitle: false });
  });

  it("does not mutate its input", () => {
    const input = ["key", "title"];
    displayColumns(input, true);
    expect(input).toEqual(["key", "title"]);
  });
});

describe("activeFilterCount", () => {
  it("counts each filter the sheet holds", () => {
    expect(activeFilterCount({ assignee: "", due: "", status: [], fields: {} })).toBe(0);
    expect(activeFilterCount({ assignee: "me", due: "overdue", status: ["a", "b"], fields: { cost: "x" } })).toBe(5);
  });
});

describe("keyboardInset", () => {
  it("is zero without the Visual Viewport API", () => {
    expect(keyboardInset(800, null)).toBe(0);
    expect(keyboardInset(800, undefined)).toBe(0);
  });

  it("is the covered height when the keyboard is open", () => {
    expect(keyboardInset(844, { height: 500, offsetTop: 0 })).toBe(344);
  });

  it("accounts for the visual viewport being scrolled", () => {
    expect(keyboardInset(844, { height: 500, offsetTop: 100 })).toBe(244);
  });

  it("ignores small differences such as a collapsing URL bar", () => {
    expect(keyboardInset(844, { height: 790, offsetTop: 0 })).toBe(0);
    expect(keyboardInset(844, { height: 900, offsetTop: 0 })).toBe(0);
  });
});
