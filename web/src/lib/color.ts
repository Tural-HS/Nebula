// Colour maths for the accent palette checks (ADR-0035): OKLCH → sRGB and
// the WCAG 2.x contrast ratio. Pure functions, used by tests and the picker.

export type RGB = [number, number, number]; // 0..1, gamma-encoded sRGB

/** Parses "oklch(L C H)" (L 0..1). */
export function parseOklch(css: string): [number, number, number] {
  const m = /^oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*\)$/.exec(css);
  if (!m) throw new Error(`not an oklch() colour: ${css}`);
  return [Number(m[1]), Number(m[2]), Number(m[3])];
}

const toGamma = (x: number) => (x <= 0.0031308 ? 12.92 * x : 1.055 * Math.pow(x, 1 / 2.4) - 0.055);
const toLinear = (x: number) => (x <= 0.04045 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4));

/** OKLCH → sRGB (Björn Ottosson's matrices); values may fall outside 0..1 when out of gamut. */
export function oklchToRgb(l: number, c: number, h: number): RGB {
  const hr = (h * Math.PI) / 180;
  const a = c * Math.cos(hr);
  const b = c * Math.sin(hr);
  const l_ = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m_ = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s_ = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const r = 4.0767416621 * l_ - 3.3077115913 * m_ + 0.2309699292 * s_;
  const g = -1.2684380046 * l_ + 2.6097574011 * m_ - 0.3413193965 * s_;
  const bl = -0.0041960863 * l_ - 0.7034186147 * m_ + 1.707614701 * s_;
  return [toGamma(r), toGamma(g), toGamma(bl)];
}

export function inGamut([r, g, b]: RGB, eps = 0.002): boolean {
  return [r, g, b].every((v) => v >= -eps && v <= 1 + eps);
}

export function hexToRgb(hex: string): RGB {
  const n = Number.parseInt(hex.replace("#", ""), 16);
  return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
}

const clamp = (v: number) => Math.min(1, Math.max(0, v));

export function luminance([r, g, b]: RGB): number {
  return 0.2126 * toLinear(clamp(r)) + 0.7152 * toLinear(clamp(g)) + 0.0722 * toLinear(clamp(b));
}

export function contrast(a: RGB, b: RGB): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

export const cssToRgb = (css: string): RGB => (css.startsWith("#") ? hexToRgb(css) : oklchToRgb(...parseOklch(css)));
