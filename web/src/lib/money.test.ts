import { describe, expect, it } from "vitest";

import type { FieldDefinition } from "../api/types";
import { isAmount, isSortable } from "./formula";
import { amountDraft, formatMoney, formatMoneyTotals, isMoneyValue, parseAmount, parseCurrencyList } from "./money";
import { formatFieldValue, sumFields } from "./views";

const NB = " ";

describe("parseAmount", () => {
  it("reads typed amounts exactly, in the UI language's style", () => {
    expect(parseAmount("1 250,50", 2, "az")).toEqual({ ok: true, amount: "1250.50" });
    expect(parseAmount("1250.5", 2, "az")).toEqual({ ok: true, amount: "1250.50" });
    expect(parseAmount("1,250.50", 2, "en")).toEqual({ ok: true, amount: "1250.50" });
    expect(parseAmount("1,250", 2, "en")).toEqual({ ok: true, amount: "1250.00" }); // en grouping
    expect(parseAmount("1,25", 2, "az")).toEqual({ ok: true, amount: "1.25" });
    expect(parseAmount("1.250,50", 2, "tr")).toEqual({ ok: true, amount: "1250.50" });
    expect(parseAmount("1.234.567", 0, "tr")).toEqual({ ok: true, amount: "1234567" });
    expect(parseAmount("-0,10", 2, "az")).toEqual({ ok: true, amount: "-0.10" });
    expect(parseAmount("-0", 2, "az")).toEqual({ ok: true, amount: "0.00" });
    expect(parseAmount(`12${NB}000`, 0, "ru")).toEqual({ ok: true, amount: "12000" });
    expect(parseAmount("0.1", 4, "en")).toEqual({ ok: true, amount: "0.1000" });
  });
  it("never rounds and says why an amount is refused", () => {
    expect(parseAmount("1,005", 2, "az")).toEqual({ ok: false, error: "scale" });
    expect(parseAmount("1.5", 0, "en")).toEqual({ ok: false, error: "scale" });
    expect(parseAmount("abc", 2, "az")).toEqual({ ok: false, error: "format" });
    expect(parseAmount("1e5", 2, "az")).toEqual({ ok: false, error: "format" });
    expect(parseAmount("1,2,3", 2, "az")).toEqual({ ok: false, error: "format" });
    expect(parseAmount("12,", 2, "az")).toEqual({ ok: false, error: "format" });
    expect(parseAmount("10000000000000", 2, "az")).toEqual({ ok: false, error: "large" });
  });
});

describe("money display", () => {
  it("formats with the number style and the currency code", () => {
    expect(formatMoney({ amount: "1250.50", currency: "AZN" }, "az")).toBe(`1${NB}250,50${NB}AZN`);
    expect(formatMoney({ amount: "-1250.50", currency: "USD" }, "en")).toBe(`-1,250.50${NB}USD`);
    expect(formatMoneyTotals([{ currency: "AZN", sum: "1200.00" }, { currency: "USD", sum: "300.00" }], "az"))
      .toBe(`1${NB}200,00${NB}AZN · 300,00${NB}USD`);
    expect(formatMoneyTotals([], "az")).toBe("—");
    expect(amountDraft({ amount: "1250.50", currency: "AZN" }, "az")).toBe("1250,50");
    expect(amountDraft({ amount: "1250.50", currency: "AZN" }, "en")).toBe("1250.50");
  });
  it("tells money values from formula errors", () => {
    expect(isMoneyValue({ amount: "1.00", currency: "AZN" })).toBe(true);
    expect(isMoneyValue({ error: "overflow" })).toBe(false);
    expect(isMoneyValue("1")).toBe(false);
  });
  it("sums, sorts and right-aligns money columns", () => {
    const price: FieldDefinition = { id: "1", key: "price", name: "Qiymət", type: "money", archived: false,
      options: { money: { currencies: ["AZN", "USD"], default_currency: "AZN", scale: 2 } } };
    const fields = new Map([["price", price]]);
    expect(isAmount(price)).toBe(true);
    expect(isSortable(price)).toBe(true);
    expect(sumFields(["title", "field.price"], fields)).toEqual(["price"]);
    expect(formatFieldValue(price, { amount: "5.00", currency: "USD" }, new Map())).toMatch(new RegExp(`5[.,]00${NB}USD`));
  });
});

describe("parseCurrencyList", () => {
  it("normalizes codes and rejects anything that is not three letters", () => {
    expect(parseCurrencyList("azn, USD eur;usd")).toEqual({ ok: true, codes: ["AZN", "USD", "EUR"] });
    expect(parseCurrencyList("AZN, manat").ok).toBe(false);
    expect(parseCurrencyList("  ").ok).toBe(false);
  });
});
