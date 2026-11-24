// Turkish strings for formula fields (ADR-0030).
import type { Messages } from "../../i18n/locale";

export const tf = {
  expression: "Formül",
  expressionHint: "Alan anahtarları, sayılar ve fonksiyonlar: amount - paid, round(amount * 0.18, 2), due - today()",
  help: "Fonksiyonlar: if, round, min, max, abs, coalesce, today, days, workdays, date. Görevin bitiş tarihi: @due_date. Boş değer boş sonuç verir; 0 saymak için coalesce(paid, 0) kullanın.",
  helpTitle: "Fonksiyonlar",
  scale: "Ondalık basamak",
  sampleItem: "Örnek görev",
  sampleItemHint: "ör. FIN-12",
  checking: "Kontrol ediliyor…",
  valid: "Formül geçerli",
  errorAt: (pos: number, msg: string) => `Konum ${String(pos)}: ${msg}`,
  result: "Sonuç",
  resultEmpty: "boş",
  resultTypes: { number: "sayı", date: "tarih", boolean: "evet/hayır" } as Record<string, string>,
  resultType: (type: string) => `Sonuç türü: ${type}`,
  refs: (n: number) => `${String(n)} alan kullanıyor`,
  volatile: "today() kullanıyor: her gün çalışma alanının saat diliminde yenilenir",
  calendar: "workdays() kullanıyor: iş günü takvimi değişince yeniden hesaplanır",
  save: "Formülü kaydet",
  readOnly: "Hesaplanan alan: değeri elle değiştirilemez",
  title: (expr: string) => `ƒ = ${expr}`,
  errors: {
    division_by_zero: "#SIFIRA BÖLME",
    overflow: "#ÇOK BÜYÜK",
    date_out_of_range: "#TARİH",
    invalid_argument: "#HATA",
  } as Record<string, string>,
  sortAsc: (name: string) => `${name} ↑`,
  sortDesc: (name: string) => `${name} ↓`,
} satisfies Messages["formulas"];
