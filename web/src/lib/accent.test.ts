// @vitest-environment jsdom
import { describe, expect, it } from "vitest";

import { ACCENT_SCALES, ACCENT_STEPS, ACCENTS, applyAccent, isAccent } from "./accent";
import { contrast, cssToRgb, inGamut, oklchToRgb, parseOklch } from "./color";

// Vitest turns CSS imports (even ?raw) into empty strings, so read the file;
// the app's tsconfig has no Node types, hence the narrow local type.
type NodeFs = { readFileSync(path: string, encoding: "utf8"): string };
const nodeProcess = (globalThis as unknown as { process: { cwd(): string; getBuiltinModule(id: "node:fs"): NodeFs } }).process;
const css = nodeProcess.getBuiltinModule("node:fs").readFileSync(`${nodeProcess.cwd()}/src/index.css`, "utf8"); // vitest runs in web/

const WHITE = cssToRgb("#ffffff");
const ZINC_900 = cssToRgb("#18181b"); // dark-mode surfaces (Tailwind zinc-900)
const ZINC_950 = cssToRgb("#09090b"); // dark-mode page background

describe("contrast", () => {
  it("matches known WCAG ratios", () => {
    expect(contrast(WHITE, cssToRgb("#000000"))).toBeCloseTo(21, 1);
    expect(contrast(WHITE, cssToRgb("#767676"))).toBeCloseTo(4.54, 1);
  });
  it("converts OKLCH like the CSS spec", () => {
    const [r, g, b] = oklchToRgb(1, 0, 0);
    expect(r).toBeCloseTo(1, 3);
    expect(g).toBeCloseTo(1, 3);
    expect(b).toBeCloseTo(1, 3);
  });
});

describe.each(ACCENTS)("accent %s", (name) => {
  const s = ACCENT_SCALES[name];
  const c = (step: keyof typeof s) => cssToRgb(s[step]);

  it("is in the sRGB gamut at every step", () => {
    for (const step of ACCENT_STEPS) expect(inGamut(oklchToRgb(...parseOklch(s[step]))), `${name}-${step}`).toBe(true);
  });
  it("gets darker step by step", () => {
    const ls = ACCENT_STEPS.map((step) => parseOklch(s[step])[0]);
    for (let i = 1; i < ls.length; i++) expect(ls[i]).toBeLessThan(ls[i - 1] ?? Infinity);
  });
  it("meets WCAG AA for text in light mode", () => {
    expect(contrast(WHITE, c("600"))).toBeGreaterThanOrEqual(4.5); // primary button label, links
    expect(contrast(c("700"), c("50"))).toBeGreaterThanOrEqual(4.5); // selected chip text
    expect(contrast(c("700"), WHITE)).toBeGreaterThanOrEqual(4.5);
  });
  it("meets WCAG AA for text in dark mode", () => {
    expect(contrast(c("300"), ZINC_900)).toBeGreaterThanOrEqual(4.5);
    expect(contrast(c("400"), ZINC_900)).toBeGreaterThanOrEqual(4.5);
    expect(contrast(c("100"), ZINC_950)).toBeGreaterThanOrEqual(4.5);
  });
  it("keeps the focus ring visible (3:1, non-text)", () => {
    expect(contrast(c("500"), WHITE)).toBeGreaterThanOrEqual(3);
    expect(contrast(c("500"), ZINC_950)).toBeGreaterThanOrEqual(3);
  });
});

describe("teal accent", () => {
  it("is exactly the brand scale of index.css", () => {
    for (const step of ACCENT_STEPS) {
      expect(css).toContain(`--color-brand-${step}: ${ACCENT_SCALES.teal[step]};`);
    }
  });
});

describe("applyAccent", () => {
  it("sets every brand variable and clears them for the default", () => {
    const el = document.createElement("div");
    applyAccent(el, "violet");
    for (const step of ACCENT_STEPS) expect(el.style.getPropertyValue(`--color-brand-${step}`)).toBe(ACCENT_SCALES.violet[step]);
    applyAccent(el, "teal");
    for (const step of ACCENT_STEPS) expect(el.style.getPropertyValue(`--color-brand-${step}`)).toBe("");
  });
  it("ignores unknown values", () => {
    const el = document.createElement("div");
    applyAccent(el, "ocean");
    applyAccent(el, "url(javascript:alert(1))");
    expect(el.style.getPropertyValue("--color-brand-600")).toBe("");
    expect(isAccent("red")).toBe(false);
    expect(isAccent(null)).toBe(false);
  });
});
