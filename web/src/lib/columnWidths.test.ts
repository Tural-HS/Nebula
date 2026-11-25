import { describe, expect, it } from "vitest";

import { clampWidth, parseWidths } from "./columnWidths";

describe("column widths", () => {
  it("clamps to 3..40 rem in quarter steps", () => {
    expect(clampWidth(1)).toBe(3);
    expect(clampWidth(99)).toBe(40);
    expect(clampWidth(7.13)).toBe(7.25);
  });
  it("reads only well-formed stored widths", () => {
    expect(parseWidths('{"title":20,"field.balance":6.5}')).toEqual({ title: 20, "field.balance": 6.5 });
    expect(parseWidths('{"title":"wide","x":null,"y":1e9}')).toEqual({ y: 40 });
    expect(parseWidths("[1,2]")).toEqual({});
    expect(parseWidths("not json")).toEqual({});
    expect(parseWidths(null)).toEqual({});
  });
});
