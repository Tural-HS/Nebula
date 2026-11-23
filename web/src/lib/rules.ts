// Pure helpers for the automation rule builder: form strings <-> API values.
import type { ConditionOp } from "../api/admin";
import type { FieldDefinition } from "../api/types";

export const NO_VALUE_OPS: readonly ConditionOp[] = ["is_empty", "is_not_empty"];
export const LIST_OPS: readonly ConditionOp[] = ["in", "not_in"];

/** Converts the text a user typed for a condition into the API value. */
export function conditionValue(op: ConditionOp, raw: string): unknown {
  if (NO_VALUE_OPS.includes(op)) return undefined;
  if (LIST_OPS.includes(op)) return raw.split(",").map((s) => s.trim()).filter(Boolean);
  return raw.trim();
}

/** Inverse of conditionValue, for editing existing rules. */
export function conditionText(value: unknown): string {
  if (value === undefined || value === null) return "";
  if (Array.isArray(value)) return value.map((v) => (typeof v === "string" ? v : JSON.stringify(v))).join(", ");
  return typeof value === "string" ? value : JSON.stringify(value);
}

/** Converts a set_field value typed by the user into the field type's JSON. */
export function fieldValue(def: FieldDefinition | undefined, raw: string): unknown {
  const s = raw.trim();
  switch (def?.type) {
    case "number": {
      const n = Number(s);
      return s === "" || Number.isNaN(n) ? s : n;
    }
    case "boolean":
      return s === "true";
    case "multi_select":
      return s.split(",").map((x) => x.trim()).filter(Boolean);
    default:
      return s;
  }
}

/** Splits an email recipient list ("assignee, ops@x.az"). */
export function recipients(raw: string): string[] {
  return raw.split(",").map((s) => s.trim()).filter(Boolean);
}
