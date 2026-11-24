// Azerbaijani strings for the working-day calendar (ADR-0027).

const MONTHS_GENITIVE = ["yanvarın", "fevralın", "martın", "aprelin", "mayın", "iyunun", "iyulun", "avqustun", "sentyabrın", "oktyabrın", "noyabrın", "dekabrın"] as const;

// Ordinal suffix follows how the number is read: bir-inci, iki-nci, üç-üncü,
// dörd-üncü, beş-inci, altı-ncı, yeddi-nci, səkkiz-inci, doqquz-uncu, on-uncu, iyirmi-nci.
const DIGIT_SUFFIX = ["", "ci", "ci", "cü", "cü", "ci", "cı", "ci", "ci", "cu"] as const;
const TENS_SUFFIX: Record<number, string> = { 1: "cu", 2: "ci", 3: "cu" };

export function ordinal(n: number): string {
  const suffix = n % 10 === 0 ? (TENS_SUFFIX[n / 10] ?? "cu") : DIGIT_SUFFIX[n % 10];
  return `${String(n)}-${suffix ?? "ci"}`;
}

/** workDay: 1..23, or -1 for the last working day. */
const workDayLabel = (workDay: number) => (workDay === -1 ? "son iş günü" : `${ordinal(workDay)} iş günü`);

export const tw = {
  nav: "İş təqvimi",
  title: "İş təqvimi",
  intro:
    "Həftə sonları, bayramlar və köçürülmüş iş günləri təkrarlanan tapşırıqların tarixlərini müəyyən edir: \"ayın son iş günü\", \"ayın 5-ci iş günü\" və qeyri-iş gününə düşən tarixin köçürülməsi. Dəyişiklik qaydaların növbəti tarixini dərhal yenidən hesablayır; keçmiş tarixlər üçün tapşırıq yaradılmır, artıq yaradılmış tapşırıqlara toxunulmur.",
  weekend: "Həftə sonu günləri",
  weekendHint: "Ən çox 3 gün. Təqvim görünüşündə də bu günlər fərqləndirilir.",
  year: "İl",
  prevYear: "Əvvəlki il",
  nextYear: "Növbəti il",
  days: "Bayramlar və köçürmələr",
  empty: (year: number) => `${String(year)} ili üçün hələ tarix əlavə edilməyib.`,
  count: (n: number, max: number) => `${String(n)} / ${String(max)} tarix`,
  date: "Tarix",
  name: "Ad",
  kind: "Növ",
  kinds: { holiday: "Qeyri-iş günü (bayram)", workday: "İş günü (köçürmə)" },
  kindShort: { holiday: "Bayram", workday: "İş günü" },
  defaultWorkdayName: "Köçürülmüş iş günü",
  add: "Əlavə et",
  remove: (date: string) => `${date} tarixini sil`,
  added: "Tarix əlavə edildi",
  removed: "Tarix silindi",
  workdayNeedsWeekend: "İş günü kimi yalnız həftə sonu günü elan etmək olar.",
  invalidDate: "2000–2100 aralığında düzgün tarix seçin.",
  nameRequired: "Ad tələb olunur (ən çox 100 simvol).",
  limitReached: (max: number) => `Bir ildə ən çox ${String(max)} tarix ola bilər.`,
  onWeekend: "həftə sonuna düşür",
  presetTitle: "Azərbaycanın sabit tarixli bayramları",
  presetButton: (year: number) => `${String(year)} ilinə əlavə et`,
  presetHint:
    "Əmək Məcəlləsinin 105-ci maddəsində sabit tarixi olan qeyri-iş günləri əlavə olunur: 1–2 yanvar, 20 yanvar, 8 mart, 9 may, 28 may, 15 iyun, 26 iyun, 8 və 9 noyabr, 31 dekabr. Artıq əlavə etdiyiniz tarixlərə toxunulmur.",
  presetOfficial: (year: number) =>
    `${String(year)} ilinin rəsmi təqvimi əlavə olunur: bütün bayramlar (Novruz, Ramazan, Qurban daxil) və köçürülmüş istirahət günləri. Mənbə: Nazirlər Kabinetinin qərarı və Əmək və Əhalinin Sosial Müdafiəsi Nazirliyinin istehsalat təqvimi. Artıq əlavə etdiyiniz tarixlərə toxunulmur.`,
  presetYearly:
    "Hər il təsdiqlənməlidir: Novruz, Ramazan və Qurban bayramlarının günləri, bayram həftə sonuna düşəndə köçürülən istirahət günləri və iş günlərinin köçürülməsi hər il Nazirlər Kabinetinin qərarı ilə elan edilir. Onları qərar dərc olunandan sonra burada əl ilə əlavə edin.",
  presetAdded: (n: number) => (n === 0 ? "Yeni tarix yoxdur: hamısı artıq əlavə edilib." : `${String(n)} tarix əlavə edildi`),

  /** Calendar view. */
  holidayLabel: (name: string) => `Qeyri-iş günü: ${name}`,
  workdayLabel: (name: string) => `İş günü: ${name}`,
  weekendLabel: "Həftə sonu",

  /** Recurrence form. */
  dayMode: "Gün",
  dayModes: { date: "Ayın günü", workday: "İş günü" },
  workDay: "Neçənci iş günü",
  lastWorkDay: "Son iş günü",
  workDayOption: (n: number) => workDayLabel(n),
  shift: "Qeyri-iş gününə düşərsə",
  shifts: { none: "Köçürülməsin", previous: "Əvvəlki iş gününə", next: "Növbəti iş gününə" },
  calendarHint: "Həftə sonu və bayramlar workspace-in iş təqvimindən götürülür.",
  describe: {
    monthly: (n: number, workDay: number) =>
      n === 1 ? `Hər ayın ${workDayLabel(workDay)}` : `Hər ${String(n)} aydan bir, ayın ${workDayLabel(workDay)}`,
    yearly: (n: number, month: number, workDay: number) =>
      `${n === 1 ? "Hər il" : `Hər ${String(n)} ildən bir`}, ${MONTHS_GENITIVE[month - 1] ?? ""} ${workDayLabel(workDay)}`,
    shift: { previous: " (qeyri-iş günüdürsə, əvvəlki iş günü)", next: " (qeyri-iş günüdürsə, növbəti iş günü)" },
  },
} as const;
