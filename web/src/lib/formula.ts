// Formula fields (ADR-0030): what the UI needs to know about computed
// fields. The server parses, validates and computes; nothing here evaluates
// a formula.

import type { FieldDefinition, FieldValue, FormulaErrorValue } from "../api/types";
import type { ItemSort } from "../api/hooks";

// Local copies of views.ts helpers: views.ts imports this module.
const isFieldColumn = (c: string) => c.startsWith("field.");
const fieldKey = (c: string) => c.slice("field.".length);

export const isFormula = (def: FieldDefinition | undefined): boolean => def?.type === "formula";

/**
 * The type a field's values have: a formula behaves like its result type
 * (number, date or boolean) for formatting, totals, sorting and filters.
 */
export function valueType(def: FieldDefinition | undefined): FieldDefinition["type"] | "boolean" | undefined {
  if (!def) return undefined;
  if (def.type !== "formula") return def.type;
  return def.options.formula?.result_type;
}

export const isNumeric = (def: FieldDefinition | undefined): boolean => valueType(def) === "number";

/** Number and money fields: right-aligned, tabular figures, a total in the table (ADR-0044). */
export const isAmount = (def: FieldDefinition | undefined): boolean => isNumeric(def) || def?.type === "money";

export function isFormulaError(v: FieldValue | undefined): v is FormulaErrorValue {
  return typeof v === "object" && v !== null && !Array.isArray(v) && "error" in v && typeof v.error === "string";
}

/** "field.<key>" ascending or "-field.<key>" descending (server-side keyset sort). */
const FIELD_SORT = /^-?field\.[a-z][a-z0-9_]{0,62}$/;

export function parseSort(s: string | null | undefined): ItemSort {
  if (s === "due") return "due";
  if (s && FIELD_SORT.test(s)) return s as ItemSort;
  return "number";
}

/** Number, money (by amount, ADR-0044), date and formula (number/date result) fields can be sorted. */
export function isSortable(def: FieldDefinition | undefined): boolean {
  const t = valueType(def);
  return !!def && !def.archived && (t === "number" || t === "date" || t === "money");
}

/** The field a sort is on, or "" for the built-in sorts. */
export function sortFieldKey(sort: ItemSort): string {
  if (sort === "number" || sort === "due") return "";
  return sort.replace(/^-?field\./, "");
}

/** Sort choices for the sortable field columns currently shown. */
export function fieldSortOptions(
  columns: readonly string[],
  fields: ReadonlyMap<string, FieldDefinition>,
): { key: string; name: string }[] {
  return columns
    .filter(isFieldColumn)
    .map((c) => fields.get(fieldKey(c)))
    .filter((d): d is FieldDefinition => isSortable(d))
    .map((d) => ({ key: d.key, name: d.name }));
}

/**
 * Splits a formula around a 1-based error position, so the editor can mark
 * the character the server pointed at. Positions count characters (code
 * points), like the server's.
 */
export function splitAt(expr: string, position: number): { before: string; at: string; after: string } {
  const chars = Array.from(expr);
  const i = Math.min(Math.max(position - 1, 0), chars.length);
  return { before: chars.slice(0, i).join(""), at: chars[i] ?? " ", after: chars.slice(i + 1).join("") };
}
