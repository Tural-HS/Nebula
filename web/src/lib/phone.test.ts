import { describe, expect, it } from "vitest";

import type { FieldDefinition } from "../api/types";
import { isPhoneField, telHref } from "./phone";

const base: FieldDefinition = { id: "f", key: "x", name: "X", type: "text", options: {}, archived: false } as unknown as FieldDefinition;
const def = (patch: Partial<FieldDefinition>): FieldDefinition => ({ ...base, ...patch });

describe("isPhoneField", () => {
  it("recognises phone fields by name or key, text only", () => {
    expect(isPhoneField(def({ name: "Əlaqə nömrəsi", key: "contact" }))).toBe(true);
    expect(isPhoneField(def({ name: "Telefon" }))).toBe(true);
    expect(isPhoneField(def({ name: "Номер", key: "num" }))).toBe(true);
    expect(isPhoneField(def({ name: "X", key: "phone" }))).toBe(true);
    expect(isPhoneField(def({ name: "X", key: "home_tel" }))).toBe(true);
    expect(isPhoneField(def({ name: "NV nömrəsi", key: "nv_number" }))).toBe(false);
    expect(isPhoneField(def({ name: "Telefon", type: "number" }))).toBe(false);
    expect(isPhoneField(undefined)).toBe(false);
  });
});

describe("telHref", () => {
  it("builds tel: links from phone-like values", () => {
    expect(telHref("515614145")).toBe("tel:515614145");
    expect(telHref("+994 (50) 123-45-67")).toBe("tel:+994501234567");
    expect(telHref(" 12024989111 ")).toBe("tel:12024989111");
  });
  it("refuses everything else", () => {
    expect(telHref("10YX930")).toBeNull();
    expect(telHref("12345")).toBeNull();
    expect(telHref("1234567890123456")).toBeNull();
    expect(telHref("")).toBeNull();
    expect(telHref(515614145)).toBeNull();
    expect(telHref("javascript:alert(1)")).toBeNull();
  });
});

import { amountText } from "./views";

describe("amountText", () => {
  const num = (unit?: string): FieldDefinition => ({ ...base, type: "number", options: unit ? { unit } : {} });
  it("shows two decimals for currency units only", () => {
    expect(amountText(622.2, num("₼"))).toBe("622,20");
    expect(amountText("1315", num("AZN")).replace(/\s/g, " ")).toBe("1 315,00"); // grouping is a no-break space
    expect(amountText(12.5, num("%"))).toBe("12,5");
    expect(amountText(7, num())).toBe("7");
  });
});
