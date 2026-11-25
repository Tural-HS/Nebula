import { useState } from "react";

import type { FieldDefinition, FieldValue, Member } from "../api/types";
import { tf } from "../i18n-formulas";
import { cn } from "../lib/cn";
import { isFormulaError, isNumeric } from "../lib/formula";
import { draftOf } from "../lib/inline";
import { formatFieldValue } from "../lib/views";
import { MoneyEditor } from "./MoneyEditor";
import { Input, Select } from "./ui";

/**
 * Edits one custom field value. Text-like inputs commit on blur/Enter; choice
 * inputs commit immediately. Empty input commits null (clears the value).
 */
export function FieldEditor({
  def,
  value,
  members,
  disabled,
  onCommit,
  id,
}: {
  def: FieldDefinition;
  value: FieldValue | undefined;
  members: Member[];
  disabled?: boolean;
  onCommit: (v: FieldValue) => void;
  id: string;
}) {
  const choices = (def.options.choices ?? []).filter((c) => !c.archived || c.id === value || (Array.isArray(value) && value.includes(c.id)));

  switch (def.type) {
    case "formula": {
      // Computed by the server (ADR-0030): shown, never edited.
      const expr = def.options.formula?.expression ?? "";
      return (
        <output id={id} data-formula="" className="flex min-w-0 items-baseline gap-1.5 text-sm" title={`${tf.readOnly}\n${tf.title(expr)}`}>
          <span aria-hidden className="font-serif text-zinc-400 italic">ƒ</span>
          <span className={cn("truncate", isNumeric(def) && "tabular-nums", isFormulaError(value) && "text-red-600 dark:text-red-400")}>
            {formatFieldValue(def, value, new Map()) || "—"}
          </span>
        </output>
      );
    }
    case "money":
      return <MoneyEditor key={JSON.stringify(value ?? null)} id={id} def={def} value={value} label={def.name} disabled={disabled} onCommit={onCommit} />;
    case "boolean":
      return (
        <input
          id={id}
          type="checkbox"
          className="size-4 accent-brand-600"
          checked={value === true}
          disabled={disabled}
          onChange={(e) => { onCommit(e.target.checked); }}
        />
      );
    case "select":
      return (
        <Select id={id} value={typeof value === "string" ? value : ""} disabled={disabled} onChange={(e) => { onCommit(e.target.value || null); }}>
          <option value="">—</option>
          {choices.map((c) => (
            <option key={c.id} value={c.id}>
              {c.label}
            </option>
          ))}
        </Select>
      );
    case "multi_select": {
      const selected = Array.isArray(value) ? value : [];
      return (
        <div id={id} className="flex flex-wrap gap-1.5">
          {choices.map((c) => {
            const on = selected.includes(c.id);
            return (
              <button
                key={c.id}
                type="button"
                disabled={disabled}
                aria-pressed={on}
                onClick={() => { onCommit(on ? selected.filter((x) => x !== c.id) : [...selected, c.id]); }}
                className={
                  "rounded-full px-2 py-0.5 text-xs ring-1 " +
                  (on ? "bg-brand-600 text-white ring-brand-600" : "text-zinc-600 ring-zinc-300 dark:text-zinc-300 dark:ring-zinc-700")
                }
              >
                {c.label}
              </button>
            );
          })}
        </div>
      );
    }
    case "user":
      return (
        <Select id={id} value={typeof value === "string" ? value : ""} disabled={disabled} onChange={(e) => { onCommit(e.target.value || null); }}>
          <option value="">—</option>
          {members.map((m) => (
            <option key={m.user_id} value={m.user_id}>
              {m.display_name}
            </option>
          ))}
        </Select>
      );
    default:
      return <TextLikeEditor key={draftOf(value)} def={def} value={value} disabled={disabled} onCommit={onCommit} id={id} />;
  }
}

const inputType: Partial<Record<FieldDefinition["type"], string>> = {
  number: "number",
  date: "date",
  datetime: "datetime-local",
  url: "url",
  email: "email",
};

function toInput(def: FieldDefinition, v: FieldValue | undefined): string {
  if (v === null || v === undefined || isFormulaError(v)) return "";
  if (def.type === "datetime" && typeof v === "string") {
    const d = new Date(v);
    // datetime-local wants local time without zone
    return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
  }
  return draftOf(v);
}

function fromInput(def: FieldDefinition, s: string): FieldValue {
  if (s.trim() === "") return null;
  if (def.type === "number") return Number(s);
  if (def.type === "datetime") return new Date(s).toISOString();
  return s;
}

function TextLikeEditor({
  def,
  value,
  disabled,
  onCommit,
  id,
}: {
  def: FieldDefinition;
  value: FieldValue | undefined;
  disabled: boolean | undefined;
  onCommit: (v: FieldValue) => void;
  id: string;
}) {
  const initial = toInput(def, value);
  const [draft, setDraft] = useState(initial);
  const commit = () => {
    if (draft !== initial) onCommit(fromInput(def, draft));
  };
  if (def.type === "long_text") {
    return (
      <textarea
        id={id}
        rows={3}
        value={draft}
        disabled={disabled}
        onChange={(e) => { setDraft(e.target.value); }}
        onBlur={commit}
        className="w-full rounded-md bg-white px-2.5 py-1.5 text-sm ring-1 ring-zinc-300 pointer-coarse:text-base focus:ring-2 focus:ring-brand-500 dark:bg-zinc-900 dark:ring-zinc-700"
      />
    );
  }
  return (
    <Input
      id={id}
      type={inputType[def.type] ?? "text"}
      value={draft}
      disabled={disabled}
      onChange={(e) => { setDraft(e.target.value); }}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") e.currentTarget.blur();
      }}
    />
  );
}
