// Turkish strings for workspace-wide search (ADR-0024).
import type { Messages } from "../../i18n/locale";

export const ts = {
  open: "Ara",
  placeholder: "Görev, yorum veya anahtar (QA-12) arayın…",
  dialogTitle: "Çalışma alanında ara",
  inputLabel: "Arama sorgusu",
  shortcut: (mac: boolean) => (mac ? "⌘K" : "Ctrl K"),
  hint: "En az 2 harf yazın. ö, ü, ş, ç, ğ, ı (ve ə) harfleri olmadan da bulunur: “odeme” → “ödeme”.",
  items: "Görevler",
  comments: "Yorumlar",
  count: (shown: number, total: number) => (total > shown ? `${String(shown)} / ${String(total)}` : String(total)),
  empty: "Hiçbir şey bulunamadı.",
  fuzzy: "Tam eşleşme yok. Benzer başlıklar:",
  keyMatch: "Anahtar",
  commentBy: (author: string) => `${author} yorumu`,
  someone: "Bir kullanıcı",
  more: "Daha fazla sonuç",
  rateLimited: "Çok sık arama yapılıyor, biraz bekleyin…",
  tooLong: "Sorgu çok uzun (en fazla 200 karakter).",
  navHint: "↑↓ seçin, Enter açın, Esc kapatın",
} satisfies Messages["search"];
