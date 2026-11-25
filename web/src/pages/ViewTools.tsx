import { useEffect, useRef, useState, type ReactNode } from "react";

import { ApiError } from "../api/client";
import type { FieldDefinition, SavedView, Status } from "../api/types";
import { useViewMutations } from "../api/views";
import { useToast } from "../components/toast-context";
import { Button, Dialog, FieldError, Input, Label, Select } from "../components/ui";
import { t } from "../i18n";
import { tv } from "../i18n-views";
import { cn } from "../lib/cn";
import { valueType } from "../lib/formula";
import { BUILTIN_COLUMNS, columnHeader, DEFAULT_COLUMNS, MAX_COLUMNS, moveColumn, viewFromState, type PageState } from "../lib/views";

/**
 * A small disclosure panel that closes on outside click and Escape. Inline,
 * it is a plain titled group instead (inside the phone filter sheet, where a
 * panel inside a panel would be hard to use).
 */
export function Popover({ label, children, align = "left", inline = false, summaryLabel }: {
  label: ReactNode; children: ReactNode; align?: "left" | "right"; inline?: boolean;
  /** Accessible name of the toggle when the label is only a symbol. */
  summaryLabel?: string;
}) {
  const ref = useRef<HTMLDetailsElement>(null);
  // Content is rendered only while open, so closed panels add no duplicate
  // labels ("Son tarix", "İcraçı") to the page.
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const close = (e: Event) => {
      const d = ref.current;
      if (!d?.open || !(e instanceof KeyboardEvent ? e.key === "Escape" : !d.contains(e.target as Node))) return;
      // Esc from inside: the content unmounts, so focus goes back to the toggle
      // instead of falling to <body> (WCAG 2.4.3).
      const inside = d.contains(document.activeElement);
      d.open = false;
      if (inside && e instanceof KeyboardEvent) d.querySelector("summary")?.focus();
    };
    document.addEventListener("pointerdown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("pointerdown", close);
      document.removeEventListener("keydown", close);
    };
  }, []);
  if (inline) {
    return (
      <section className="rounded-lg p-3 text-sm ring-1 ring-zinc-200 dark:ring-zinc-700">
        <h3 className="mb-2 font-medium">{label}</h3>
        {children}
      </section>
    );
  }
  return (
    <details ref={ref} className="relative" onToggle={(e) => { setOpen(e.currentTarget.open); }}>
      <summary aria-label={summaryLabel} className="inline-flex cursor-pointer list-none items-center justify-center gap-1 rounded-md pointer-coarse:min-h-11 pointer-coarse:min-w-11 bg-white px-3 py-1.5 text-sm font-medium whitespace-nowrap text-zinc-800 ring-1 ring-zinc-300 hover:bg-zinc-50 dark:bg-zinc-900 dark:text-zinc-100 dark:ring-zinc-700 dark:hover:bg-zinc-800 [&::-webkit-details-marker]:hidden">
        {label}
      </summary>
      {open && <div
        className={cn(
          "absolute z-20 mt-1 max-h-[70vh] w-72 max-w-[calc(100vw-2rem)] overflow-y-auto rounded-lg bg-white p-3 text-sm shadow-lg ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-700",
          align === "right" ? "right-0" : "left-0",
        )}
      >
        {children}
      </div>}
    </details>
  );
}

const errText = (err: unknown) => (err instanceof ApiError ? err.message : t.errorGeneric);

// ---------------------------------------------------------------------------
// View switcher: "Hamısı" plus personal and shared views; save, rename,
// share and delete where allowed (the server decides, can_edit mirrors it).
// ---------------------------------------------------------------------------

export function ViewSwitcher({ ws, project, views, active, dirty, state, isAdmin, onSelect, defaultViewId = null, onSetDefault }: {
  ws: string;
  project: string;
  views: SavedView[];
  active: SavedView | undefined;
  dirty: boolean;
  state: PageState;
  isAdmin: boolean;
  onSelect: (v: SavedView | null) => void;
  /** The project's default view (it opens with it) and, for admins, how to change it. */
  defaultViewId?: string | null;
  onSetDefault?: (id: string | null) => Promise<unknown>;
}) {
  const toast = useToast();
  const m = useViewMutations(ws, project);
  const [dialog, setDialog] = useState<"create" | "rename" | null>(null);
  const [name, setName] = useState("");
  const [shared, setShared] = useState(false);
  const personal = views.filter((v) => !v.shared);
  const common = views.filter((v) => v.shared);

  const submit = async () => {
    try {
      if (dialog === "create") {
        const v = await m.create.mutateAsync({ name, shared, ...viewFromState(state) });
        toast("success", tv.savedView(v.name));
        onSelect(v);
      } else if (dialog === "rename" && active) {
        await m.update.mutateAsync({ id: active.id, body: { name } });
      }
      setDialog(null);
    } catch {
      // shown in the dialog
    }
  };
  const run = async (p: Promise<unknown>, ok?: string) => {
    try {
      await p;
      if (ok) toast("success", ok);
    } catch (err) {
      toast("error", errText(err));
    }
  };

  return (
    <div className="flex shrink-0 items-center gap-2">
      <Select
        aria-label={tv.views}
        value={active?.id ?? ""}
        onChange={(e) => { onSelect(views.find((v) => v.id === e.target.value) ?? null); }}
        className="max-w-48"
      >
        <option value="">{tv.allItems}</option>
        {personal.length > 0 && (
          <optgroup label={tv.personal}>
            {personal.map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
          </optgroup>
        )}
        {common.length > 0 && (
          <optgroup label={tv.shared}>
            {common.map((v) => <option key={v.id} value={v.id}>{v.id === defaultViewId ? `${v.name} · ${tv.defaultMark}` : v.name}</option>)}
          </optgroup>
        )}
      </Select>
      {active && dirty && <span className="text-xs whitespace-nowrap text-amber-700 dark:text-amber-400">{tv.modified}</span>}
      {active?.can_edit && dirty && (
        <Button
          variant="primary"
          disabled={m.update.isPending}
          onClick={() => void run(m.update.mutateAsync({ id: active.id, body: viewFromState(state) }), t.saved)}
        >
          {tv.saveChanges}
        </Button>
      )}
      <Button
        title={tv.saveAs}
        onClick={() => {
          setName("");
          setShared(false);
          m.create.reset();
          setDialog("create");
        }}
      >
        {/* Tablet and small desktop: a "+" keeps the toolbar one row (ADR-0035); the name stays for screen readers. */}
        <span aria-hidden className="hidden md:max-xl:inline">＋</span>
        <span className="md:max-xl:sr-only">{tv.saveAs}</span>
      </Button>
      {active?.can_edit && (
        <Popover label={<span aria-hidden>⋯</span>} summaryLabel={tv.viewMenu}>
          <div className="flex flex-col items-stretch gap-1">
            <Button variant="ghost" className="justify-start" onClick={() => { setName(active.name); m.update.reset(); setDialog("rename"); }}>
              {tv.rename}
            </Button>
            {(active.shared || isAdmin) && (
              <Button
                variant="ghost"
                className="justify-start"
                onClick={() => void run(m.update.mutateAsync({ id: active.id, body: { shared: !active.shared } }), t.saved)}
              >
                {active.shared ? tv.unshare : tv.share}
              </Button>
            )}
            {active.shared && isAdmin && onSetDefault && (
              <Button
                variant="ghost"
                className="justify-start"
                onClick={() => void run(onSetDefault(active.id === defaultViewId ? null : active.id), t.saved)}
              >
                {active.id === defaultViewId ? tv.unsetDefault : tv.makeDefault}
              </Button>
            )}
            <Button
              variant="ghost"
              className="justify-start text-red-700 dark:text-red-400"
              onClick={() => {
                if (!window.confirm(tv.confirmDelete(active.name))) return;
                void run(m.remove.mutateAsync(active.id).then(() => { onSelect(null); }), t.deleted);
              }}
            >
              {tv.remove}
            </Button>
          </div>
        </Popover>
      )}

      <Dialog open={dialog !== null} onClose={() => { setDialog(null); }} title={dialog === "rename" ? tv.renameTitle : tv.newViewTitle}>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <Label htmlFor="view-name">{tv.viewName}</Label>
          <Input id="view-name" required maxLength={100} value={name} onChange={(e) => { setName(e.target.value); }} autoFocus />
          {dialog === "create" && isAdmin && (
            <label className="mt-3 flex items-start gap-2 text-sm">
              <input type="checkbox" className="mt-0.5" checked={shared} onChange={(e) => { setShared(e.target.checked); }} />
              <span>
                {tv.share}
                <span className="block text-xs text-zinc-500 dark:text-zinc-400">{tv.shareHint}</span>
              </span>
            </label>
          )}
          <FieldError error={dialog === "create" ? m.create.error : m.update.error} />
          <div className="mt-4 flex justify-end gap-2">
            <Button onClick={() => { setDialog(null); }}>{t.cancel}</Button>
            <Button type="submit" variant="primary" disabled={m.create.isPending || m.update.isPending || !name.trim()}>
              {t.save}
            </Button>
          </div>
        </form>
      </Dialog>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Column picker: built-ins and active custom fields, toggled and reordered.
// ---------------------------------------------------------------------------

export function ColumnPicker({ columns, fields, onChange, inline }: {
  columns: string[];
  fields: ReadonlyMap<string, FieldDefinition>;
  onChange: (columns: string[]) => void;
  inline?: boolean;
}) {
  const fieldCols = [...fields.values()].filter((f) => !f.archived).map((f) => "field." + f.key);
  const all = [...BUILTIN_COLUMNS, ...fieldCols];
  const hidden = all.filter((c) => !columns.includes(c));
  const toggle = (c: string, on: boolean) => {
    onChange(on ? [...columns, c] : columns.filter((x) => x !== c));
  };
  return (
    <Popover label={tv.columns} inline={inline}>
      <p className="mb-2 text-xs text-zinc-500 dark:text-zinc-400">{tv.columnsHint}</p>
      <ul className="flex flex-col gap-1">
        {[...columns.filter((c) => all.includes(c)), ...hidden].map((c) => {
          const on = columns.includes(c);
          const label = columnHeader(c, fields);
          return (
            <li key={c} className="flex items-center gap-2">
              <label className="flex min-w-0 flex-1 items-center gap-2 pointer-coarse:min-h-11">
                <input
                  type="checkbox"
                  checked={on}
                  disabled={c === "title" || (!on && columns.length >= MAX_COLUMNS)}
                  onChange={(e) => { toggle(c, e.target.checked); }}
                />
                <span className="truncate" title={c === "key" ? undefined : label}>{c === "key" ? "#" : label}</span>
              </label>
              {on && (
                <span className="flex shrink-0 gap-0.5">
                  <button type="button" className={MOVE_BUTTON} aria-label={`${label}: ${tv.moveLeft}`} onClick={() => { onChange(moveColumn(columns, c, -1)); }}>
                    ↑
                  </button>
                  <button type="button" className={MOVE_BUTTON} aria-label={`${label}: ${tv.moveRight}`} onClick={() => { onChange(moveColumn(columns, c, 1)); }}>
                    ↓
                  </button>
                </span>
              )}
            </li>
          );
        })}
      </ul>
      <Button variant="ghost" className="mt-2 w-full" onClick={() => { onChange([...DEFAULT_COLUMNS]); }}>
        {tv.resetColumns}
      </Button>
    </Popover>
  );
}

// ---------------------------------------------------------------------------
// More filters: statuses and select / boolean custom fields.
// ---------------------------------------------------------------------------

const MOVE_BUTTON = "rounded px-1.5 text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100 pointer-coarse:min-h-11 pointer-coarse:min-w-11 dark:hover:bg-zinc-800";

export function FilterPicker({ statuses, fields, status, values, onStatus, onField, onClear, inline }: {
  statuses: Status[];
  fields: ReadonlyMap<string, FieldDefinition>;
  status: string[];
  values: Record<string, string>;
  onStatus: (ids: string[]) => void;
  onField: (key: string, value: string) => void;
  onClear: () => void;
  inline?: boolean;
}) {
  const filterable = [...fields.values()].filter((f) => !f.archived && (f.type === "select" || f.type === "multi_select" || f.type === "money" || valueType(f) === "boolean"));
  const active = status.length + Object.keys(values).length;
  return (
    <Popover label={inline ? tv.statusFilter : active > 0 ? tv.activeFilters(active) : tv.filters} inline={inline}>
      <fieldset>
        <legend className={cn("mb-1 text-xs font-medium text-zinc-600 dark:text-zinc-400", inline && "sr-only")}>{tv.statusFilter}</legend>
        <ul className="flex flex-col gap-1">
          {statuses.map((s) => (
            <li key={s.id}>
              <label className="flex items-center gap-2 pointer-coarse:min-h-11">
                <input
                  type="checkbox"
                  checked={status.includes(s.id)}
                  onChange={(e) => { onStatus(e.target.checked ? [...status, s.id] : status.filter((x) => x !== s.id)); }}
                />
                {s.name}
              </label>
            </li>
          ))}
        </ul>
      </fieldset>
      {filterable.map((f) => (
        <div key={f.key} className="mt-3">
          <Label htmlFor={`filter-${f.key}`}>{tv.fieldFilter(f.name)}</Label>
          <Select id={`filter-${f.key}`} value={values[f.key] ?? ""} onChange={(e) => { onField(f.key, e.target.value); }}>
            <option value="">{tv.anyValue}</option>
            {valueType(f) === "boolean" ? (
              <>
                <option value="true">{t.yes}</option>
                <option value="false">{t.no}</option>
              </>
            ) : f.type === "money" ? (
              // Money filters by currency (ADR-0044).
              (f.options.money?.currencies ?? []).map((c) => <option key={c} value={c}>{c}</option>)
            ) : (
              (f.options.choices ?? []).map((c) => <option key={c.id} value={c.id}>{c.label}</option>)
            )}
          </Select>
        </div>
      ))}
      {active > 0 && (
        <Button variant="ghost" className="mt-3 w-full" onClick={onClear}>
          {tv.clearFilters}
        </Button>
      )}
    </Popover>
  );
}
