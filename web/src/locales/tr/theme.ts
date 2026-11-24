// Turkish strings for the colour theme and workspace accent (ADR-0035).
import type { Messages } from "../../i18n/locale";

export const tth = {
  theme: "Renk modu",
  themeHint: "Açık, koyu veya sistemdeki gibi; hemen uygulanır.",
  themes: { system: "Sistem", light: "Açık", dark: "Koyu" },
  accent: "Vurgu rengi",
  accentHint: "Bu çalışma alanında düğmeler, bağlantılar ve seçili öğeler bu renkle görünür. Herkes için değişir.",
  accentSaved: "Vurgu rengi kaydedildi",
  accents: { teal: "Turkuaz (varsayılan)", ocean: "Okyanus", indigo: "Çivit", violet: "Mor", plum: "Erik", forest: "Orman", graphite: "Grafit" },
} satisfies Messages["theme"];
