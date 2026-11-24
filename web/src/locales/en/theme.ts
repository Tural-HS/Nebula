// English strings for the colour theme and workspace accent (ADR-0035).
import type { Messages } from "../../i18n/locale";

export const tth = {
  theme: "Colour mode",
  themeHint: "Light, dark or like the system; applies at once.",
  themes: { system: "System", light: "Light", dark: "Dark" },
  accent: "Accent colour",
  accentHint: "Buttons, links and selected items use this colour in this workspace. It changes for everyone.",
  accentSaved: "Accent colour saved",
  accents: { teal: "Teal (default)", ocean: "Ocean", indigo: "Indigo", violet: "Violet", plum: "Plum", forest: "Forest", graphite: "Graphite" },
} satisfies Messages["theme"];
