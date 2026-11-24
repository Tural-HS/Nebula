import { describe, expect, it } from "vitest";

import { detectLocale, isLocale } from "./locale";

describe("detectLocale", () => {
  it("prefers the stored choice", () => {
    expect(detectLocale("ru", ["en-US"])).toBe("ru");
  });
  it("ignores an unknown stored value", () => {
    expect(detectLocale("de", ["tr-TR"])).toBe("tr");
  });
  it("takes the first supported browser language", () => {
    expect(detectLocale(null, ["de-DE", "ru-RU", "en"])).toBe("ru");
    expect(detectLocale(null, ["EN-gb"])).toBe("en");
  });
  it("falls back to Azerbaijani", () => {
    expect(detectLocale(null, ["de", "fr"])).toBe("az");
    expect(detectLocale(null, [])).toBe("az");
  });
});

describe("isLocale", () => {
  it("accepts only the four UI languages", () => {
    expect(["az", "en", "tr", "ru", "de", "", null].map(isLocale)).toEqual([true, true, true, true, false, false, false]);
  });
});
