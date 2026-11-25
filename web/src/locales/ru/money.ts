// Russian strings for money fields (ADR-0044).
import type { Messages } from "../../i18n/locale";

export const tm = {
  amount: "Сумма",
  currency: "Валюта",
  currencies: "Разрешённые валюты",
  currenciesHint: "Коды ISO 4217 через запятую: AZN, USD, EUR",
  defaultCurrency: "Валюта по умолчанию",
  scale: "Знаков после запятой",
  scaleFixed: "Не меняется после создания",
  saveCurrencies: "Сохранить валюты",
  badCodes: (codes: string) => `Неизвестный код валюты: ${codes}. Укажите код ISO 4217, например AZN, USD, EUR.`,
  amountOf: (field: string) => `${field}: сумма`,
  currencyOf: (field: string) => `${field}: валюта`,
  errors: {
    format: (field: string) => `${field}: введите сумму числом, например 1 250,50.`,
    scale: (field: string, scale: number) =>
      scale === 0 ? `${field}: введите целую сумму (в этом поле нет дробной части).` : `${field}: не больше ${String(scale)} знаков после запятой; округлите сумму.`,
    large: (field: string) => `${field}: сумма слишком большая; введите меньшее значение.`,
  },
  totalsLabel: (text: string) => `Итоги по валютам: ${text}`,
  noFx: "Валюты не складываются: у каждой валюты свой итог.",
} satisfies Messages["money"];
