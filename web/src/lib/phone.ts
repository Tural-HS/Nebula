import type { FieldDefinition } from "../api/types";

// Phone numbers in text fields become tap-to-call links (collections teams
// call from the list). Recognised by the field, not by the value alone, so an
// account or document number made of digits never turns into a link.
const PHONE_NAME = /(telefon|phone|mobil|əlaqə nömrəsi|elaqe nomresi|номер|телефон)/i;
const PHONE_KEY = /(^|_)(phone|tel|telefon|mobile|mobil)(_|$)/;

export function isPhoneField(def: FieldDefinition | undefined): boolean {
  return def?.type === "text" && (PHONE_NAME.test(def.name) || PHONE_KEY.test(def.key));
}

// Azerbaijani numbers are often stored without the country code: 9 digits
// ("515614145") or with the trunk 0 ("0515614145"). Dialled as they are from
// a phone they reach nobody, so they become +994… (operator and area codes:
// mobile 10/50/51/55/60/70/77/99, Baku 12, other cities 2x and 18, 36).
const AZ_NATIONAL = /^(10|12|18|2\d|36|50|51|55|60|70|77|99)\d{7}$/;

/** E.164-style digits of a phone-like value (7–15 digits, optional leading +), else null. */
function phoneDigits(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const v = value.trim();
  if (!/^\+?[\d\s().-]+$/.test(v)) return null;
  let digits = v.replace(/\D/g, "");
  if (digits.length < 7 || digits.length > 15) return null;
  if (v.startsWith("+")) return `+${digits}`;
  if (digits.length === 10 && digits.startsWith("0") && AZ_NATIONAL.test(digits.slice(1))) digits = digits.slice(1);
  if (digits.length === 9 && AZ_NATIONAL.test(digits)) return `+994${digits}`;
  if (digits.length === 12 && digits.startsWith("994") && AZ_NATIONAL.test(digits.slice(3))) return `+${digits}`;
  return digits;
}

/** tel: URI of a phone-like value, else null. */
export function telHref(value: unknown): string | null {
  const d = phoneDigits(value);
  return d === null ? null : `tel:${d}`;
}

/** An Azerbaijani number in the usual grouping ("+994 51 561 41 45"); other values as they are. */
export function phoneText(value: unknown, fallback: string): string {
  const d = phoneDigits(value);
  const m = d === null ? null : /^\+994(\d{2})(\d{3})(\d{2})(\d{2})$/.exec(d);
  return m ? `+994 ${m[1] ?? ""} ${m[2] ?? ""} ${m[3] ?? ""} ${m[4] ?? ""}` : fallback;
}
