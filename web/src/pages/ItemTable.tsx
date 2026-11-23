import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useMemo, useRef, type CSSProperties, type ReactNode } from "react";

import type { FieldDefinition, FieldSums, Item, Member } from "../api/types";
import { Avatar, DueChip, StatusBadge } from "../components/ui";
import { t } from "../i18n";
import { tv } from "../i18n-views";
import { cn } from "../lib/cn";
import { formatDateTime, formatRelative } from "../lib/format";
import { columnHeader, fieldKey, formatDecimal, formatFieldValue, isFieldColumn } from "../lib/views";

// ---------------------------------------------------------------------------
// Table: virtualized, so only visible rows are in the DOM (100k rows stay
// smooth). Columns come from the page state (built-ins and custom fields);
// one scroll container moves both ways, with a sticky header, key column and
// totals row, so many columns stay usable on a phone.
// ---------------------------------------------------------------------------

const ROW_HEIGHT = 44;
const ROW = "grid items-center gap-3 px-4";
// Sticky cells need an opaque background: the page's.
const STICKY_LEFT = "sticky left-0 z-[1] bg-zinc-50 dark:bg-zinc-950";

/** Grid track and minimum width (rem) of a column. */
function track(col: string, def: FieldDefinition | undefined): [string, number] {
  switch (col) {
    case "key":
      return ["6.5rem", 6.5];
    case "title":
      return ["minmax(14rem,1fr)", 14];
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

export function ItemTable({ items, columns, fields, members, sums, hasMore, loadingMore, loadMore, onOpen }: {
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
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Virtual is designed for this usage
  const virtualizer = useVirtualizer({
    count: hasMore ? items.length + 1 : items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 12,
  });
  const rows = virtualizer.getVirtualItems();
  const lastIndex = rows.at(-1)?.index ?? 0;

  useEffect(() => {
    if (hasMore && !loadingMore && lastIndex >= items.length - 1) loadMore();
  }, [hasMore, loadingMore, lastIndex, items.length, loadMore]);

  const grid = useMemo<CSSProperties>(() => {
    const tracks = columns.map((c) => track(c, isFieldColumn(c) ? fields.get(fieldKey(c)) : undefined));
    const rem = tracks.reduce((n, [, w]) => n + w, 0) + 0.75 * (tracks.length - 1) + 2; // gaps and padding
    return { gridTemplateColumns: tracks.map(([g]) => g).join(" "), minWidth: `${String(rem)}rem` };
  }, [columns, fields]);
  const numeric = (c: string) => isFieldColumn(c) && fields.get(fieldKey(c))?.type === "number";
  const showDueInTitle = !columns.includes("due_date");
  const hasSums = columns.some(numeric);
  const labelAt = Math.max(0, columns.findIndex((c) => !numeric(c)));

  const cell = (it: Item, col: string): ReactNode => {
    switch (col) {
      case "key":
        return <span className="font-mono text-xs text-zinc-500">{it.key}</span>;
      case "title":
        return (
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate">{it.title}</span>
            {showDueInTitle && it.due_date && <DueChip date={it.due_date} overdue={it.overdue} />}
          </span>
        );
      case "type":
        return <span className="block truncate text-zinc-600 dark:text-zinc-400">{it.type.name}</span>;
      case "status":
        return <StatusBadge name={it.status.name} category={it.status.category} />;
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
    return <span className={cn("block truncate", numeric(col) && "text-right tabular-nums")} title={text}>{text}</span>;
  };

  return (
    <div ref={scrollRef} className="h-full overflow-auto" role="table" aria-rowcount={items.length}>
      <div role="row" style={grid} className={cn(ROW, "sticky top-0 z-[2] h-9 border-b border-zinc-200 bg-zinc-50 text-xs font-medium tracking-wide text-zinc-500 uppercase dark:border-zinc-800 dark:bg-zinc-950")}>
        {columns.map((c, i) => (
          <span key={c} role="columnheader" className={cn("truncate", numeric(c) && "text-right", i === 0 && STICKY_LEFT)}>
            {columnHeader(c, fields)}
          </span>
        ))}
      </div>
      <div style={{ height: virtualizer.getTotalSize(), position: "relative", minWidth: grid.minWidth }}>
        {rows.map((row) => {
          const it = items[row.index];
          return (
            <div
              key={row.key}
              role="row"
              style={{ position: "absolute", top: 0, left: 0, right: 0, height: row.size, transform: `translateY(${String(row.start)}px)` }}
            >
              {it ? (
                <button
                  type="button"
                  onClick={() => { onOpen(it.number); }}
                  style={grid}
                  className={cn(ROW, "h-full w-full border-b border-zinc-100 text-left text-sm hover:bg-zinc-100/70 dark:border-zinc-800/60 dark:hover:bg-zinc-800/60")}
                >
                  {columns.map((c, i) => (
                    <span key={c} role="cell" className={cn("min-w-0", i === 0 && STICKY_LEFT)}>
                      {cell(it, c)}
                    </span>
                  ))}
                </button>
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
                className={cn("truncate", numeric(c) && "text-right tabular-nums", i === 0 && "sticky left-0 z-[1] bg-zinc-100 dark:bg-zinc-900")}
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
