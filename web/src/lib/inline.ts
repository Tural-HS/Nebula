// Inline cell editing helpers (ADR-0023), shared by the table and the bulk toolbar.

import type { ItemPatch } from "../api/hooks";
import type { FieldDefinition, FieldValue, Item, Member } from "../api/types";
import { fieldKey, isFieldColumn } from "./views";

export type InlineKind = "status" | "assignee" | "due_date" | "select" | "user" | "number" | "text" | "date" | "url" | "email";

export interface InlineEdit {
  ws: string;
  project: string;
  /** Today in the workspace's time zone (for the optimistic overdue flag). */
  today: string;
  /** People who may be assigned in this project. */
  assignable: Member[];
  commit: (item: Item, patch: Omit<ItemPatch, "version">, optimistic: Item) => void;
}

const FIELD_KINDS: Partial<Record<FieldDefinition["type"], InlineKind>> = {
  select: "select",
  user: "user",
  number: "number",
  text: "text",
  date: "date",
  url: "url",
  email: "email",
};

/** Which editor a column gets, or null when it is not edited inline. */
export function inlineKind(col: string, fields: ReadonlyMap<string, FieldDefinition>): InlineKind | null {
  if (col === "status" || col === "assignee" || col === "due_date") return col;
  if (!isFieldColumn(col)) return null;
  const def = fields.get(fieldKey(col));
  return def && !def.archived ? (FIELD_KINDS[def.type] ?? null) : null;
}

/** The text an input starts with for a field value. */
export function draftOf(v: FieldValue | undefined): string {
  return v === null || v === undefined ? "" : String(v);
}

/** Parses an input back to a field value; undefined means "not a valid value". */
export function parseDraft(kind: InlineKind, s: string): FieldValue | undefined {
  const v = s.trim();
  if (v === "") return null;
  if (kind === "number") {
    const n = Number(v.replace(",", "."));
    return Number.isFinite(n) ? n : undefined;
  }
  return v;
}

