// Azerbaijani strings for money fields (ADR-0044).
export const tm = {
  amount: "Məbləğ",
  currency: "Valyuta",
  currencies: "İcazəli valyutalar",
  currenciesHint: "ISO 4217 kodları, vergüllə: AZN, USD, EUR",
  defaultCurrency: "Əsas valyuta",
  scale: "Onluq rəqəmlər",
  scaleFixed: "Yaradıldıqdan sonra dəyişmir",
  saveCurrencies: "Valyutaları saxla",
  badCodes: (codes: string) => `Naməlum valyuta kodu: ${codes}. ISO 4217 kodu yazın, məsələn AZN, USD, EUR.`,
  amountOf: (field: string) => `${field}: məbləğ`,
  currencyOf: (field: string) => `${field}: valyuta`,
  errors: {
    format: (field: string) => `${field}: məbləği rəqəmlə yazın, məsələn 1250,50.`,
    scale: (field: string, scale: number) =>
      scale === 0 ? `${field}: tam məbləğ yazın (bu sahədə onluq hissə yoxdur).` : `${field}: ən çox ${String(scale)} onluq rəqəm yazın, məbləği yuvarlaqlaşdırın.`,
    large: (field: string) => `${field}: məbləğ çox böyükdür, daha kiçik dəyər yazın.`,
  },
  totalsLabel: (text: string) => `Cəm valyutalar üzrə: ${text}`,
  noFx: "Valyutalar toplanmır: hər valyutanın cəmi ayrıca göstərilir.",
};
