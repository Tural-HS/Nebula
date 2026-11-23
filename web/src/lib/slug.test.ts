import { describe, expect, it } from "vitest";

import { slugKey } from "./slug";

describe("slugKey", () => {
  it.each([
    ["Müştəri növü", "musteri_novu"],
    ["Ölkə / Şəhər", "olke_seher"],
    ["İcraçı", "icraci"],
    ["Severity", "severity"],
    ["  2nd  priority!! ", "nd_priority"],
    ["Ğ", "g"],
    ["???", ""],
  ])("%s -> %s", (input, want) => {
    expect(slugKey(input)).toBe(want);
  });
  it("produces valid API keys", () => {
    for (const s of ["Ağ yol", "çay", "1-2-3 go", "x".repeat(100)]) {
      const k = slugKey(s);
      expect(k === "" || /^[a-z][a-z0-9_]{0,62}$/.test(k)).toBe(true);
    }
  });
});
