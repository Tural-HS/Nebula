import { DndContext, KeyboardSensor, PointerSensor, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent, type DragStartEvent } from "@dnd-kit/core";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";

import { api, ApiError, path } from "../api/client";
import { keys, useColumnItems, useItemCounts, useItemTypes, useUpdateItem, type ItemFilters } from "../api/hooks";
import type { Item, Member, Status, StatusCategory } from "../api/types";
import { useToast } from "../components/toast-context";
import { Avatar, DueChip, EmptyState, ErrorState, Spinner, StatusBadge } from "../components/ui";
import { cn } from "../lib/cn";
import { t } from "../i18n";
import { buildColumns, columnCount, dropTargets, flattenPages, otherColumn, pickTargetStatus, type BoardColumn, type GroupBy } from "../lib/board";

// ---------------------------------------------------------------------------
// Board: one column per status (or per category). Every column loads its own
// items page by page and shows its true size from /items/counts, so a project
// with thousands of items stays fast and nothing is cut off. Drag a card to
// change its status; columns the workflow does not allow are dimmed.
// ---------------------------------------------------------------------------

/** An item being moved: shown in its target column until the server answers. */
interface Move {
  item: Item;
  from: string; // column key
  to: string;
}

export function Board({ ws, project, filters, groupBy, members, canEdit, onOpen }: {
  ws: string; project: string; filters: ItemFilters; groupBy: GroupBy; members: Map<string, Member>; canEdit: boolean;
  onOpen: (n: number) => void;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const types = useItemTypes(ws);
  const counts = useItemCounts(ws, project, filters);
  const update = useUpdateItem(ws, project);
  const [moves, setMoves] = useState<Record<string, Move>>({});
  const [drag, setDrag] = useState<{ item: Item; targets: Set<string> | null } | null>(null);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }), // small moves stay clicks
    useSensor(KeyboardSensor),
  );

  const columns = useMemo(() => {
    const cols = buildColumns(types.data ?? [], groupBy, (c) => t.category[c]);
    const other = otherColumn(cols, Object.keys(counts.data?.by_status ?? {}), t.boardOther);
    return other ? [...cols, other] : cols;
  }, [types.data, groupBy, counts.data]);

  if (types.isPending) return <Spinner />;
  if (types.isError) return <ErrorState error={types.error} onRetry={() => void types.refetch()} />;
  if (columns.length === 0) return <EmptyState>{t.boardNoTypes}</EmptyState>;

  const allowedFor = (item: Item) =>
    qc.query({
      queryKey: keys.transitions(ws, project, item.number),
      queryFn: async () => (await api<{ statuses: Status[] }>("GET", path`/workspaces/${ws}/projects/${project}/items/${item.number}/transitions`)).statuses,
      staleTime: 0,
    });

  const onDragStart = ({ active }: DragStartEvent) => {
    const item = (active.data.current as { item?: Item } | undefined)?.item;
    if (!item) return;
    setDrag({ item, targets: null });
    allowedFor(item)
      .then((allowed) => { setDrag((d) => (d?.item.id === item.id ? { item, targets: dropTargets(item, columns, types.data, allowed) } : d)); })
      .catch(() => undefined); // the drop re-checks and reports errors
  };

  const onDragEnd = async ({ active, over }: DragEndEvent) => {
    setDrag(null);
    const data = active.data.current as { item?: Item; column?: string } | undefined;
    const item = data?.item;
    const column = columns.find((c) => c.key === over?.id);
    if (!item || !column || !canEdit || column.key === data.column) return;
    try {
      const target = pickTargetStatus(item, column, types.data, await allowedFor(item));
      if (!target) {
        toast("error", t.transitionNotAllowed);
        return;
      }
      setMoves((m) => ({ ...m, [item.id]: { item: { ...item, status: target }, from: data.column ?? "", to: column.key } }));
      await update.mutateAsync({ n: item.number, patch: { version: item.version, status_id: target.id } });
      // Keep the card where it was dropped until the columns have reloaded.
      // The mutation already started that reload; join it instead of
      // cancelling and restarting every column's request.
      await qc.invalidateQueries({ queryKey: keys.items(ws, project) }, { cancelRefetch: false });
    } catch (err) {
      toast("error", err instanceof ApiError && err.isConflict ? t.conflict : err instanceof ApiError ? err.message : t.errorGeneric);
    } finally {
      setMoves((m) => Object.fromEntries(Object.entries(m).filter(([id]) => id !== item.id)));
    }
  };

  const pending = Object.values(moves);
  return (
    <DndContext sensors={sensors} onDragStart={onDragStart} onDragCancel={() => { setDrag(null); }} onDragEnd={(e) => void onDragEnd(e)}>
      <div className="flex h-full min-h-0 gap-3 overflow-x-auto p-4">
        {columns.map((c) => {
          // A status filter narrows every column to the chosen statuses.
          const shown = filters.status && filters.status.length > 0 ? { ...c, statusIds: c.statusIds.filter((id) => filters.status?.includes(id)) } : c;
          const base = shown.statusIds.length === 0 ? 0 : columnCount(shown, counts.data?.by_status);
          const delta = pending.filter((m) => m.to === c.key).length - pending.filter((m) => m.from === c.key).length;
          return (
            <BoardColumnView
              key={c.key}
              ws={ws}
              project={project}
              filters={filters}
              column={shown}
              count={base === undefined ? undefined : base + delta}
              moves={moves}
              members={members}
              canEdit={canEdit && !c.other}
              dimmed={drag !== null && drag.targets !== null && !drag.targets.has(c.key) && !c.statusIds.includes(drag.item.status.id)}
              onOpen={onOpen}
            />
          );
        })}
      </div>
    </DndContext>
  );
}

function BoardColumnView({ ws, project, filters, column, count, moves, members, canEdit, dimmed, onOpen }: {
  ws: string; project: string; filters: ItemFilters; column: BoardColumn; count: number | undefined;
  moves: Record<string, Move>; members: Map<string, Member>; canEdit: boolean; dimmed: boolean; onOpen: (n: number) => void;
}) {
  const q = useColumnItems(ws, project, filters, column.statusIds);
  const { setNodeRef, isOver } = useDroppable({ id: column.key, disabled: Boolean(column.other) });
  const sentinel = useRef<HTMLDivElement>(null);
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = q;

  // Load the next page when the end of the column scrolls into view.
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !hasNextPage) return;
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting) && !isFetchingNextPage) void fetchNextPage();
    }, { rootMargin: "200px" });
    io.observe(el);
    return () => { io.disconnect(); };
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const incoming = Object.values(moves).filter((m) => m.to === column.key).map((m) => m.item);
  const own = flattenPages(q.data?.pages ?? []).filter((it) => !(it.id in moves));
  const shown = [...incoming, ...own];

  return (
    <section
      ref={setNodeRef}
      aria-label={column.name}
      className={cn(
        "flex min-h-0 w-72 min-w-64 shrink-0 flex-col rounded-xl bg-zinc-100 transition-opacity dark:bg-zinc-900",
        isOver && !dimmed && "ring-2 ring-brand-500",
        dimmed && "opacity-40",
      )}
    >
      <h2 className="flex items-center justify-between gap-2 px-3 py-2 text-xs font-semibold tracking-wide text-zinc-600 uppercase dark:text-zinc-400">
        <span className="flex min-w-0 items-center gap-1.5">
          <span aria-hidden className={cn("size-2 shrink-0 rounded-full", CATEGORY_DOT[column.category])} />
          <span className="truncate">{column.name}</span>
        </span>
        <span className="rounded-full bg-zinc-200 px-1.5 text-[11px] tabular-nums dark:bg-zinc-800">{count ?? "…"}</span>
      </h2>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-2 pb-2">
        {q.isPending && column.statusIds.length > 0 ? (
          <Spinner />
        ) : q.isError ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : shown.length === 0 ? (
          <p className="px-1 py-6 text-center text-xs text-zinc-400">{t.columnEmpty}</p>
        ) : (
          shown.map((it) => <Card key={it.id} item={it} column={column.key} members={members} draggable={canEdit} onOpen={onOpen} />)
        )}
        <div ref={sentinel} aria-hidden className="h-px shrink-0" />
        {isFetchingNextPage && <Spinner />}
      </div>
    </section>
  );
}

const CATEGORY_DOT: Record<StatusCategory, string> = {
  todo: "bg-zinc-400",
  in_progress: "bg-brand-500",
  done: "bg-emerald-500",
  canceled: "bg-zinc-300 dark:bg-zinc-600",
};

function Card({ item, column, members, draggable, onOpen }: {
  item: Item; column: string; members: Map<string, Member>; draggable: boolean; onOpen: (n: number) => void;
}) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: item.id, disabled: !draggable, data: { item, column } });
  const style = transform ? { transform: `translate3d(${String(transform.x)}px, ${String(transform.y)}px, 0)` } : undefined;
  const assignee = item.assignee_id ? members.get(item.assignee_id)?.display_name : undefined;
  return (
    <div
      ref={setNodeRef}
      style={style}
      {...listeners}
      {...attributes}
      onClick={() => { onOpen(item.number); }}
      onKeyDown={(e) => {
        if (e.key === "Enter") onOpen(item.number);
      }}
      className={cn(
        "cursor-pointer rounded-lg bg-white p-3 text-sm shadow-sm ring-1 ring-zinc-200 dark:bg-zinc-800 dark:ring-zinc-700",
        isDragging && "relative z-10 opacity-80 shadow-lg",
      )}
    >
      <p className="line-clamp-3">{item.title}</p>
      {item.due_date && (
        <div className="mt-1.5">
          <DueChip date={item.due_date} overdue={item.overdue} />
        </div>
      )}
      <div className="mt-2 flex items-center justify-between gap-2">
        <span className="font-mono text-xs text-zinc-500">{item.key}</span>
        <span className="flex items-center gap-1.5">
          <StatusBadge name={item.status.name} category={item.status.category} />
          {assignee && <Avatar name={assignee} />}
        </span>
      </div>
    </div>
  );
}
