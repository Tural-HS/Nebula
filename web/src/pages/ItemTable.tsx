import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useMemo, useRef, type CSSProperties, type ReactNode } from "react";

import type { FieldDefinition, FieldSums, Item, Member } from "../api/types";
import { SignalCell, StatusText } from "../components/Signals";
import { tc } from "../i18n-calendar";
import { inlineKind, type InlineEdit } from "../lib/inline";
import { InlineCell } from "./InlineCell";
import { Avatar, DueChip } from "../components/ui";
import { t } from "../i18n";
import { tv } from "../i18n-views";
import { cn } from "../lib/cn";
import { formatDateTime, formatRelative } from "../lib/format";
import { displayColumns, FINE_POINTER, useCompact, useMediaQuery } from "../lib/responsive";
import { columnHeader, fieldKey, formatDecimal, formatFieldValue, isFieldColumn } from "../lib/views";
import { isFormula, isNumeric } from "../lib/formula";
import { tf } from "../i18n-formulas";
import { tlg } from "../i18n-ledger";
import { ROW_STRIPE, rowSignal } from "../lib/queues";

// ---------------------------------------------------------------------------
// Table: virtualized, so only visible rows are in the DOM (100k rows stay
// smooth). Columns come from the page state (built-ins and custom fields);
// one scroll container moves both ways, with a sticky header, key column and
// totals row, so many columns stay usable on a phone. On narrow screens the
// title is pinned instead, with the key folded into it (lib/responsive).
// ---------------------------------------------------------------------------

// Dense rows for a mouse (ADR-0035); touch keeps 44 px targets (ADR-0019).
const ROW_HEIGHT = 36;
const TOUCH_ROW_HEIGHT = 44;
/** Not a user column: always right after the title (ADR-0035). */
const SIGNALS = "signals";
const ROW = "grid items-center gap-3 px-3 md:px-4";
// Sticky cells need an opaque background: the page's.
const STICKY_LEFT = "sticky left-0 z-[1] bg-zinc-50 dark:bg-zinc-950";
// On phones the pinned title gets a hairline so scrolled cells do not
// seem to run into it.
const STICKY_EDGE = "max-md:shadow-[1px_0_0_var(--color-zinc-200)] max-md:dark:shadow-[1px_0_0_var(--color-zinc-800)]";

/** Grid track and minimum width (rem) of a column. */
function track(col: string, def: FieldDefinition | undefined, compact: boolean): [string, number] {
  switch (col) {
    case "key":
      return ["6.5rem", 6.5];
    case SIGNALS:
      return ["7.25rem", 7.25];
    case "title":
      // Pinned on phones: leave about half of a 375 px screen for the rest.
      return compact ? ["minmax(10.5rem,1fr)", 10.5] : ["minmax(14rem,1fr)", 14];
    case "type":
      return ["7rem", 7];
    case "status":
      return ["9rem", 9];
    case "assignee":
      return ["11rem", 11];
    case "due_date":
    case "created_at":
    case "updated_at":
      return ["8rem", 8];
  }
  switch (def?.type) {
    case "number":
    case "date":
    case "boolean":
      return ["8rem", 8];
    case "long_text":
    case "multi_select":
      return ["14rem", 14];
    default:
      return ["11rem", 11];
  }
}

/** Row selection for bulk edits (ADR-0023); absent on phones. */
export interface TableSelection {
  selected: ReadonlySet<number>;
  toggle: (n: number) => void;
  /** "all" when every loaded row is selected, "some", or "none". */
  state: "all" | "some" | "none";
  toggleAll: () => void;
}

export function ItemTable({ items, columns: chosen, fields, members, sums, hasMore, loadingMore, loadMore, onOpen, selection, inline }: {
  items: Item[];
  columns: string[];
  fields: ReadonlyMap<string, FieldDefinition>;
  members: Map<string, Member>;
  /** Totals of the number columns; undefined while loading or when there are none. */
  sums: FieldSums | undefined;
  hasMore: boolean;
  loadingMore: boolean;
  loadMore: () => void;
  onOpen: (n: number) => void;
  selection?: TableSelection | undefined;
  /** Inline cell editing; absent for read-only users. */
  inline?: InlineEdit | undefined;
}) {
  const compact = useCompact();
  // Touch screens keep one tap target per row (ADR-0019): no checkboxes,
  // no inline editors; the item panel edits there. A mouse or trackpad
  // (also on a touch laptop) gets both.
  const fine = useMediaQuery(FINE_POINTER);
  const plainRows = compact || !fine;
  const select = plainRows ? undefined : selection;
  const edit = plainRows ? undefined : inline;
  const { columns, keyInTitle } = useMemo(() => {
    const d = displayColumns(chosen, compact);
    // Phones: the signals ride on the key line of the pinned title column, so
    // they are never scrolled out of view; no separate column.
    if (compact && d.keyInTitle) return d;
    const at = d.columns.indexOf("title");
    return { ...d, columns: [...d.columns.slice(0, at + 1), SIGNALS, ...d.columns.slice(at + 1)] };
  }, [chosen, compact]);
  const rowHeight = plainRows ? TOUCH_ROW_HEIGHT : ROW_HEIGHT;
  const scrollRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Virtual is designed for this usage
  const virtualizer = useVirtualizer({
    count: hasMore ? items.length + 1 : items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    overscan: 12,
  });
  const rows = virtualizer.getVirtualItems();
  const lastIndex = rows.at(-1)?.index ?? 0;

  useEffect(() => {
    if (hasMore && !loadingMore && lastIndex >= items.length - 1) loadMore();
  }, [hasMore, loadingMore, lastIndex, items.length, loadMore]);

  const grid = useMemo<CSSProperties>(() => {
    const tracks = columns.map((c): [string, number] =>
      c === SIGNALS && !columns.includes("due_date") ? ["14.5rem", 14.5] : track(c, isFieldColumn(c) ? fields.get(fieldKey(c)) : undefined, compact));
    // The first column also holds the row checkbox: 2 rem wider.
    const first = tracks[0];
    if (select && first) {
      const [g, w] = first;
      tracks[0] = [g.startsWith("minmax(") ? g.replace(/^minmax\(([\d.]+)rem/, (_m, n: string) => `minmax(${String(Number(n) + 2)}rem`) : `${String(w + 2)}rem`, w + 2];
    }
    const rem = tracks.reduce((n, [, w]) => n + w, 0) + 0.75 * (tracks.length - 1) + 2; // gaps and padding
    return { gridTemplateColumns: tracks.map(([g]) => g).join(" "), minWidth: `${String(rem)}rem` };
  }, [columns, fields, compact, select]);
  const header = (c: string) => (c === SIGNALS ? tlg.signals : columnHeader(c, fields));
  const numeric = (c: string) => isFieldColumn(c) && isNumeric(fields.get(fieldKey(c)));
  // Formula columns are computed and read-only (ADR-0030): marked with ƒ.
  const formulaDef = (c: string) => {
    const def = isFieldColumn(c) ? fields.get(fieldKey(c)) : undefined;
    return isFormula(def) ? def : undefined;
  };
  const showDueInTitle = !columns.includes("due_date");
  const hasSums = columns.some(numeric);
  const labelAt = Math.max(0, columns.findIndex((c) => !numeric(c)));

  const cell = (it: Item, col: string): ReactNode => {
    switch (col) {
      case "key":
        return <span className="text-xs text-zinc-500 tabular-nums">{it.key}</span>;
      case "title": {
        const title = (
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium">{it.title}</span>
          </span>
        );
        return keyInTitle ? (
          <span className="flex min-w-0 flex-col leading-tight">
            <span className="flex min-w-0 items-center gap-1.5 overflow-hidden text-[11px] text-zinc-500 tabular-nums">
              <span className="shrink-0">{it.key}</span>
              <SignalCell item={it} withDue inline /* the due column is off-screen on phones */ />
            </span>
            {title}
          </span>
        ) : title;
      }
      case SIGNALS:
        return <SignalCell item={it} withDue={showDueInTitle} />;
      case "type":
        return <span className="block truncate text-zinc-600 dark:text-zinc-400">{it.type.name}</span>;
      case "status":
        return <StatusText name={it.status.name} category={it.status.category} />;
      case "assignee":
        return (
          <span className="flex min-w-0 items-center gap-2">
            <AssigneeCell id={it.assignee_id} members={members} />
          </span>
        );
      case "due_date":
        return it.due_date ? <DueChip date={it.due_date} overdue={it.overdue} /> : null;
      case "created_at":
      case "updated_at": {
        const v = col === "created_at" ? it.created_at : it.updated_at;
        return <span className="text-xs text-zinc-500" title={formatDateTime(v)}>{formatRelative(v)}</span>;
      }
    }
    const def = fields.get(fieldKey(col));
    const text = formatFieldValue(def, it.fields[fieldKey(col)], members);
    const expr = isFormula(def) ? def?.options.formula?.expression : undefined;
    return (
      <span
        className={cn("block min-w-0 flex-1 truncate", numeric(col) && "text-right tabular-nums", expr !== undefined && "text-zinc-700 dark:text-zinc-300")}
        title={expr !== undefined ? `${text}\n${tf.title(expr)}` : text}
        data-formula={expr !== undefined ? "" : undefined}
      >
        {text}
      </span>
    );
  };

  return (
    <div ref={scrollRef} className="h-full overflow-auto" role="table" aria-rowcount={items.length}>
      <div role="row" style={grid} className={cn(ROW, "sticky top-0 z-[2] h-8 border-b border-zinc-200 bg-zinc-50 text-xs font-medium text-zinc-500 dark:border-zinc-800 dark:bg-zinc-950")}>
        {columns.map((c, i) => (
          <span key={c} role="columnheader" title={c === SIGNALS ? tlg.signalsHint : undefined} className={cn("truncate", numeric(c) && "text-right", i === 0 && STICKY_LEFT, i === 0 && STICKY_EDGE, i === 0 && select && "flex items-center gap-3")}>
            {i === 0 && select && (
              <input
                type="checkbox"
                aria-label={tc.selectAll}
                className="size-4 accent-brand-600"
                checked={select.state === "all"}
                ref={(el) => { if (el) el.indeterminate = select.state === "some"; }}
                onChange={select.toggleAll}
              />
            )}
            {formulaDef(c) && <span className="mr-1 font-serif italic" title={`${tf.readOnly}\n${tf.title(formulaDef(c)?.options.formula?.expression ?? "")}`}>ƒ</span>}
            {c === "title" && keyInTitle ? `${columnHeader("key", fields)} / ${header(c)}` : header(c)}
          </span>
        ))}
      </div>
      <div style={{ height: virtualizer.getTotalSize(), position: "relative", minWidth: grid.minWidth }}>
        {rows.map((row) => {
          const it = items[row.index];
          const signal = it ? rowSignal(it) : null;
          return (
            <div
              key={row.key}
              role="row"
              style={{ position: "absolute", top: 0, left: 0, right: 0, height: row.size, transform: `translateY(${String(row.start)}px)` }}
            >
              {/* The row's most urgent signal on its left edge; sticky, so it stays while the row scrolls sideways. */}
              {signal && (
                <span aria-hidden className="pointer-events-none sticky left-0 z-[3] block size-0">
                  <span className={cn("absolute top-0 left-0 w-[3px]", ROW_STRIPE[signal])} style={{ height: row.size }} />
                </span>
              )}
              {it && plainRows ? (
                <button
                  type="button"
                  onClick={() => { onOpen(it.number); }}
                  style={grid}
                  className={cn(ROW, "h-full w-full border-b border-zinc-100 text-left text-sm tabular-nums hover:bg-zinc-100/70 dark:border-zinc-800/60 dark:hover:bg-zinc-800/60")}
                >
                  {columns.map((c, i) => (
                    <span key={c} role="cell" className={cn("min-w-0", i === 0 && STICKY_LEFT, i === 0 && STICKY_EDGE)}>
                      {cell(it, c)}
                    </span>
                  ))}
                </button>
              ) : it ? (
                // Desktop: the title opens the item (a button, so the
                // keyboard reaches it), editable cells edit in place, and a
                // click anywhere else on the row opens the item as well.
                <div
                  style={grid}
                  onClick={(e) => {
                    if (!(e.target as HTMLElement).closest("button, input, select, textarea, a, label")) onOpen(it.number);
                  }}
                  data-selected={select?.selected.has(it.number) || undefined}
                  className={cn(
                    ROW,
                    "h-full w-full cursor-pointer border-b border-zinc-100 text-left text-sm tabular-nums hover:bg-zinc-100/70 dark:border-zinc-800/60 dark:hover:bg-zinc-800/60",
                    select?.selected.has(it.number) && "bg-brand-50/70 dark:bg-brand-950/30",
                  )}
                >
                  {columns.map((c, i) => {
                    const kind = edit ? inlineKind(c, fields) : null;
                    return (
                      <span key={c} role="cell" className={cn("min-w-0", i === 0 && STICKY_LEFT, i === 0 && STICKY_EDGE, i === 0 && select && "flex items-center gap-3")}>
                        {i === 0 && select && (
                          <input
                            type="checkbox"
                            aria-label={tc.select(it.key)}
                            className="size-4 shrink-0 accent-brand-600"
                            checked={select.selected.has(it.number)}
                            onChange={() => { select.toggle(it.number); }}
                          />
                        )}
                        {c === "title" ? (
                          <button type="button" className="flex min-w-0 flex-1 text-left hover:underline" onClick={() => { onOpen(it.number); }}>
                            {cell(it, c)}
                          </button>
                        ) : kind && edit ? (
                          <InlineCell item={it} col={c} kind={kind} label={header(c)} edit={edit} fields={fields}>
                            {cell(it, c)}
                          </InlineCell>
                        ) : (
                          cell(it, c)
                        )}
                      </span>
                    );
                  })}
                </div>
              ) : (
                <div className="px-4 py-3 text-sm text-zinc-500">{t.loadingMore}</div>
              )}
            </div>
          );
        })}
      </div>
      {hasSums && (
        <div
          role="row"
          aria-label={tv.total}
          style={grid}
          className={cn(ROW, "sticky bottom-0 z-[2] h-10 border-t border-zinc-300 bg-zinc-100 text-sm font-medium dark:border-zinc-700 dark:bg-zinc-900")}
        >
          {columns.map((c, i) => {
            const s = numeric(c) ? sums?.[fieldKey(c)] : undefined;
            return (
              <span
                key={c}
                role="cell"
                className={cn("truncate", numeric(c) && "text-right tabular-nums", i === 0 && "sticky left-0 z-[1] bg-zinc-100 dark:bg-zinc-900", i === 0 && STICKY_EDGE)}
                title={s ? tv.totalHint(s.count) : undefined}
              >
                {numeric(c) ? (s ? formatDecimal(s.sum) : "…") : i === labelAt ? tv.total : ""}
              </span>
            );
          })}
        </div>
      )}
    </div>
  );
}

function AssigneeCell({ id, members }: { id: string | null; members: Map<string, Member> }) {
  if (!id) return <span className="text-zinc-400">{t.unassigned}</span>;
  const name = members.get(id)?.display_name ?? "…";
  return (
    <>
      <Avatar name={name} />
      <span className="truncate">{name}</span>
    </>
  );
}
