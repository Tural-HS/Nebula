// Intake form logic (ADR-0032) that needs no React: turning what a requester
// typed into API values, client-side required checks (the server checks
// again), and the builder's field list, ordering and title template rules.
import type { FormFieldDef, FormInput, PublicFormField } from "../api/forms";
import type { FieldDefinition, ItemType } from "../api/types";

export const BUILTIN_KEYS = ["description", "due_date"] as const;
export type BuiltinKey = (typeof BUILTIN_KEYS)[number];
export const isBuiltin = (key: string): key is BuiltinKey => (BUILTIN_KEYS as readonly string[]).includes(key);

/** Field types a requester can type in (no member pickers). Mirrors store.formFieldTypes. */
const INPUT_TYPES = new Set(["text", "long_text", "number", "date", "datetime", "boolean", "select", "multi_select", "url", "email"]);

/** The DOM id of a field's input, for labels and error-summary links. */
export const inputId = (prefix: string, key: string) => `${prefix}-${key}`;

/** What a requester has typed so far, per field key. */
export type Draft = Record<string, string | boolean | string[] | undefined>;

export type FieldProblem = "required" | "checked" | "number";

/**
 * Converts the draft into the API's `values`: blanks are left out, numbers
 * parsed, datetime-local becomes ISO. Returns the problems the browser can
 * see by itself, in form order.
 */
export function toValues(fields: readonly PublicFormField[], draft: Draft): { values: Record<string, unknown>; problems: [string, FieldProblem][] } {
  const values: Record<string, unknown> = {};
  const problems: [string, FieldProblem][] = [];
  for (const f of fields) {
    const raw = draft[f.key];
    let value: unknown;
    switch (f.type) {
      case "boolean":
        if (raw === true) value = true;
        else if (f.required) {
          problems.push([f.key, "checked"]);
          continue;
        }
        break;
      case "multi_select":
        if (Array.isArray(raw) && raw.length > 0) value = raw;
        break;
      case "number": {
        const s = typeof raw === "string" ? raw.trim() : "";
        if (s !== "") {
          const n = Number(s.replace(",", "."));
          if (!Number.isFinite(n)) {
            problems.push([f.key, "number"]);
            continue;
          }
          value = n;
        }
        break;
      }
      case "datetime": {
        const s = typeof raw === "string" ? raw.trim() : "";
        if (s !== "") {
          const d = new Date(s);
          if (!Number.isNaN(d.getTime())) value = d.toISOString();
        }
        break;
      }
      default: {
        const s = typeof raw === "string" ? raw.trim() : "";
        if (s !== "") value = s;
      }
    }
    if (value === undefined) {
      if (f.required) problems.push([f.key, "required"]);
      continue;
    }
    values[f.key] = value;
  }
  return { values, problems };
}

/** "values.vendor" → "vendor"; anything else → undefined. */
export function valueKey(serverField: string | undefined): string | undefined {
  return serverField?.startsWith("values.") ? serverField.slice("values.".length) : undefined;
}

/** A field the builder can add: a built-in or a custom field bound to the type. */
export interface FieldOption {
  key: string;
  name: string;
  builtin: boolean;
}

/** Keys the item type requires: they must be on the form, as required. */
export function requiredByType(type: ItemType | undefined): Set<string> {
  return new Set((type?.fields ?? []).filter((f) => f.required).map((f) => f.key));
}

/** Fields that may still be added, in the item type's order (built-ins first). */
export function addableFields(
  type: ItemType | undefined,
  defs: readonly FieldDefinition[],
  onForm: readonly FormFieldDef[],
  builtinName: (k: BuiltinKey) => string,
): FieldOption[] {
  const taken = new Set(onForm.map((f) => f.key));
  const byKey = new Map(defs.map((d) => [d.key, d]));
  const out: FieldOption[] = BUILTIN_KEYS.filter((k) => !taken.has(k)).map((k) => ({ key: k, name: builtinName(k), builtin: true }));
  for (const b of type?.fields ?? []) {
    const d = byKey.get(b.key);
    if (!d || d.archived || !INPUT_TYPES.has(d.type) || taken.has(b.key)) continue;
    out.push({ key: d.key, name: d.name, builtin: false });
  }
  return out;
}

/** A new form for an item type: the type's required fields are already on it. */
export function newForm(type: ItemType | undefined, defs: readonly FieldDefinition[], title = ""): FormInput {
  const byKey = new Map(defs.map((d) => [d.key, d]));
  const fields: FormFieldDef[] = (type?.fields ?? [])
    .filter((f) => f.required && byKey.get(f.key) && !byKey.get(f.key)?.archived)
    .map((f) => ({ key: f.key, label: byKey.get(f.key)?.name ?? f.key, required: true }));
  return {
    slug: "", title, intro: null, item_type: type?.key ?? "", title_template: "{{form}}", fields,
    default_assignee_id: null, initial_status_id: null, audience: "members", enabled: true, max_files: 0,
  };
}

/** Moves the entry at `from` to index `to` (drag and drop, arrow buttons). */
export function moveField<T>(list: readonly T[], from: number, to: number): T[] {
  if (from === to || from < 0 || from >= list.length || to < 0 || to >= list.length) return [...list];
  const next = [...list];
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved as T);
  return next;
}

const PLACEHOLDER = /\{\{\s*([a-z0-9_.]*)\s*\}\}/g;

/** Placeholders the server would reject (mirror of store.checkTitleTemplate). */
export function unknownPlaceholders(template: string, fields: readonly FormFieldDef[]): string[] {
  const keys = new Set(fields.filter((f) => !isBuiltin(f.key)).map((f) => f.key));
  const bad: string[] = [];
  for (const m of template.matchAll(PLACEHOLDER)) {
    const name = m[1] ?? "";
    if (name === "requester" || name === "form") continue;
    if (name.startsWith("field.") && keys.has(name.slice("field.".length))) continue;
    bad.push(name);
  }
  return bad;
}

/** The title an item would get (mirror of store.renderTitle, for the preview). */
export function previewTitle(template: string, display: Readonly<Record<string, string>>, requester: string, formTitle: string): string {
  const oneLine = (s: string) => s.split(/\s+/).filter(Boolean).join(" ");
  const out = template
    .replace(PLACEHOLDER, (_, name: string) => {
      if (name === "requester") return oneLine(requester);
      if (name === "form") return oneLine(formTitle);
      return oneLine(display[name.replace(/^field\./, "")] ?? "");
    })
    .trim();
  return out === "" ? formTitle : out;
}

/** The address of a form ("Ödəniş tələbi" → "odenis-telebi"), for a new form. */
export function suggestSlug(title: string, slugKey: (s: string) => string): string {
  return slugKey(title).replace(/_/g, "-").slice(0, 63);
}

/** The shareable link of a form. */
export function formLink(origin: string, slug: string): string {
  return `${origin.replace(/\/+$/, "")}/f/${encodeURIComponent(slug)}`;
}
