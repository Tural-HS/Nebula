// Money fields (ADR-0044): exact decimal amounts with an ISO 4217 currency.
// Amounts stay text end to end ("1250.50"): never parsed into a float, so a
// cent is never lost. Shown as "1 250,50 AZN" (az) / "1,250.50 AZN" (en):
// the UI language's number style (formatDecimal) and the currency code, not
// a symbol — "$" is ambiguous and screen readers read codes reliably.
import type { FieldDefinition, FieldValue, MoneyOptions, MoneyValue } from "../api/types";
import { activeLocale, type Locale } from "../i18n/locale";
import { tm } from "../i18n-money";
import { formatDecimal } from "./views";

const NBSP = "\u00a0";

/** The options of a money field, with the server's defaults. */
export function moneyOptions(def: FieldDefinition | undefined): MoneyOptions {
  return def?.options.money ?? { currencies: ["AZN", "USD", "EUR"], default_currency: "AZN", scale: 2 };
}

export function isMoneyValue(v: FieldValue | undefined): v is MoneyValue {
  return typeof v === "object" && v !== null && !Array.isArray(v) && "amount" in v && typeof v.amount === "string";
}

/** "1 250,50 AZN" in the UI language. */
export function formatMoney(v: MoneyValue, locale: Locale = activeLocale()): string {
  return formatDecimal(v.amount, locale) + NBSP + v.currency;
}

/** Per-currency totals: "1 200,00 AZN · 300,00 USD" (never added together). */
export function formatMoneyTotals(list: readonly { currency: string; sum: string }[], locale: Locale = activeLocale()): string {
  if (list.length === 0) return "—";
  return list.map((c) => formatMoney({ amount: c.sum, currency: c.currency }, locale)).join(" · ");
}

export type AmountResult = { ok: true; amount: string } | { ok: false; error: "format" | "scale" | "large" };

const GROUP: Record<Locale, string> = { az: NBSP, en: ",", tr: ".", ru: NBSP };
const DECIMAL: Record<Locale, string> = { az: ",", en: ".", tr: ",", ru: "," };
const MAX_DIGITS = 15; // as the server: exact in a float64 and in Excel

/**
 * Parses a typed amount ("1 250,50", "1250.5", "-12") into the API's decimal
 * text with exactly `scale` decimals. Spaces are grouping; with both marks
 * the last one is the decimal mark; a lone mark is the decimal mark unless
 * it is the UI language's grouping mark followed by groups of three digits
 * ("1,250" in English is 1250). Never rounds: more decimals is an error.
 */
export function parseAmount(input: string, scale: number, locale: Locale = activeLocale()): AmountResult {
  let s = input.replace(/[\s\u00a0\u202f'’]/g, "").replace("\u2212", "-");
  let neg = false;
  if (s.startsWith("-")) { neg = true; s = s.slice(1); } else if (s.startsWith("+")) s = s.slice(1);
  if (!/^[\d.,]+$/.test(s) || !/\d/.test(s)) return { ok: false, error: "format" };
  let int = s, frac = "", group: string | null = null;
  const at = Math.max(s.lastIndexOf(","), s.lastIndexOf("."));
  if (at >= 0) {
    const mark = s.charAt(at);
    const other = mark === "," ? "." : ",";
    const count = s.split(mark).length - 1;
    let dec: string | null = mark;
    if (s.includes(other)) group = other; // both: the last one is the decimal mark
    else if (count > 1) { dec = null; group = mark; } // "1.234.567"
    else if (mark === GROUP[locale] && mark !== DECIMAL[locale] && /^\d{1,3}[.,]\d{3}$/.test(s)) { dec = null; group = mark; } // en "1,250"
    if (dec) {
      int = s.slice(0, at) || "0";
      frac = s.slice(at + 1);
      if (frac === "" || !/^\d+$/.test(frac)) return { ok: false, error: "format" };
    }
  }
  if (group && int.includes(group)) {
    const parts = int.split(group);
    if (parts.some((p, i) => !/^\d+$/.test(p) || (i === 0 ? p.length > 3 : p.length !== 3))) return { ok: false, error: "format" };
    int = parts.join("");
  }
  if (!/^\d+$/.test(int)) return { ok: false, error: "format" };
  int = int.replace(/^0+(?=\d)/, "");
  frac = frac.replace(/0+$/, "");
  if (frac.length > scale) return { ok: false, error: "scale" };
  if (int.length + scale > MAX_DIGITS) return { ok: false, error: "large" };
  let out = int + (scale > 0 ? "." + frac.padEnd(scale, "0") : "");
  if (neg && /[1-9]/.test(out)) out = "-" + out;
  return { ok: true, amount: out };
}

/** The text an amount input starts with: the stored amount in the UI language's style, without grouping. */
export function amountDraft(v: FieldValue | undefined, locale: Locale = activeLocale()): string {
  if (!isMoneyValue(v)) return "";
  return DECIMAL[locale] === "," ? v.amount.replace(".", ",") : v.amount;
}

/** The message for an amount that cannot be saved, naming the field and the fix. */
export function amountError(r: Extract<AmountResult, { ok: false }>, field: string, scale: number): string {
  return r.error === "scale" ? tm.errors.scale(field, scale) : tm.errors[r.error](field);
}

/** Splits "azn, USD eur" into unique upper-case codes; the server checks them against ISO 4217. */
export function parseCurrencyList(s: string): { ok: true; codes: string[] } | { ok: false; error: string } {
  const parts = s.split(/[\s,;]+/).map((x) => x.trim().toUpperCase()).filter(Boolean);
  const bad = parts.filter((x) => !/^[A-Z]{3}$/.test(x));
  if (parts.length === 0 || bad.length > 0) return { ok: false, error: tm.badCodes(bad.join(", ") || "—") };
  return { ok: true, codes: [...new Set(parts)] };
}
