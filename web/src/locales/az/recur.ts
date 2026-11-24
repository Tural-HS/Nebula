// Azerbaijani strings for recurring items (ADR-0012).

const MONTHS_GENITIVE = ["yanvarın", "fevralın", "martın", "aprelin", "mayın", "iyunun", "iyulun", "avqustun", "sentyabrın", "oktyabrın", "noyabrın", "dekabrın"] as const;

// Possessive suffix after a number follows how the number is read:
// bir-i, iki-si, üç-ü, dörd-ü, beş-i, altı-sı, yeddi-si, səkkiz-i, doqquz-u, on-u, iyirmi-si, otuz-u.
const DIGIT_SUFFIX = ["", "i", "si", "ü", "ü", "i", "sı", "si", "i", "u"] as const;
const TENS_SUFFIX: Record<number, string> = { 1: "u", 2: "si", 3: "u" };

export function dayWithSuffix(n: number): string {
  const suffix = n % 10 === 0 ? (TENS_SUFFIX[n / 10] ?? "u") : DIGIT_SUFFIX[n % 10];
  return `${String(n)}-${suffix ?? "i"}`;
}

/** day: 1..31, or -1 for the last day of the month. */
const dayLabel = (day: number) => (day === -1 ? "son günü" : dayWithSuffix(day));

export const tr = {
  nav: "Təkrarlanan",
  title: "Təkrarlanan tapşırıqlar",
  intro: "Qayda hər dəfə yalnız növbəti tapşırığı yaradır, son tarixindən bir neçə gün əvvəl. Tapşırığın son tarixi təkrarın tarixidir, ona görə ödənilməyən iş avtomatik \"gecikir\" olur.",
  newRule: "Yeni qayda",
  editRule: "Qaydanı redaktə et",
  empty: "Hələ təkrarlanan tapşırıq yoxdur.",
  titleLabel: "Başlıq şablonu",
  titleHint: "{{month}}, {{year}} və {{date}} tarixlə əvəz olunur: \"Ofis icarəsi — {{month}} ödənişi\".",
  freq: "Tezlik",
  freqs: { daily: "Gündəlik", weekly: "Həftəlik", monthly: "Aylıq", yearly: "İllik" },
  every: "Hər neçə",
  everyUnit: { daily: "gündən bir", weekly: "həftədən bir", monthly: "aydan bir", yearly: "ildən bir" },
  weekdays: "Həftənin günləri",
  monthDay: "Ayın günü",
  lastDay: "Ayın son günü",
  month: "Ay",
  months: ["Yanvar", "Fevral", "Mart", "Aprel", "May", "İyun", "İyul", "Avqust", "Sentyabr", "Oktyabr", "Noyabr", "Dekabr"],
  start: "Başlanğıc",
  end: "Bitmə (istəyə görə)",
  lead: "Neçə gün əvvəl yaradılsın",
  enabled: "Aktiv",
  next: "Növbəti",
  upcoming: "Növbəti tarixlər",
  ended: "Bitib",
  disabled: "Dayandırılıb",
  failed: "Xəta: qayda dayandırıldı",
  confirmDelete: "Qayda silinsin? Artıq yaradılmış tapşırıqlar qalacaq.",
  adminOnlyEdit: "Qaydaları yalnız adminlər dəyişə bilər.",
  noAccess: "Təkrarlanan tapşırıqlar qonaqlar üçün əlçatan deyil.",

  /** Monday first. */
  weekdayShort: ["B.e.", "Ç.a.", "Ç.", "C.a.", "C.", "Ş.", "B."],
  weekdayLong: ["bazar ertəsi", "çərşənbə axşamı", "çərşənbə", "cümə axşamı", "cümə", "şənbə", "bazar"],
  /** One-line schedule summaries ("Hər ayın 9-u"); day is 1..31 or -1 (last day), month 1..12. */
  describe: {
    daily: (n: number) => (n === 1 ? "Hər gün" : `Hər ${String(n)} gündən bir`),
    weekly: (n: number, days: string) => `${n === 1 ? "Hər həftə" : `Hər ${String(n)} həftədən bir`}: ${days}`,
    monthly: (n: number, day: number) => (n === 1 ? `Hər ayın ${dayLabel(day)}` : `Hər ${String(n)} aydan bir, ayın ${dayLabel(day)}`),
    yearly: (n: number, month: number, day: number) =>
      `${n === 1 ? "Hər il" : `Hər ${String(n)} ildən bir`}, ${MONTHS_GENITIVE[month - 1] ?? ""} ${dayLabel(day)}`,
  },
} as const;
