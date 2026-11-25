// English strings for money fields (ADR-0044).
import type { Messages } from "../../i18n/locale";

export const tm = {
  amount: "Amount",
  currency: "Currency",
  currencies: "Allowed currencies",
  currenciesHint: "ISO 4217 codes, comma-separated: AZN, USD, EUR",
  defaultCurrency: "Default currency",
  scale: "Decimal places",
  scaleFixed: "Cannot change after creation",
  saveCurrencies: "Save currencies",
  badCodes: (codes: string) => `Unknown currency code: ${codes}. Use an ISO 4217 code such as AZN, USD, EUR.`,
  amountOf: (field: string) => `${field}: amount`,
  currencyOf: (field: string) => `${field}: currency`,
  errors: {
    format: (field: string) => `${field}: type the amount as a number, e.g. 1,250.50.`,
    scale: (field: string, scale: number) =>
      scale === 0 ? `${field}: use a whole amount (this field has no decimals).` : `${field}: use at most ${String(scale)} decimals; round the amount.`,
    large: (field: string) => `${field}: the amount is too large; use a smaller value.`,
  },
  totalsLabel: (text: string) => `Totals by currency: ${text}`,
  noFx: "Currencies are never added together: each currency has its own total.",
} satisfies Messages["money"];
