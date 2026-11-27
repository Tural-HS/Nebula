import { describe, expect, it } from "vitest";
import { summaryValue } from "./summary";

describe("summaryValue (ADR-0064)", () => {
  const sums = { monthly: { sum: "67425", count: 60 }, balance: { sum: "11524", count: 12 } };
  it("sums one field or subtracts another", () => {
    expect(summaryValue({ label: "x", field: "monthly" }, sums)).toBe(67425);
    expect(summaryValue({ label: "x", field: "monthly", minus: "balance" }, sums)).toBe(55901);
  });
  it("treats a missing sum as zero and no data as unknown", () => {
    expect(summaryValue({ label: "x", field: "nope" }, sums)).toBe(0);
    expect(summaryValue({ label: "x", field: "monthly" }, undefined)).toBeNull();
  });
});
