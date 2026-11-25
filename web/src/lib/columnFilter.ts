import type { FieldDefinition } from "../api/types";
import { valueType } from "./formula";

// Column header filters (ADR-0048). The value is the one of ?field.<key>=:
// "~text" contains (text-like fields), "min..max" ranges (number, date),
// a choice id, "true"/"false", or a currency code (money).

export type FilterKind = "contains" | "number" | "date" | "choice" | "boolean" | "money";

const TEXT_LIKE = new Set(["text", "long_text", "url", "email"]);

export function filterKind(def: FieldDefinition | undefined): FilterKind | null {
  if (!def || def.archived) return null;
  if (TEXT_LIKE.has(def.type)) return "contains";
  if (def.type === "select" || def.type === "multi_select") return "choice";
  if (def.type === "money") return "money";
  const t = valueType(def);
  if (t === "number") return "number";
  if (t === "date") return "date";
  if (t === "boolean") return "boolean";
  return null;
}

/** A "contains" filter value, or "" to clear. */
export function containsValue(text: string): string {
  const v = text.trim().slice(0, 100);
  return v ? `~${v}` : "";
}

/** A range filter value from two (possibly empty) bounds, or "" to clear. */
export function rangeValue(from: string, to: string): string {
  const a = from.trim();
  const b = to.trim();
  return a || b ? `${a}..${b}` : "";
}

/** The parts of a stored value, for the editor's inputs. */
export function parseFilter(value: string | undefined): { contains: string; from: string; to: string; exact: string } {
  const v = value ?? "";
  if (v.startsWith("~")) return { contains: v.slice(1), from: "", to: "", exact: "" };
  if (v.includes("..")) {
    const [from = "", to = ""] = v.split("..", 2);
    return { contains: "", from, to, exact: "" };
  }
  return { contains: "", from: "", to: "", exact: v };
}

/** Human text of an operator value for the filter chip; exact values return null (the caller labels choices). */
export function operatorText(value: string, fmt: (bound: string) => string, words: { contains: (v: string) => string; from: (v: string) => string; to: (v: string) => string }): string | null {
  const p = parseFilter(value);
  if (value.startsWith("~")) return words.contains(p.contains);
  if (value.includes("..")) {
    if (p.from && p.to) return `${fmt(p.from)} – ${fmt(p.to)}`;
    return p.from ? words.from(fmt(p.from)) : words.to(fmt(p.to));
  }
  return null;
}
