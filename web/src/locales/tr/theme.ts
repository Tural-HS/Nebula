// Turkish strings for the colour theme and workspace accent (ADR-0035).
import type { Messages } from "../../i18n/locale";

export const tth = {
  theme: "Renk modu",
  themeHint: "Açık, koyu, kâğıt (gözü az yorar), gece mavisi, koyu yeşil, siyah (OLED), alacakaranlık, yüksek kontrast veya sistemdeki gibi; hemen uygulanır.",
  themes: { system: "Sistem", light: "Açık", dark: "Koyu", green: "Yeşil", paper: "Kâğıt", midnight: "Gece mavisi", evergreen: "Koyu yeşil", oled: "Siyah (OLED)", dusk: "Alacakaranlık", contrast: "Yüksek kontrast" },
  accent: "Vurgu rengi",
  accentHint: "Bu çalışma alanında düğmeler, bağlantılar ve seçili öğeler bu renkle görünür. Herkes için değişir.",
  accentSaved: "Vurgu rengi kaydedildi",
  accents: { teal: "Turkuaz (varsayılan)", ocean: "Okyanus", indigo: "Çivit", violet: "Mor", plum: "Erik", forest: "Orman", graphite: "Grafit" },
} satisfies Messages["theme"];
