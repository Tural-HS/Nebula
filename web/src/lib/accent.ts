// Workspace accent colours (ADR-0035). An admin picks one of these curated
// scales; inside that workspace it replaces the brand-* tokens through CSS
// variables, so every bg-brand-600 / text-brand-700 / ring-brand-500 follows
// without component changes. Red and amber hues are left out on purpose: they
// already mean "overdue" and "waiting for approval" in the table.
//
// Every scale is checked by accent.test.ts (WCAG 2.x): white on 600 and 600 on
// white ≥ 4.5:1, 700 on 50 ≥ 4.5:1, 300 and 400 on zinc-900 ≥ 4.5:1 (dark
// mode text), 500 ≥ 3:1 on white and zinc-950 (focus ring, non-text).

export const ACCENT_STEPS = ["50", "100", "200", "300", "400", "500", "600", "700", "800", "900", "950"] as const;
export type AccentStep = (typeof ACCENT_STEPS)[number];
export type AccentScale = Record<AccentStep, string>;

export const ACCENTS = ["teal", "ocean", "indigo", "violet", "plum", "forest", "graphite"] as const;
export type Accent = (typeof ACCENTS)[number];
export const DEFAULT_ACCENT: Accent = "teal";

export const isAccent = (v: unknown): v is Accent => typeof v === "string" && (ACCENTS as readonly string[]).includes(v);

export const ACCENT_SCALES: Record<Accent, AccentScale> = {
  // The logo's navy → blue → teal gradient (docs/brand); 50/100/500/600/700/900
  // are the original tokens of index.css, the other steps fill the gaps.
  teal: {
    50: "oklch(0.975 0.018 205)",
    100: "oklch(0.94 0.04 200)",
    200: "oklch(0.885 0.06 203)",
    300: "oklch(0.81 0.08 205)",
    400: "oklch(0.72 0.095 205)",
    500: "oklch(0.62 0.098 205)",
    600: "oklch(0.53 0.088 215)",
    700: "oklch(0.45 0.08 225)",
    800: "oklch(0.38 0.08 245)",
    900: "oklch(0.3 0.08 272)",
    950: "oklch(0.23 0.065 272)",
  },
  ocean: {
    50: "oklch(0.975 0.013 240)",
    100: "oklch(0.94 0.031 240)",
    200: "oklch(0.885 0.062 240)",
    300: "oklch(0.81 0.106 240)",
    400: "oklch(0.72 0.133 240)",
    500: "oklch(0.62 0.140 240)",
    600: "oklch(0.53 0.120 240)",
    700: "oklch(0.45 0.103 240)",
    800: "oklch(0.38 0.086 240)",
    900: "oklch(0.3 0.069 240)",
    950: "oklch(0.23 0.052 240)",
  },
  indigo: {
    50: "oklch(0.975 0.011 275)",
    100: "oklch(0.94 0.029 275)",
    200: "oklch(0.885 0.056 275)",
    300: "oklch(0.81 0.096 275)",
    400: "oklch(0.72 0.146 275)",
    500: "oklch(0.62 0.160 275)",
    600: "oklch(0.53 0.160 275)",
    700: "oklch(0.45 0.152 275)",
    800: "oklch(0.38 0.144 275)",
    900: "oklch(0.3 0.136 275)",
    950: "oklch(0.23 0.128 275)",
  },
  violet: {
    50: "oklch(0.975 0.013 300)",
    100: "oklch(0.94 0.033 300)",
    200: "oklch(0.885 0.066 300)",
    300: "oklch(0.81 0.112 300)",
    400: "oklch(0.72 0.142 300)",
    500: "oklch(0.62 0.150 300)",
    600: "oklch(0.53 0.150 300)",
    700: "oklch(0.45 0.142 300)",
    800: "oklch(0.38 0.135 300)",
    900: "oklch(0.3 0.128 300)",
    950: "oklch(0.23 0.120 300)",
  },
  plum: {
    50: "oklch(0.975 0.015 345)",
    100: "oklch(0.94 0.037 345)",
    200: "oklch(0.885 0.076 345)",
    300: "oklch(0.81 0.120 345)",
    400: "oklch(0.72 0.142 345)",
    500: "oklch(0.62 0.150 345)",
    600: "oklch(0.53 0.150 345)",
    700: "oklch(0.45 0.142 345)",
    800: "oklch(0.38 0.135 345)",
    900: "oklch(0.3 0.128 345)",
    950: "oklch(0.23 0.098 345)",
  },
  forest: {
    50: "oklch(0.975 0.020 150)",
    100: "oklch(0.94 0.042 150)",
    200: "oklch(0.885 0.066 150)",
    300: "oklch(0.81 0.088 150)",
    400: "oklch(0.72 0.104 150)",
    500: "oklch(0.62 0.110 150)",
    600: "oklch(0.53 0.110 150)",
    700: "oklch(0.45 0.104 150)",
    800: "oklch(0.38 0.099 150)",
    900: "oklch(0.3 0.081 150)",
    950: "oklch(0.23 0.064 150)",
  },
  graphite: {
    50: "oklch(0.975 0.006 255)",
    100: "oklch(0.94 0.013 255)",
    200: "oklch(0.885 0.021 255)",
    300: "oklch(0.81 0.028 255)",
    400: "oklch(0.72 0.033 255)",
    500: "oklch(0.62 0.035 255)",
    600: "oklch(0.53 0.035 255)",
    700: "oklch(0.45 0.033 255)",
    800: "oklch(0.38 0.032 255)",
    900: "oklch(0.3 0.030 255)",
    950: "oklch(0.23 0.028 255)",
  },
};

/** CSS custom properties that override the brand tokens for one workspace. */
export function accentVars(accent: Accent): Record<`--color-brand-${AccentStep}`, string> {
  const scale = ACCENT_SCALES[accent];
  const vars = {} as Record<`--color-brand-${AccentStep}`, string>;
  for (const s of ACCENT_STEPS) vars[`--color-brand-${s}`] = scale[s];
  return vars;
}

/** Sets (or, for the default/unknown accent, clears) the overrides on an element. */
export function applyAccent(el: HTMLElement, accent: string | null | undefined): void {
  for (const s of ACCENT_STEPS) el.style.removeProperty(`--color-brand-${s}`);
  if (!isAccent(accent) || accent === DEFAULT_ACCENT) return;
  for (const [k, v] of Object.entries(accentVars(accent))) el.style.setProperty(k, v);
}
