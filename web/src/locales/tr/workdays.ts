// Turkish strings for the working-day calendar (ADR-0027).
import type { Messages } from "../../i18n/locale";

const MONTHS_LOWER = ["ocak", "şubat", "mart", "nisan", "mayıs", "haziran", "temmuz", "ağustos", "eylül", "ekim", "kasım", "aralık"] as const;

const workDayLabel = (workDay: number) => (workDay === -1 ? "son iş günü" : `${String(workDay)}. iş günü`);

export const tw = {
  nav: "İş günleri",
  title: "İş günü takvimi",
  intro:
    "Hafta sonları, bayramlar ve taşınan iş günleri tekrarlanan görevlerin tarihlerini belirler: \"ayın son iş günü\", \"ayın 5. iş günü\" ve iş günü olmayan bir güne denk gelen tarihin kaydırılması. Değişiklik kuralların sonraki tarihini hemen yeniden hesaplar; geçmiş tarihler için görev oluşturulmaz, mevcut görevlere dokunulmaz.",
  weekend: "Hafta sonu günleri",
  weekendHint: "En fazla 3 gün. Takvim görünümü de bu günleri işaretler.",
  year: "Yıl",
  prevYear: "Önceki yıl",
  nextYear: "Sonraki yıl",
  days: "Bayramlar ve taşımalar",
  empty: (year: number) => `${String(year)} yılı için henüz tarih eklenmedi.`,
  count: (n: number, max: number) => `${String(n)} / ${String(max)} tarih`,
  date: "Tarih",
  name: "Ad",
  kind: "Tür",
  kinds: { holiday: "Çalışılmayan gün (bayram)", workday: "İş günü (taşıma)" },
  kindShort: { holiday: "Bayram", workday: "İş günü" },
  defaultWorkdayName: "Taşınan iş günü",
  add: "Ekle",
  remove: (date: string) => `${date} tarihini sil`,
  added: "Tarih eklendi",
  removed: "Tarih silindi",
  workdayNeedsWeekend: "Yalnızca hafta sonu günü iş günü ilan edilebilir.",
  invalidDate: "2000–2100 arasında geçerli bir tarih seçin.",
  nameRequired: "Ad zorunludur (en fazla 100 karakter).",
  limitReached: (max: number) => `Bir yılda en fazla ${String(max)} tarih olabilir.`,
  onWeekend: "hafta sonuna denk geliyor",
  presetTitle: "Azerbaycan'ın sabit tarihli bayramları",
  presetButton: (year: number) => `${String(year)} yılına ekle`,
  presetHint:
    "İş Kanunu'nun (Əmək Məcəlləsi) 105. maddesinde sabit tarihi olan çalışılmayan günler eklenir: 1–2 Ocak, 8 Mart, 9 Mayıs, 28 Mayıs, 15 Haziran, 26 Haziran, 8 ve 9 Kasım, 31 Aralık. Daha önce eklediğiniz tarihlere dokunulmaz.",
  presetYearly:
    "Her yıl doğrulanmalıdır: Nevruz, Ramazan ve Kurban bayramlarının günleri, bayram hafta sonuna denk geldiğinde taşınan dinlenme günleri ve iş günü taşımaları her yıl Bakanlar Kurulu kararıyla ilan edilir. Karar yayımlandıktan sonra bunları buraya elle ekleyin.",
  presetAdded: (n: number) => (n === 0 ? "Yeni tarih yok: hepsi zaten ekli." : `${String(n)} tarih eklendi`),

  holidayLabel: (name: string) => `Çalışılmayan gün: ${name}`,
  workdayLabel: (name: string) => `İş günü: ${name}`,
  weekendLabel: "Hafta sonu",

  dayMode: "Gün",
  dayModes: { date: "Ayın günü", workday: "İş günü" },
  workDay: "Kaçıncı iş günü",
  lastWorkDay: "Son iş günü",
  workDayOption: (n: number) => (n === -1 ? "Son iş günü" : `${String(n)}. iş günü`),
  shift: "Çalışılmayan güne denk gelirse",
  shifts: { none: "Kaydırma", previous: "Önceki iş gününe", next: "Sonraki iş gününe" },
  calendarHint: "Hafta sonları ve bayramlar çalışma alanının iş günü takviminden alınır.",
  describe: {
    monthly: (n: number, workDay: number) =>
      n === 1 ? `Her ayın ${workDayLabel(workDay)}` : `${String(n)} ayda bir, ayın ${workDayLabel(workDay)}`,
    yearly: (n: number, month: number, workDay: number) =>
      `${n === 1 ? "Her yıl," : `${String(n)} yılda bir,`} ${MONTHS_LOWER[month - 1] ?? ""} ayının ${workDayLabel(workDay)}`,
    shift: { previous: " (iş günü değilse: önceki iş günü)", next: " (iş günü değilse: sonraki iş günü)" },
  },
} satisfies Messages["workdays"];
