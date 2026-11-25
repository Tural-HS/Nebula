import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";

import { useTransitions } from "../api/hooks";
import type { FieldDefinition, Item, Status } from "../api/types";
import { useToast } from "../components/toast-context";
import { MoneyEditor } from "../components/MoneyEditor";
import { Input, Select } from "../components/ui";
import { t } from "../i18n";
import { tc } from "../i18n-calendar";
import { overdueOn } from "../lib/calendar";
import { draftOf, parseDraft, type InlineEdit, type InlineKind } from "../lib/inline";
import { fieldKey, isFieldColumn } from "../lib/views";

// ---------------------------------------------------------------------------
// Inline cell editing (ADR-0023): click (or Enter on) a cell to edit it in
// place. Enter or leaving the cell saves, Escape cancels. The list shows the
// new value at once and goes back if the server refuses it; the edit carries
// the item's version, so a concurrent change is a 409 as in the item panel.
// ---------------------------------------------------------------------------

export function InlineCell({ item, col, kind, label, edit, fields, children }: {
  item: Item;
  col: string;
  kind: InlineKind;
  label: string;
  edit: InlineEdit;
  fields: ReadonlyMap<string, FieldDefinition>;
  children: ReactNode;
}) {
  const [editing, setEditing] = useState(false);
  const button = useRef<HTMLButtonElement>(null);
  const toast = useToast();
  const wasEditing = useRef(false);
  useEffect(() => {
    // Back to the cell after saving or cancelling, so the keyboard stays put.
    if (wasEditing.current && !editing) button.current?.focus();
    wasEditing.current = editing;
  }, [editing]);

  if (kind === "boolean") return <BooleanToggle item={item} col={col} label={label} edit={edit} />;

  if (!editing) {
    return (
      <button
        ref={button}
        type="button"
        onClick={() => { setEditing(true); }}
        className="-mx-1 flex h-8 w-[calc(100%+0.5rem)] min-w-0 items-center rounded px-1 text-left hover:ring-1 hover:ring-zinc-300 focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:outline-none dark:hover:ring-zinc-700"
      >
        {children ?? <span aria-hidden className="text-zinc-300 dark:text-zinc-600">—</span>}
        {/* The visible value stays the start of the name (WCAG 2.5.3), the
            action follows: "In Progress, QA-1: Status dəyiş". */}
        <span className="sr-only">{children ? ", " : ""}{tc.edit(label, item.key)}</span>
      </button>
    );
  }

  const done = () => { setEditing(false); };
  const key = isFieldColumn(col) ? fieldKey(col) : "";
  const def = key ? fields.get(key) : undefined;
  const aria = tc.edit(label, item.key);

  switch (kind) {
    case "status":
      return (
        <StatusEditor
          item={item}
          ws={edit.ws}
          project={edit.project}
          aria={aria}
          onDone={done}
          onCommit={(st) => {
            edit.commit(item, { status_id: st.id }, { ...item, status: st, overdue: overdueOn({ status: st }, item.due_date, edit.today) });
          }}
        />
      );
    case "assignee":
    case "user": {
      const current = kind === "assignee" ? (item.assignee_id ?? "") : draftOf(item.fields[key]);
      return (
        <ChoiceEditor
          aria={aria}
          value={current}
          options={[{ value: "", label: kind === "assignee" ? t.unassigned : "—" }, ...edit.assignable.map((m) => ({ value: m.user_id, label: m.display_name }))]}
          onDone={done}
          onCommit={(v) => {
            if (kind === "assignee") edit.commit(item, { assignee_id: v || null }, { ...item, assignee_id: v || null });
            else edit.commit(item, { fields: { [key]: v || null } }, { ...item, fields: { ...item.fields, [key]: v || null } });
          }}
        />
      );
    }
    case "select": {
      const value = draftOf(item.fields[key]);
      const choices = (def?.options.choices ?? []).filter((c) => !c.archived || c.id === value);
      return (
        <ChoiceEditor
          aria={aria}
          value={value}
          options={[{ value: "", label: "—" }, ...choices.map((c) => ({ value: c.id, label: c.label }))]}
          onDone={done}
          onCommit={(v) => { edit.commit(item, { fields: { [key]: v || null } }, { ...item, fields: { ...item.fields, [key]: v || null } }); }}
        />
      );
    }
    case "due_date":
      return (
        <TextEditor
          aria={aria}
          type="date"
          initial={item.due_date ?? ""}
          onDone={done}
          onCommit={(s) => {
            const due = s.trim() || null;
            edit.commit(item, { due_date: due }, { ...item, due_date: due, overdue: overdueOn(item, due, edit.today) });
          }}
        />
      );
    case "money":
      if (!def) return null;
      return (
        <MoneyEditor
          def={def}
          value={item.fields[key]}
          label={label}
          compact
          autoFocus
          onCancel={done}
          onInvalid={(msg) => { toast("error", msg); }}
          onCommit={(v) => { edit.commit(item, { fields: { [key]: v } }, { ...item, fields: { ...item.fields, [key]: v } }); }}
        />
      );
    default: {
      const type = kind === "number" ? "text" : kind; // text input for numbers: "1,5" is accepted
      return (
        <TextEditor
          aria={aria}
          type={type}
          inputMode={kind === "number" ? "decimal" : undefined}
          initial={draftOf(item.fields[key])}
          onDone={done}
          onCommit={(s) => {
            const v = parseDraft(kind, s);
            if (v === undefined) {
              // Not a number: the old value stays, and the reader is told why.
              toast("error", tc.invalidNumber(label));
              return;
            }
            edit.commit(item, { fields: { [key]: v } }, { ...item, fields: { ...item.fields, [key]: v } });
          }}
        />
      );
    }
  }
}

/** Escape cancels without closing anything around the table. */
function onEscape(e: KeyboardEvent, cancel: () => void) {
  if (e.key === "Escape") {
    e.preventDefault();
    e.stopPropagation();
    cancel();
    return true;
  }
  return false;
}

function TextEditor({ aria, type, inputMode, initial, onCommit, onDone }: {
  aria: string; type: string; inputMode?: "decimal" | undefined; initial: string; onCommit: (s: string) => void; onDone: () => void;
}) {
  const [draft, setDraft] = useState(initial);
  const settled = useRef(false);
  const finish = (save: boolean) => {
    if (settled.current) return;
    settled.current = true;
    if (save && draft.trim() !== initial.trim()) onCommit(draft);
    onDone();
  };
  return (
    <Input
      autoFocus
      aria-label={aria}
      type={type}
      inputMode={inputMode}
      value={draft}
      className="h-8 py-0"
      onChange={(e) => { setDraft(e.target.value); }}
      onBlur={() => { finish(true); }}
      onKeyDown={(e) => {
        if (onEscape(e, () => { finish(false); })) return;
        if (e.key === "Enter") {
          e.preventDefault();
          finish(true);
        }
      }}
    />
  );
}

function ChoiceEditor({ aria, value, options, onCommit, onDone }: {
  aria: string; value: string; options: { value: string; label: string }[]; onCommit: (v: string) => void; onDone: () => void;
}) {
  const [draft, setDraft] = useState(value);
  const settled = useRef(false);
  // Arrow keys change a closed <select> in some browsers: a keyboard
  // change waits for Enter or leaving the cell; a pick with the mouse saves.
  const byKeyboard = useRef(false);
  const finish = (save: boolean, v = draft) => {
    if (settled.current) return;
    settled.current = true;
    if (save && v !== value) onCommit(v);
    onDone();
  };
  return (
    <Select
      autoFocus
      aria-label={aria}
      value={draft}
      className="h-8 py-0"
      onKeyDown={(e) => {
        if (onEscape(e, () => { finish(false); })) return;
        if (e.key === "Enter") {
          e.preventDefault();
          finish(true);
          return;
        }
        byKeyboard.current = true;
      }}
      onPointerDown={() => { byKeyboard.current = false; }}
      onChange={(e) => {
        setDraft(e.target.value);
        if (!byKeyboard.current) finish(true, e.target.value);
      }}
      onBlur={() => { finish(true); }}
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>{o.label}</option>
      ))}
    </Select>
  );
}

/** Offers the item's current status and the ones the workflow allows next. */
function StatusEditor({ item, ws, project, aria, onCommit, onDone }: {
  item: Item; ws: string; project: string; aria: string; onCommit: (s: Status) => void; onDone: () => void;
}) {
  const allowed = useTransitions(ws, project, item.number);
  const list = [item.status, ...(allowed.data ?? []).filter((s) => s.id !== item.status.id)];
  return (
    <ChoiceEditor
      aria={aria}
      value={item.status.id}
      options={list.map((s) => ({ value: s.id, label: s.name }))}
      onDone={onDone}
      onCommit={(id) => {
        const st = list.find((s) => s.id === id);
        if (st) onCommit(st);
      }}
    />
  );
}

/**
 * A yes/no cell toggles on one click (ADR-0050): a native checkbox role, the
 * value as a green check (yes), an empty ring (no) or a dash (not set), and
 * a toast with "Geri qaytar". Undo writes the previous value back with the
 * next version (the toggle itself is one change).
 */
function BooleanToggle({ item, col, label, edit }: { item: Item; col: string; label: string; edit: InlineEdit }) {
  const toast = useToast();
  const key = fieldKey(col);
  const raw = item.fields[key];
  const on = raw === true;
  const set = (base: Item, value: boolean | null) => {
    edit.commit(base, { fields: { [key]: value } }, { ...base, fields: { ...base.fields, [key]: value } });
  };
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={on}
      aria-label={`${label}, ${item.key}`}
      title={tc.edit(label, item.key)}
      onClick={() => {
        const prev = typeof raw === "boolean" ? raw : null;
        set(item, !on);
        toast("success", tc.toggled(label, !on ? t.yes : t.no, item.key), {
          label: tc.undo,
          run: () => { set({ ...item, version: item.version + 1, fields: { ...item.fields, [key]: !on } }, prev); },
        });
      }}
      className="mx-auto flex size-6 items-center justify-center rounded-full hover:ring-1 hover:ring-zinc-400 focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:outline-none"
    >
      {on ? (
        <span aria-hidden className="inline-flex size-4 items-center justify-center rounded-full bg-emerald-600 text-2xs leading-none font-bold text-white dark:bg-emerald-500 dark:text-emerald-950">✓</span>
      ) : raw === false ? (
        <span aria-hidden className="size-4 rounded-full ring-1 ring-zinc-400 ring-inset dark:ring-zinc-500" />
      ) : (
        <span aria-hidden className="text-zinc-400 dark:text-zinc-500">–</span>
      )}
    </button>
  );
}
