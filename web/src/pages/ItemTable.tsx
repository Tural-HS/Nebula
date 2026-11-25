import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useMemo, useRef, type CSSProperties, type ReactNode } from "react";

import type { FieldDefinition, FieldSums, Item, Member } from "../api/types";
import { SignalCell, StatusText } from "../components/Signals";
import { hasSignal } from "../lib/signals";
import { isPhoneField, telHref } from "../lib/phone";
import { tc } from "../i18n-calendar";
import { inlineKind, type InlineEdit } from "../lib/inline";
import { InlineCell } from "./InlineCell";
import { Avatar, DueChip } from "../components/ui";
import { t } from "../i18n";
import { tv } from "../i18n-views";
import { cn } from "../lib/cn";
import { formatDateTime, formatRelative } from "../lib/format";
import { displayColumns, FINE_POINTER, useCompact, useMediaQuery } from "../lib/responsive";
import { columnHeader, fieldKey, formatDecimal, formatFieldValue, isFieldColumn, withUnit } from "../lib/views";
import { isAmount, isFormula, isSortable, valueType } from "../lib/formula";
import { filterKind } from "../lib/columnFilter";
import type { Density } from "../lib/density";
import { ColumnMenu } from "../components/ColumnMenu";
import type { ItemSort } from "../api/hooks";
import { formatMoneyTotals } from "../lib/money";
import { tm } from "../i18n-money";
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
const ROW_HEIGHT = 28; // dense, like a spreadsheet (ADR-0047)
const COMPACT_ROW_HEIGHT = 24; // "Sıx" density (ADR-0048)
const TOUCH_ROW_HEIGHT = 44;
/** Not a user column: always right after the title (ADR-0035). */
const SIGNALS = "signals";
// overflow-clip (not hidden): the column lines below are cut at the row edge
// without making the row a scroll container, which would break sticky cells.
const ROW = "grid items-center gap-1.5 overflow-clip px-2";
// A column line in the middle of the gap left of a cell (ADR-0047): spreadsheet-
// like ledgers need columns that read as columns. Taller than any row, clipped.
const COL_LINE = "relative before:pointer-events-none before:absolute before:-inset-y-10 before:-left-[3px] before:w-px before:bg-zinc-400 dark:before:bg-zinc-600";
// The pinned title starts in the gap (TITLE_PIN bleed): its line sits inside it.
const COL_LINE_PINNED = "relative before:pointer-events-none before:absolute before:-inset-y-10 before:left-[3px] before:w-px before:bg-zinc-400 dark:before:bg-zinc-600";
// Sticky cells need an opaque background: the page's.
const STICKY_LEFT = "sticky left-0 z-[1] bg-zinc-100 group-hover:bg-brand-50 dark:bg-zinc-950 dark:group-hover:bg-zinc-900";
const PINNED_FILL = "flex items-center self-stretch";
const STICKY_LEFT_HEAD = "sticky left-0 z-[1] bg-brand-50 dark:bg-zinc-900";
// On phones the pinned title gets a hairline so scrolled cells do not
// seem to run into it.
const STICKY_EDGE = "max-md:shadow-[1px_0_0_var(--color-zinc-200)] max-md:dark:shadow-[1px_0_0_var(--color-zinc-800)]";
// The pinned title beside the key (wide tables on desktop): the hairline on
// every screen, and a left bleed over the grid gap so cells never show through.
const TITLE_PIN = "-ml-1.5 pl-1.5 shadow-[1px_0_0_var(--color-zinc-400)] dark:shadow-[1px_0_0_var(--color-zinc-600)]";
const STICKY_LEFT_TOTAL = "sticky left-0 z-[1] bg-zinc-200 dark:bg-zinc-900";

/** Grid track and minimum width (rem) of a column. */
function track(col: string, def: FieldDefinition | undefined, compact: boolean): [string, number] {
  switch (col) {
    case "key":
      return ["4.5rem", 4.5];
    case SIGNALS:
      return ["10.5rem", 10.5]; // + the SLA slot (ADR-0045)
    case "title":
      // Pinned on phones: leave about half of a 375 px screen for the rest.
      // Desktop: wide enough for full person and company names (ledgers).
      return compact ? ["minmax(10.5rem,1fr)", 10.5] : ["minmax(14rem,1fr)", 14];
    case "type":
      return ["7rem", 7];
    case "status":
      return ["7rem", 7];
    case "assignee":
      return ["8.5rem", 8.5];
    case "due_date":
    case "created_at":
    case "updated_at":
      return ["6.5rem", 6.5];
  }
  switch (def?.type) {
    case "number":
      return ["5.5rem", 5.5];
    case "date":
      return ["6rem", 6];
    case "boolean":
      return ["5rem", 5]; // a check mark; the header (with its filter button) wraps to two lines
    case "text":
      return ["7.5rem", 7.5];
    case "select":
      return ["6.5rem", 6.5];
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

export function ItemTable({ items, columns: chosen, fields, members, sums, hasMore, loadingMore, loadMore, onOpen, selection, inline, filters, onFilter, sort, onSort, density = "normal" }: {
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
  /** Field filters of the list (?field.<key>=) and their setter: column header menus (ADR-0048). */
  filters?: Readonly<Record<string, string>> | undefined;
  onFilter?: ((key: string, value: string) => void) | undefined;
  sort?: ItemSort | undefined;
  onSort?: ((s: ItemSort) => void) | undefined;
  /** Row height and text size, chosen per browser (lib/density). */
  density?: Density;
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
    // No loaded row has a signal: no empty 10 rem column.
    const withDue = !d.columns.includes("due_date");
    if (!items.some((it) => hasSignal(it, withDue))) return d;
    const at = d.columns.indexOf("title");
    return { ...d, columns: [...d.columns.slice(0, at + 1), SIGNALS, ...d.columns.slice(at + 1)] };
  }, [chosen, compact, items]);
  const dense = density === "compact";
  const rowHeight = plainRows ? TOUCH_ROW_HEIGHT : dense ? COMPACT_ROW_HEIGHT : ROW_HEIGHT;
  const rowText = dense ? "text-xs" : "text-[0.8125rem]";
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

  const { grid, firstWidth } = useMemo(() => {
    const tracks = columns.map((c): [string, number] =>
      c === SIGNALS && !columns.includes("due_date") ? ["17.75rem", 17.75] : track(c, isFieldColumn(c) ? fields.get(fieldKey(c)) : undefined, compact));
    // The first column also holds the row checkbox: 2 rem wider.
    const first = tracks[0];
    if (select && first) {
      const [g, w] = first;
      tracks[0] = [g.startsWith("minmax(") ? g.replace(/^minmax\(([\d.]+)rem/, (_m, n: string) => `minmax(${String(Number(n) + 1.5)}rem`) : `${String(w + 1.5)}rem`, w + 1.5];
    }
    const rem = tracks.reduce((n, [, w]) => n + w, 0) + 0.375 * (tracks.length - 1) + 1; // gaps and padding
    const g: CSSProperties = { gridTemplateColumns: tracks.map(([t]) => t).join(" "), minWidth: `${String(rem)}rem` };
    return { grid: g, firstWidth: tracks[0]?.[1] ?? 0 };
  }, [columns, fields, compact, select]);
  // Desktop: the key and the title both stay while the table scrolls sideways
  // (phones already pin the title, which carries the key).
  const pinTitle = columns[0] === "key" && columns[1] === "title";
  const pin = (i: number, sticky: string = STICKY_LEFT): { className: string | false; style: CSSProperties | undefined } =>
    // Pinned cells fill the row height (self-stretch), so their background
    // covers taller cells (two-line headers) scrolling underneath.
    i === 0
      ? { className: cn(sticky, PINNED_FILL, !pinTitle && STICKY_EDGE), style: undefined }
      : pinTitle && i === 1
        ? { className: cn(sticky, PINNED_FILL, TITLE_PIN), style: { left: `${String(firstWidth)}rem` } }
        : { className: false, style: undefined };
  const line = (i: number) => (i === 0 ? false : pinTitle && i === 1 ? COL_LINE_PINNED : COL_LINE);
  const header = (c: string) => (c === SIGNALS ? tlg.signals : columnHeader(c, fields));
  // Number and money columns: right-aligned tabular figures with a total.
  const numeric = (c: string) => isFieldColumn(c) && isAmount(fields.get(fieldKey(c)));
  const isBoolCol = (c: string) => isFieldColumn(c) && valueType(fields.get(fieldKey(c))) === "boolean";
  const isMoneyCol = (c: string) => isFieldColumn(c) && fields.get(fieldKey(c))?.type === "money";
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
        return <span className="text-xs text-zinc-500 dark:text-zinc-400 tabular-nums">{it.key}</span>;
      case "title": {
        const title = (
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium" title={it.title}>{it.title}</span>
          </span>
        );
        return keyInTitle ? (
          <span className="flex min-w-0 flex-col leading-tight">
            <span className="flex min-w-0 items-center gap-1.5 overflow-hidden text-2xs text-zinc-500 dark:text-zinc-400 tabular-nums">
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
        return <span className="text-xs text-zinc-500 dark:text-zinc-400" title={formatDateTime(v)}>{formatRelative(v)}</span>;
      }
    }
    const def = fields.get(fieldKey(col));
    const raw = it.fields[fieldKey(col)];
    // Yes/no columns: a check mark, like the "ok" of the Boards tables; no is blank.
    if (valueType(def) === "boolean" && typeof raw === "boolean") {
      return raw ? (
        <span className="flex justify-center">
          <span role="img" aria-label={t.yes} title={t.yes} className="inline-flex size-4 items-center justify-center rounded-full bg-emerald-600 text-2xs leading-none font-bold text-white dark:bg-emerald-500 dark:text-emerald-950">✓</span>
        </span>
      ) : (
        <span className="sr-only">{t.no}</span>
      );
    }
    const text = formatFieldValue(def, raw, members);
    // Tap to call where cells do not edit in place (touch, read-only); a link
    // inside the inline editor's button would be invalid and unclickable.
    const tel = !edit && isPhoneField(def) ? telHref(raw) : null;
    if (tel) {
      return (
        <a href={tel} className="block min-w-0 truncate text-brand-700 tabular-nums hover:underline dark:text-brand-300" title={text}>
          {text}
        </a>
      );
    }
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
    <div ref={scrollRef} className="h-full scroll-pt-10 scroll-pb-10 overflow-auto" role="table" aria-rowcount={items.length}>
      <div role="row" style={grid} className={cn(ROW, "sticky top-0 z-[2] h-10 border-b border-zinc-400 bg-brand-50 text-2xs font-semibold text-zinc-600 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300")}>
        {columns.map((c, i) => {
          const def = isFieldColumn(c) ? fields.get(fieldKey(c)) : undefined;
          const menu = def && onFilter && (filterKind(def) || (onSort && isSortable(def))) ? def : undefined;
          const clamp = cn("line-clamp-2 leading-tight [overflow-wrap:break-word]", numeric(c) && "text-right", isBoolCol(c) && "text-center");
          return (
          <span key={c} role="columnheader" title={c === SIGNALS ? tlg.signalsHint : header(c)} style={pin(i, STICKY_LEFT_HEAD).style} className={cn(menu ? "flex items-center gap-1" : pin(i, STICKY_LEFT_HEAD).className ? "truncate" : clamp, pin(i, STICKY_LEFT_HEAD).className, line(i), i === 0 && select && "flex items-center gap-2")}>
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
            {menu && onFilter ? (
              <>
                <span className={cn("min-w-0 flex-1", clamp)}>{header(c)}</span>
                <ColumnMenu
                  def={menu}
                  label={header(c)}
                  value={filters?.[menu.key]}
                  onFilter={(v) => { onFilter(menu.key, v); }}
                  sort={sort}
                  onSort={onSort && isSortable(menu) ? onSort : undefined}
                />
              </>
            ) : c === "title" && keyInTitle ? `${columnHeader("key", fields)} / ${header(c)}` : header(c)}
          </span>
          );
        })}
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
                // Touch and compact rows: the cells stay cells (a button's
                // children are presentational, which flattened the table into
                // one long name). The title is the keyboard's button; a tap
                // anywhere else on the row opens the item too.
                <div
                  style={grid}
                  onClick={(e) => {
                    if (!(e.target as HTMLElement).closest("button, a")) onOpen(it.number);
                  }}
                  className={cn(ROW, rowText, "group h-full w-full cursor-pointer border-b border-zinc-300 text-left tabular-nums hover:bg-brand-50/80 dark:border-zinc-700 dark:hover:bg-zinc-800/60")}
                >
                  {columns.map((c, i) => (
                    <span key={c} role="cell" style={pin(i).style} className={cn("min-w-0", pin(i).className, line(i))}>
                      {c === "title" ? (
                        <button type="button" className="flex min-w-0 max-w-full text-left" onClick={() => { onOpen(it.number); }}>
                          {cell(it, c)}
                        </button>
                      ) : (
                        cell(it, c)
                      )}
                    </span>
                  ))}
                </div>
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
                    rowText,
                    "group h-full w-full cursor-pointer border-b border-zinc-300 text-left tabular-nums hover:bg-brand-50/80 dark:border-zinc-700 dark:hover:bg-zinc-800/60",
                    select?.selected.has(it.number) && "bg-brand-50/70 dark:bg-brand-950/30",
                  )}
                >
                  {columns.map((c, i) => {
                    const kind = edit ? inlineKind(c, fields) : null;
                    return (
                      <span key={c} role="cell" style={pin(i).style} className={cn("min-w-0", pin(i).className, line(i), i === 0 && select && "flex items-center gap-2")}>
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
                          <button type="button" className="flex w-full min-w-0 flex-1 text-left hover:underline" onClick={() => { onOpen(it.number); }}>
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
                <div className="px-4 py-3 text-sm text-zinc-500 dark:text-zinc-400">{t.loadingMore}</div>
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
          className={cn(ROW, "sticky bottom-0 z-[2] h-8 border-t border-zinc-300 bg-zinc-200 text-[0.8125rem] font-semibold dark:border-zinc-700 dark:bg-zinc-900")}
        >
          {columns.map((c, i) => {
            const s = numeric(c) ? sums?.[fieldKey(c)] : undefined;
            // Money: one total per currency, never added together (ADR-0044).
            const money = isMoneyCol(c) && s ? formatMoneyTotals(s.currencies ?? []) : undefined;
            return (
              <span
                key={c}
                role="cell"
                style={pin(i, STICKY_LEFT_TOTAL).style}
                className={cn("truncate", numeric(c) && "text-right tabular-nums", pin(i, STICKY_LEFT_TOTAL).className, line(i))}
                title={s ? (money !== undefined ? `${tm.totalsLabel(money)}\n${tm.noFx}\n${tv.totalHint(s.count)}` : tv.totalHint(s.count)) : undefined}
                data-money-total={money !== undefined ? "" : undefined}
              >
                {numeric(c) ? (s ? (money ?? withUnit(formatDecimal(s.sum ?? "0"), fields.get(fieldKey(c)))) : "…") : i === labelAt ? tv.total : ""}
              </span>
            );
          })}
        </div>
      )}
    </div>
  );
}

function AssigneeCell({ id, members }: { id: string | null; members: Map<string, Member> }) {
  if (!id) return <span className="text-zinc-500 dark:text-zinc-400">{t.unassigned}</span>;
  const name = members.get(id)?.display_name ?? "…";
  return (
    <>
      <Avatar name={name} />
      <span className="truncate">{name}</span>
    </>
  );
}
