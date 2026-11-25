import type { FieldDefinition } from "../api/types";

// Phone numbers in text fields become tap-to-call links (collections teams
// call from the list). Recognised by the field, not by the value alone, so an
// account or document number made of digits never turns into a link.
const PHONE_NAME = /(telefon|phone|mobil|əlaqə nömrəsi|elaqe nomresi|номер|телефон)/i;
const PHONE_KEY = /(^|_)(phone|tel|telefon|mobile|mobil)(_|$)/;

export function isPhoneField(def: FieldDefinition | undefined): boolean {
  return def?.type === "text" && (PHONE_NAME.test(def.name) || PHONE_KEY.test(def.key));
}

/** tel: URI of a phone-like value (7–15 digits, optional leading +), else null. */
export function telHref(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const v = value.trim();
  if (!/^\+?[\d\s().-]+$/.test(v)) return null;
  const digits = v.replace(/\D/g, "");
  if (digits.length < 7 || digits.length > 15) return null;
  return `tel:${v.startsWith("+") ? "+" : ""}${digits}`;
}
