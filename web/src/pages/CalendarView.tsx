import {
  DndContext, DragOverlay, KeyboardSensor, MouseSensor, pointerWithin, rectIntersection, TouchSensor, useDraggable, useDroppable, useSensor, useSensors,
  type CollisionDetection, type DragEndEvent, type DragStartEvent,
} from "@dnd-kit/core";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState, type HTMLAttributes, type Ref } from "react";

import { useOptimisticUpdate } from "../api/bulk";
import { useWorkCalendar } from "../api/calendar";
import { api, ApiError, path } from "../api/client";
import { keys, useItems, type ItemFilters } from "../api/hooks";
import type { Item, ItemPage, Member } from "../api/types";
import { useToast } from "../components/toast-context";
import { Avatar, Button, Dialog, ErrorState, Spinner } from "../components/ui";
import { t } from "../i18n";
import { tc } from "../i18n-calendar";
import { tw } from "../i18n-workdays";
import { LATE_CHIP, LATE_EDGE, lateDays, lateLevel, daysLate, localToday, type LateDay } from "../lib/overdue";
import { byDay, monthDays, monthGrid, monthOf, monthTitle, overdueOn, overflow, shiftMonth, weekday, type Month } from "../lib/calendar";
import { flattenPages } from "../lib/board";
import { cn } from "../lib/cn";
import { formatDate } from "../lib/format";
import { dayLookup, type DayInfo } from "../lib/workdays";

// ---------------------------------------------------------------------------
// Calendar (ADR-0023): the month's items by due date, Monday first, in the
// workspace's time zone. The visible weeks load with one ranged query
// (?due_from&due_to, sorted by due date) page by page up to a cap; items
// without a due date are listed beside the grid. Drag a card to another day
// (or onto the side list) to change or clear its due date. Phones get an
// agenda list of the month instead of the grid (ADR-0019).
// ---------------------------------------------------------------------------

const PAGE = 200;
/** Items one month loads at most (5 requests); more shows a hint to filter. */
const CALENDAR_CAP = 1000;
const PER_DAY = 3;
const NO_DUE = "nodue";

const collision: CollisionDetection = (args) => {
  const under = pointerWithin(args);
  return under.length > 0 ? under : rectIntersection(args);
};

function useRangeItems(ws: string, project: string, filters: ItemFilters, from: string, to: string) {
  const f: ItemFilters = { ...filters, sort: "due", dueFrom: from, dueTo: to };
  const q = useInfiniteQuery({
    // Under keys.items: every item mutation refreshes the calendar too.
    queryKey: [...keys.items(ws, project), "calendar", f],
    initialPageParam: null as string | null,
    queryFn: ({ pageParam }) => {
      const p = new URLSearchParams({ limit: String(PAGE), sort: "due", due_from: from, due_to: to });
      if (f.q.trim()) p.set("q", f.q.trim());
      if (f.assignee) p.set("assignee", f.assignee);
      if (f.due) p.set("due", f.due);
      if (f.status && f.status.length > 0) p.set("status", f.status.join(","));
      for (const [k, v] of Object.entries(f.fields ?? {}).sort(([a], [b]) => a.localeCompare(b))) p.set("field." + k, v);
      if (pageParam) p.set("cursor", pageParam);
      return api<ItemPage>("GET", path`/workspaces/${ws}/projects/${project}/items` + "?" + p.toString());
    },
    getNextPageParam: (last) => last.next_cursor,
  });
  const loaded = q.data?.pages.reduce((n, p) => n + p.items.length, 0) ?? 0;
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = q;
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage && loaded < CALENDAR_CAP) void fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, loaded, fetchNextPage]);
  return { query: q, truncated: loaded >= CALENDAR_CAP && hasNextPage };
}

export function CalendarView({ ws, project, filters, month, today, onMonth, members, canEdit, compact, onOpen }: {
  ws: string;
  project: string;
  filters: ItemFilters;
  month: Month;
  /** Today in the workspace's time zone, YYYY-MM-DD. */
  today: string;
  onMonth: (m: Month) => void;
  members: Map<string, Member>;
  canEdit: boolean;
  compact: boolean;
  onOpen: (n: number) => void;
}) {
  const toast = useToast();
  const grid = useMemo(() => monthGrid(month), [month]);
  const from = compact ? month + "-01" : grid.first;
  const to = compact ? (monthDays(month).at(-1) ?? grid.last) : grid.last;
  const range = useRangeItems(ws, project, filters, from, to);
  // Weekends and holidays come from the workspace calendar (ADR-0027); until
  // it loads (or if it fails) the default Saturday/Sunday weekend is shown.
  const workCal = useWorkCalendar(ws, from, to);
  const lookup = useMemo(() => dayLookup(workCal.data), [workCal.data]);
  // A due filter other than "none" leaves nothing undated to show.
  const showUndated = !filters.due || filters.due === "none";
  const undatedQ = useItems(ws, project, { ...filters, due: "none", sort: "number" }, showUndated);
  // Overdue items, whatever month they fell due in: every day they have
  // been late is marked in its band colour (ADR-0057). The first page (the
  // longest overdue first) is enough for a month view.
  const showLate = !compact && (!filters.due || filters.due === "overdue");
  const lateQ = useItems(ws, project, { ...filters, due: "overdue", sort: "due" }, showLate);
  const update = useOptimisticUpdate(ws, project);
  const [drag, setDrag] = useState<Item | null>(null);
  const [dayOpen, setDayOpen] = useState<string | null>(null);

  // An item moved by a drop is optimistically updated in whichever list
  // holds it, so sort both lists by their current due date.
  const { dated, undated } = useMemo(() => {
    const seen = new Set<string>();
    const all = [...flattenPages(range.query.data?.pages ?? []), ...flattenPages(undatedQ.data?.pages ?? [])].filter((it) => {
      if (seen.has(it.id)) return false;
      seen.add(it.id);
      return true;
    });
    const inRange = all.filter((it) => it.due_date !== null && it.due_date >= from && it.due_date <= to);
    inRange.sort((a, b) => (a.due_date ?? "").localeCompare(b.due_date ?? "") || a.number - b.number);
    return { dated: byDay(inRange), undated: showUndated ? all.filter((it) => it.due_date === null) : [] };
  }, [range.query.data, undatedQ.data, from, to, showUndated]);

  const late = useMemo(
    () => (showLate ? lateDays(flattenPages(lateQ.data?.pages ?? []), grid.first, grid.last, today) : new Map<string, LateDay<Item>[]>()),
    [showLate, lateQ.data, grid.first, grid.last, today],
  );

  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 8 } }),
    useSensor(KeyboardSensor, { keyboardCodes: { start: ["Space"], cancel: ["Escape"], end: ["Space", "Enter"] } }),
  );

  const move = (item: Item, due: string | null) => {
    if (!canEdit || item.due_date === due) return;
    update.mutate(
      { item, patch: { due_date: due }, optimistic: { ...item, due_date: due, overdue: overdueOn(item, due, today) } },
      {
        onSuccess: (saved) => { toast("success", tc.moved(saved.key, due ? formatDate(due) : tc.noDue)); },
        onError: (err) => { toast("error", err instanceof ApiError && err.isConflict ? t.conflict : err instanceof ApiError ? err.message : t.errorGeneric); },
      },
    );
  };
  const onDragStart = ({ active }: DragStartEvent) => {
    setDrag((active.data.current as { item?: Item } | undefined)?.item ?? null);
  };
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    setDrag(null);
    const item = (active.data.current as { item?: Item } | undefined)?.item;
    const target = over ? String(over.id) : null;
    if (!item || !target) return;
    move(item, target === NO_DUE ? null : target.slice("day:".length));
  };

  const header = (
    <div className="flex items-center gap-2 border-b border-zinc-200 bg-white px-3 py-2 dark:border-zinc-800 dark:bg-zinc-900 md:px-4">
      <Button aria-label={tc.prevMonth} className="pointer-coarse:min-w-11" onClick={() => { onMonth(shiftMonth(month, -1)); }}>
        <span aria-hidden>‹</span>
      </Button>
      <h2 className="min-w-36 text-center text-sm font-semibold" aria-live="polite" data-testid="calendar-month">
        {monthTitle(month)}
      </h2>
      <Button aria-label={tc.nextMonth} className="pointer-coarse:min-w-11" onClick={() => { onMonth(shiftMonth(month, 1)); }}>
        <span aria-hidden>›</span>
      </Button>
      {monthOf(today) !== month && <Button onClick={() => { onMonth(monthOf(today)); }}>{tc.thisMonth}</Button>}
      {(range.query.isFetching || undatedQ.isFetching) && <span className="text-xs text-zinc-500 dark:text-zinc-400">{t.loading}</span>}
      {range.truncated && <span role="status" className="text-xs text-amber-700 dark:text-amber-400">{tc.truncated(CALENDAR_CAP)}</span>}
    </div>
  );

  if (range.query.isPending) return <div className="flex h-full flex-col">{header}<Spinner /></div>;
  if (range.query.isError) return <div className="flex h-full flex-col">{header}<ErrorState error={range.query.error} onRetry={() => void range.query.refetch()} /></div>;

  if (compact) {
    const days = monthDays(month).filter((d) => dated.has(d));
    return (
      <div className="flex h-full flex-col">
        {header}
        <div className="min-h-0 flex-1 overflow-y-auto" data-testid="calendar-agenda">
          {days.length === 0 && <p className="px-4 py-6 text-center text-sm text-zinc-500 dark:text-zinc-400">{tc.agendaEmpty}</p>}
          {days.map((d) => (
            <section key={d} aria-labelledby={`agenda-${d}`} className="border-b border-zinc-200 dark:border-zinc-800">
              <h3 id={`agenda-${d}`} className={cn("sticky top-0 z-[1] bg-zinc-50 px-3 py-1.5 text-xs font-medium text-zinc-600 dark:bg-zinc-950 dark:text-zinc-400", d === today && "text-brand-700 dark:text-brand-400")}>
                {tc.weekdaysLong[weekday(d)]}, {formatDate(d)}{d === today ? ` · ${tc.today}` : ""}
                <DayNote info={lookup.info(d)} />
              </h3>
              <ul>
                {(dated.get(d) ?? []).map((it) => (
                  <li key={it.id}>
                    <AgendaRow item={it} members={members} onOpen={onOpen} />
                  </li>
                ))}
              </ul>
            </section>
          ))}
          {showUndated && (
            <details className="border-b border-zinc-200 dark:border-zinc-800">
              <summary className="min-h-11 content-center px-3 text-sm font-medium">
                {tc.noDue} ({undated.length}{undatedQ.hasNextPage ? "+" : ""})
              </summary>
              <ul>
                {undated.map((it) => (
                  <li key={it.id}>
                    <AgendaRow item={it} members={members} onOpen={onOpen} />
                  </li>
                ))}
              </ul>
              {undatedQ.hasNextPage && (
                <div className="p-3"><Button onClick={() => void undatedQ.fetchNextPage()} disabled={undatedQ.isFetchingNextPage}>{tc.loadMore}</Button></div>
              )}
            </details>
          )}
        </div>
      </div>
    );
  }

  const dayList = dayOpen ? (dated.get(dayOpen) ?? []) : [];
  const dayLate = dayOpen ? (late.get(dayOpen) ?? []) : [];
  return (
    <DndContext sensors={sensors} collisionDetection={collision} onDragStart={onDragStart} onDragCancel={() => { setDrag(null); }} onDragEnd={onDragEnd}>
      <div className="flex h-full flex-col">
        {header}
        <div className="flex min-h-0 flex-1">
          <div className="flex min-w-0 flex-1 flex-col overflow-auto">
            <div role="row" className="grid grid-cols-7 border-b border-zinc-200 text-xs font-medium text-zinc-500 dark:text-zinc-400 dark:border-zinc-800">
              {tc.weekdays.map((w, i) => (
                <span
                  key={w}
                  role="columnheader"
                  title={tc.weekdaysLong[i]}
                  className={cn("px-2 py-1.5", lookup.isWeekend((i + 1) % 7) && "text-zinc-400 dark:text-zinc-600")}
                >
                  {w}
                </span>
              ))}
            </div>
            <div role="grid" aria-label={monthTitle(month)} data-testid="calendar-grid" className="grid flex-1 auto-rows-[minmax(7rem,1fr)] grid-cols-7">
              {grid.days.map((d) => (
                <DayCell
                  key={d}
                  day={d}
                  inMonth={monthOf(d) === month}
                  today={d === today}
                  info={lookup.info(d)}
                  items={dated.get(d) ?? []}
                  late={late.get(d) ?? []}
                  members={members}
                  canEdit={canEdit}
                  onOpen={onOpen}
                  onMore={() => { setDayOpen(d); }}
                />
              ))}
            </div>
          </div>
          {showUndated && (
            <UndatedList
              items={undated}
              hasMore={undatedQ.hasNextPage}
              loadingMore={undatedQ.isFetchingNextPage}
              loadMore={() => void undatedQ.fetchNextPage()}
              members={members}
              canEdit={canEdit}
              onOpen={onOpen}
            />
          )}
        </div>
      </div>
      <DragOverlay dropAnimation={null}>{drag ? <CardBody item={drag} members={members} className="shadow-lg ring-2 ring-brand-500" /> : null}</DragOverlay>
      <Dialog open={dayOpen !== null} onClose={() => { setDayOpen(null); }} title={dayOpen ? `${tc.weekdaysLong[weekday(dayOpen)]}, ${formatDate(dayOpen)}` : ""}>
        <ul className="flex flex-col gap-1.5">
          {dayList.map((it) => (
            <li key={it.id}>
              <button type="button" className="w-full text-left" onClick={() => { setDayOpen(null); onOpen(it.number); }}>
                <CardBody item={it} members={members} />
              </button>
            </li>
          ))}
          {dayLate.map((l) => (
            <li key={"late:" + l.item.id}>
              <button
                type="button"
                className={cn("flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs", LATE_CHIP[l.level])}
                onClick={() => { setDayOpen(null); onOpen(l.item.number); }}
              >
                <span className="shrink-0 font-mono text-2xs">{l.item.key}</span>
                <span className="min-w-0 flex-1 truncate">{l.item.title}</span>
                <span className="shrink-0">{t.lateBy(l.days)}</span>
              </button>
            </li>
          ))}
        </ul>
      </Dialog>
    </DndContext>
  );
}


/** Accessible text for a non-working day or a transferred working day. */
function dayNote(info: DayInfo): string | null {
  if (info.kind === "holiday") return tw.holidayLabel(info.name ?? "");
  if (info.kind === "workday") return tw.workdayLabel(info.name ?? "");
  if (info.kind === "weekend") return tw.weekendLabel;
  return null;
}

function DayNote({ info }: { info: DayInfo }) {
  if (info.kind !== "holiday" && info.kind !== "workday") return null;
  return (
    <span className={cn("ml-1 font-normal", info.kind === "holiday" ? "text-amber-800 dark:text-amber-300" : "text-emerald-800 dark:text-emerald-300")}>
      · {info.name}
    </span>
  );
}

function DayCell({ day, inMonth, today, info, items, late, members, canEdit, onOpen, onMore }: {
  day: string; inMonth: boolean; today: boolean; info: DayInfo; items: Item[]; late: LateDay<Item>[]; members: Map<string, Member>; canEdit: boolean;
  onOpen: (n: number) => void; onMore: () => void;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: "day:" + day, disabled: !canEdit });
  const { shown, more } = overflow(items, PER_DAY);
  const lateShown = overflow(late, PER_DAY);
  const note = dayNote(info);
  const label = `${tc.weekdaysLong[weekday(day)]}, ${formatDate(day)}${note ? ` (${note})` : ""}`;
  return (
    <div
      ref={setNodeRef}
      role="gridcell"
      aria-label={tc.dayItems(label, items.length)}
      data-day={day}
      data-nonworking={info.working ? undefined : (info.kind ?? undefined)}
      title={info.name ?? undefined}
      className={cn(
        "flex min-w-0 flex-col gap-1 border-r border-b border-zinc-200 p-1 dark:border-zinc-800",
        !info.working && "bg-zinc-100/60 dark:bg-zinc-900/60",
        info.kind === "holiday" && "bg-amber-50/80 dark:bg-amber-950/30",
        !inMonth && "bg-zinc-50/70 text-zinc-500 dark:text-zinc-400 dark:bg-zinc-900/40",
        isOver && "bg-brand-50 ring-2 ring-brand-500 ring-inset dark:bg-brand-950/40",
      )}
    >
      <span aria-hidden className="flex min-w-0 items-baseline gap-1">
        {(info.kind === "holiday" || info.kind === "workday") && (
          <span className={cn("min-w-0 flex-1 truncate text-2xs", info.kind === "holiday" ? "text-amber-800 dark:text-amber-300" : "text-emerald-800 dark:text-emerald-300")}>
            {info.name}
          </span>
        )}
        <span className={cn("ml-auto px-1 text-xs tabular-nums", today && "rounded-full bg-brand-600 px-1.5 font-semibold text-white")}>
          {Number(day.slice(8))}
        </span>
      </span>
      {shown.map((it) => (
        <DraggableCard key={it.id} item={it} members={members} canEdit={canEdit} onOpen={onOpen} compact />
      ))}
      {more > 0 && (
        <button type="button" onClick={onMore} className="self-start rounded px-1 text-xs font-medium text-brand-700 hover:underline dark:text-brand-400">
          {tc.more(more)}
        </button>
      )}
      {lateShown.shown.map((l) => (
        <button
          key={"late:" + l.item.id}
          type="button"
          data-testid="calendar-late"
          data-late={l.level}
          title={`${l.item.key} ${l.item.title} · ${t.lateBy(l.days)}`}
          onClick={() => { onOpen(l.item.number); }}
          className={cn("flex min-w-0 items-center gap-1 rounded px-1 py-0.5 text-left text-2xs", LATE_CHIP[l.level])}
        >
          <span className="min-w-0 flex-1 truncate">{l.item.title}</span>
          <span className="shrink-0 tabular-nums">{l.days}</span>
          <span className="sr-only">{t.lateBy(l.days)}</span>
        </button>
      ))}
      {lateShown.more > 0 && (
        <button type="button" onClick={onMore} className="self-start rounded px-1 text-2xs font-medium text-red-700 hover:underline dark:text-red-400">
          {tc.moreLate(lateShown.more)}
        </button>
      )}
    </div>
  );
}

function UndatedList({ items, hasMore, loadingMore, loadMore, members, canEdit, onOpen }: {
  items: Item[]; hasMore: boolean; loadingMore: boolean; loadMore: () => void; members: Map<string, Member>; canEdit: boolean; onOpen: (n: number) => void;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: NO_DUE, disabled: !canEdit });
  return (
    <aside
      ref={setNodeRef}
      aria-label={tc.noDue}
      data-testid="calendar-undated"
      className={cn("flex w-64 shrink-0 flex-col border-l border-zinc-200 dark:border-zinc-800 max-lg:w-52", isOver && "bg-brand-50 dark:bg-brand-950/40")}
    >
      <h3 className="border-b border-zinc-200 px-3 py-2 text-sm font-medium dark:border-zinc-800">{tc.noDue}</h3>
      {canEdit && <p className="px-3 pt-2 text-xs text-zinc-500 dark:text-zinc-400">{tc.noDueHint}</p>}
      <div className="flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto p-2">
        {items.length === 0 && <p className="text-xs text-zinc-500 dark:text-zinc-400">{tc.noDueEmpty}</p>}
        {items.map((it) => <DraggableCard key={it.id} item={it} members={members} canEdit={canEdit} onOpen={onOpen} />)}
        {hasMore && <Button onClick={loadMore} disabled={loadingMore}>{tc.loadMore}</Button>}
      </div>
    </aside>
  );
}

function DraggableCard({ item, members, canEdit, onOpen, compact = false }: {
  item: Item; members: Map<string, Member>; canEdit: boolean; onOpen: (n: number) => void; compact?: boolean;
}) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: item.id, data: { item }, disabled: !canEdit });
  return (
    <CardBody
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      item={item}
      members={members}
      compact={compact}
      aria-roledescription={canEdit ? tc.dragHint : undefined}
      aria-label={`${item.key} ${item.title}`}
      onClick={() => { onOpen(item.number); }}
      onKeyDown={(e) => {
        // Space and Enter are taken by the keyboard sensor while dragging.
        if (e.key === "Enter" && !isDragging) onOpen(item.number);
        listeners?.onKeyDown?.(e);
      }}
      className={cn("cursor-pointer", isDragging && "opacity-40", canEdit && "select-none [-webkit-touch-callout:none]")}
    />
  );
}

function CardBody({ item, members, compact = false, className, ref, ...rest }: {
  item: Item; members: Map<string, Member>; compact?: boolean; ref?: Ref<HTMLDivElement>;
} & HTMLAttributes<HTMLDivElement>) {
  const name = item.assignee_id ? (members.get(item.assignee_id)?.display_name ?? "…") : null;
  const level = item.overdue && item.due_date ? lateLevel(Math.max(1, daysLate(item.due_date, localToday()))) : 0;
  return (
    <div
      ref={ref}
      data-testid="calendar-item"
      data-overdue={item.overdue || undefined}
      {...rest}
      className={cn(
        "flex min-w-0 items-center gap-1.5 rounded-md bg-white px-1.5 py-1 text-left text-xs ring-1 ring-zinc-200 hover:ring-zinc-300 focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:outline-none dark:bg-zinc-900 dark:ring-zinc-700",
        level ? LATE_EDGE[level] : undefined,
        level === 1 && "text-yellow-900 ring-yellow-400 dark:text-yellow-200 dark:ring-yellow-600",
        level === 2 && "text-orange-800 ring-orange-300 dark:text-orange-200 dark:ring-orange-800",
        level === 3 && "text-red-700 ring-red-300 dark:text-red-400 dark:ring-red-800",
        item.status.category === "done" && "text-zinc-500 dark:text-zinc-400 line-through",
        className,
      )}
    >
      {!compact && <span className="shrink-0 font-mono text-2xs text-zinc-500 dark:text-zinc-400">{item.key}</span>}
      <span className="min-w-0 flex-1 truncate">{item.title}</span>
      {item.overdue && <span className="sr-only">{t.overdue}</span>}
      {name && !compact && <Avatar name={name} />}
    </div>
  );
}

function AgendaRow({ item, members, onOpen }: { item: Item; members: Map<string, Member>; onOpen: (n: number) => void }) {
  return (
    <button type="button" onClick={() => { onOpen(item.number); }} className="block w-full px-3 py-1.5 text-left">
      <CardBody item={item} members={members} className="min-h-11 text-sm" />
    </button>
  );
}
