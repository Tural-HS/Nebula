import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

import type { ItemSort } from "../api/hooks";
import type { FieldDefinition } from "../api/types";
import { t } from "../i18n";
import { tv } from "../i18n-views";
import { cn } from "../lib/cn";
import { containsValue, filterKind, parseFilter, rangeValue } from "../lib/columnFilter";
import { Button, Input, Label, Select } from "./ui";

// A filter or sort reloads the list, which can remount the table (loading
// state): the column whose menu was used takes the focus back when it mounts.
let refocus: string | null = null;

/**
 * Filter and sort for one table column (ADR-0048), from a small button in the
 * column header. The panel is portalled to <body> with fixed position: the
 * header row clips its overflow (column lines) and scrolls sideways.
 */
export function ColumnMenu({ def, label, value, onFilter, sort, onSort }: {
  def: FieldDefinition;
  label: string;
  value: string | undefined;
  /** "" clears the column's filter. */
  onFilter: (value: string) => void;
  /** Present when the column can be sorted. */
  sort?: ItemSort | undefined;
  onSort?: ((s: ItemSort) => void) | undefined;
}) {
  const kind = filterKind(def);
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  const button = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const id = useId();
  const active = Boolean(value);
  const sorted = sort === `field.${def.key}` ? "asc" : sort === `-field.${def.key}` ? "desc" : null;

  // On mount only: a remounted table gives the focus back to this column.
  useEffect(() => {
    if (refocus === def.key) {
      refocus = null;
      button.current?.focus();
    }
  }, [def.key]);

  useLayoutEffect(() => {
    if (!open || !button.current) return;
    const r = button.current.getBoundingClientRect();
    const width = 288; // w-72
    setPos({ top: r.bottom + 4, left: Math.max(8, Math.min(r.left, window.innerWidth - width - 8)) });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => {
      if (e instanceof KeyboardEvent) {
        if (e.key !== "Escape") return;
        setOpen(false);
        button.current?.focus(); // WCAG 2.4.3: back to the toggle
        return;
      }
      const target = e.target as Node;
      if (panel.current?.contains(target) || button.current?.contains(target)) return;
      setOpen(false);
    };
    // The page or the table scrolled: the anchor moved, close instead of drifting.
    const onScroll = (e: Event) => {
      if (!panel.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    document.addEventListener("keydown", close);
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onScroll);
    return () => {
      document.removeEventListener("pointerdown", close);
      document.removeEventListener("keydown", close);
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onScroll);
    };
  }, [open]);

  if (!kind && !onSort) return null;
  const done = () => {
    setOpen(false);
    refocus = def.key;
    // If the table does not remount, forget the request soon.
    window.setTimeout(() => { if (refocus === def.key) refocus = null; }, 3000);
    button.current?.focus();
  };
  const apply = (v: string) => {
    onFilter(v);
    done();
  };

  return (
    <>
      <button
        ref={button}
        type="button"
        aria-label={tv.columnMenu(label) + (active ? `, ${tv.filterOn}` : "")}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        title={tv.columnMenu(label)}
        onClick={(e) => {
          e.stopPropagation();
          setOpen((o) => !o);
        }}
        className={cn(
          "inline-flex size-5 shrink-0 items-center justify-center rounded text-2xs leading-none hover:bg-zinc-200 dark:hover:bg-zinc-700",
          active || sorted ? "bg-brand-600 text-white hover:bg-brand-700 dark:bg-brand-500 dark:text-zinc-950" : "text-zinc-600 dark:text-zinc-300",
        )}
      >
        <span aria-hidden>{sorted === "asc" ? "↑" : sorted === "desc" ? "↓" : active ? "⏷" : "▾"}</span>
      </button>
      {open && pos && createPortal(
        <div
          ref={panel}
          id={id}
          role="dialog"
          aria-label={tv.columnMenu(label)}
          style={{ top: pos.top, left: pos.left }}
          className="fixed z-50 w-72 max-w-[calc(100vw-1rem)] space-y-3 rounded-lg bg-white p-3 text-sm font-normal text-zinc-900 shadow-lg ring-1 ring-zinc-300 dark:bg-zinc-900 dark:text-zinc-100 dark:ring-zinc-700"
        >
          <p className="font-semibold">{label}</p>
          {onSort && (
            <div className="flex gap-2">
              <Button className="flex-1" variant={sorted === "asc" ? "primary" : "secondary"} onClick={() => { onSort(`field.${def.key}` as ItemSort); done(); }}>↑ {tv.sortAscShort}</Button>
              <Button className="flex-1" variant={sorted === "desc" ? "primary" : "secondary"} onClick={() => { onSort(`-field.${def.key}` as ItemSort); done(); }}>↓ {tv.sortDescShort}</Button>
            </div>
          )}
          {kind && <FilterEditor def={def} kind={kind} value={value} onApply={apply} />}
        </div>,
        document.body,
      )}
    </>
  );
}

function FilterEditor({ def, kind, value, onApply }: {
  def: FieldDefinition;
  kind: NonNullable<ReturnType<typeof filterKind>>;
  value: string | undefined;
  onApply: (v: string) => void;
}) {
  const p = parseFilter(value);
  const [contains, setContains] = useState(p.contains);
  const [from, setFrom] = useState(p.from);
  const [to, setTo] = useState(p.to);
  const base = useId();
  const clear = value ? <Button variant="ghost" onClick={() => { onApply(""); }}>{tv.clearColumnFilter}</Button> : null;

  if (kind === "choice" || kind === "boolean" || kind === "money") {
    return (
      <div className="space-y-2">
        <Label htmlFor={`${base}-v`}>{tv.fieldFilter(def.name)}</Label>
        <Select id={`${base}-v`} value={p.exact} onChange={(e) => { onApply(e.target.value); }}>
          <option value="">{tv.anyValue}</option>
          {kind === "boolean" ? (
            <>
              <option value="true">{t.yes}</option>
              <option value="false">{t.no}</option>
            </>
          ) : kind === "money" ? (
            (def.options.money?.currencies ?? []).map((c) => <option key={c} value={c}>{c}</option>)
          ) : (
            (def.options.choices ?? []).map((c) => <option key={c.id} value={c.id}>{c.label}</option>)
          )}
        </Select>
      </div>
    );
  }
  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault();
        onApply(kind === "contains" ? containsValue(contains) : rangeValue(from, to));
      }}
    >
      {kind === "contains" ? (
        <>
          <Label htmlFor={`${base}-c`}>{tv.containsLabel}</Label>
          <Input id={`${base}-c`} autoFocus maxLength={100} value={contains} placeholder={tv.containsPlaceholder} onChange={(e) => { setContains(e.target.value); }} />
        </>
      ) : (
        <div className="grid grid-cols-2 gap-2">
          <div>
            <Label htmlFor={`${base}-f`}>{tv.rangeFrom}</Label>
            <Input id={`${base}-f`} autoFocus type={kind === "date" ? "date" : "number"} step="any" value={from} onChange={(e) => { setFrom(e.target.value); }} />
          </div>
          <div>
            <Label htmlFor={`${base}-t`}>{tv.rangeTo}</Label>
            <Input id={`${base}-t`} type={kind === "date" ? "date" : "number"} step="any" value={to} onChange={(e) => { setTo(e.target.value); }} />
          </div>
        </div>
      )}
      <div className="flex items-center justify-between gap-2">
        {clear ?? <span />}
        <Button type="submit" variant="primary">{tv.applyFilter}</Button>
      </div>
    </form>
  );
}
