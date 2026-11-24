// Turkish strings for recurring items (ADR-0012).
import type { Messages } from "../../i18n/locale";

const MONTHS = ["Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"] as const;
const MONTHS_LOWER = ["ocak", "şubat", "mart", "nisan", "mayıs", "haziran", "temmuz", "ağustos", "eylül", "ekim", "kasım", "aralık"] as const;

// The possessive suffix after a number follows how its last word is read:
// bir'i, iki'si, üç'ü, dört'ü, beş'i, altı'sı, yedi'si, sekiz'i, dokuz'u, on'u, yirmi'si, otuz'u.
const DIGIT_SUFFIX = ["", "i", "si", "ü", "ü", "i", "sı", "si", "i", "u"] as const;
const TENS_SUFFIX: Record<number, string> = { 1: "u", 2: "si", 3: "u" };

export function dayWithSuffix(n: number): string {
  const suffix = n % 10 === 0 ? (TENS_SUFFIX[n / 10] ?? "u") : DIGIT_SUFFIX[n % 10];
  return `${String(n)}'${suffix ?? "i"}`;
}

/** day: 1..31, or -1 for the last day of the month. */
const dayLabel = (day: number) => (day === -1 ? "son günü" : dayWithSuffix(day));

/** "9 Aralık", or "aralık ayının son günü" for day -1. */
const dateInYear = (month: number, day: number) =>
  day === -1 ? `${MONTHS_LOWER[month - 1] ?? ""} ayının son günü` : `${String(day)} ${MONTHS[month - 1] ?? ""}`;

export const tr = {
  nav: "Tekrarlanan",
  title: "Tekrarlanan görevler",
  intro: "Kural her seferinde yalnızca sıradaki görevi, bitiş tarihinden birkaç gün önce oluşturur. Görevin bitiş tarihi tekrarın tarihidir, bu yüzden tamamlanmayan iş otomatik olarak \"gecikti\" olur.",
  newRule: "Yeni kural",
  editRule: "Kuralı düzenle",
  empty: "Henüz tekrarlanan görev yok.",
  titleLabel: "Başlık şablonu",
  titleHint: "{{month}}, {{year}} ve {{date}} tarihle değiştirilir: \"Ofis kirası — {{month}} ödemesi\".",
  freq: "Sıklık",
  freqs: { daily: "Günlük", weekly: "Haftalık", monthly: "Aylık", yearly: "Yıllık" },
  every: "Tekrar aralığı",
  everyUnit: { daily: "günde bir", weekly: "haftada bir", monthly: "ayda bir", yearly: "yılda bir" },
  weekdays: "Haftanın günleri",
  monthDay: "Ayın günü",
  lastDay: "Ayın son günü",
  month: "Ay",
  months: [...MONTHS],
  start: "Başlangıç",
  end: "Bitiş (isteğe bağlı)",
  lead: "Kaç gün önce oluşturulsun",
  enabled: "Etkin",
  next: "Sıradaki",
  upcoming: "Sonraki tarihler",
  ended: "Sona erdi",
  disabled: "Durduruldu",
  failed: "Hata: kural durduruldu",
  confirmDelete: "Kural silinsin mi? Önceden oluşturulmuş görevler kalacak.",
  adminOnlyEdit: "Kuralları yalnızca yöneticiler değiştirebilir.",
  noAccess: "Tekrarlanan görevler misafirler için kullanılamaz.",

  /** Monday first. */
  weekdayShort: ["Pzt", "Sal", "Çar", "Per", "Cum", "Cmt", "Paz"],
  weekdayLong: ["pazartesi", "salı", "çarşamba", "perşembe", "cuma", "cumartesi", "pazar"],
  /** One-line schedule summaries ("Her ayın 9'u"); day is 1..31 or -1 (last day), month 1..12. */
  describe: {
    daily: (n: number) => (n === 1 ? "Her gün" : `${String(n)} günde bir`),
    weekly: (n: number, days: string) => `${n === 1 ? "Her hafta" : `${String(n)} haftada bir`}: ${days}`,
    monthly: (n: number, day: number) => (n === 1 ? `Her ayın ${dayLabel(day)}` : `${String(n)} ayda bir, ayın ${dayLabel(day)}`),
    yearly: (n: number, month: number, day: number) =>
      `${n === 1 ? "Her yıl" : `${String(n)} yılda bir,`} ${dateInYear(month, day)}`,
  },
} satisfies Messages["recur"];
