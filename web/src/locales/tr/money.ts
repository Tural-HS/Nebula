// Turkish strings for money fields (ADR-0044).
import type { Messages } from "../../i18n/locale";

export const tm = {
  amount: "Tutar",
  currency: "Para birimi",
  currencies: "İzin verilen para birimleri",
  currenciesHint: "ISO 4217 kodları, virgülle: AZN, USD, EUR",
  defaultCurrency: "Varsayılan para birimi",
  scale: "Ondalık basamak",
  scaleFixed: "Oluşturulduktan sonra değişmez",
  saveCurrencies: "Para birimlerini kaydet",
  badCodes: (codes: string) => `Bilinmeyen para birimi kodu: ${codes}. AZN, USD, EUR gibi bir ISO 4217 kodu yazın.`,
  amountOf: (field: string) => `${field}: tutar`,
  currencyOf: (field: string) => `${field}: para birimi`,
  errors: {
    format: (field: string) => `${field}: tutarı sayı olarak yazın, örn. 1.250,50.`,
    scale: (field: string, scale: number) =>
      scale === 0 ? `${field}: tam tutar yazın (bu alanda ondalık yok).` : `${field}: en fazla ${String(scale)} ondalık basamak kullanın; tutarı yuvarlayın.`,
    large: (field: string) => `${field}: tutar çok büyük; daha küçük bir değer yazın.`,
  },
  totalsLabel: (text: string) => `Para birimine göre toplamlar: ${text}`,
  noFx: "Para birimleri toplanmaz: her para biriminin kendi toplamı vardır.",
} satisfies Messages["money"];
