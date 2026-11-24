import {
  DndContext, DragOverlay, KeyboardSensor, MouseSensor, pointerWithin, rectIntersection, TouchSensor, useDraggable, useDroppable, useSensor, useSensors,
  type CollisionDetection, type DragEndEvent, type DragStartEvent,
} from "@dnd-kit/core";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState, type HTMLAttributes, type Ref } from "react";

import { api, ApiError, path } from "../api/client";
import { keys, useColumnItems, useItemCounts, useItemTypes, useUpdateItem, type ItemFilters } from "../api/hooks";
import type { Item, Member, Status, StatusCategory } from "../api/types";
import { useToast } from "../components/toast-context";
import { ApprovalBadge } from "../components/Approvals";
import { BlockedBadge } from "../components/Links";
import { SubtaskBadge } from "../components/Subtasks";
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

/** Drops land in the column under the finger or pointer, which matters on a
 *  narrow screen where the lifted card overlaps two columns; keyboard drags
 *  have no pointer and fall back to the card's overlap. */
const collision: CollisionDetection = (args) => {
  const under = pointerWithin(args);
  return under.length > 0 ? under : rectIntersection(args);
};

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

  // Separate mouse and touch sensors (ADR-0019): a mouse drag starts after
  // 6 px, so a click stays a click; a finger has to rest on the card for
  // 250 ms first, so swiping across cards scrolls the board and the columns
  // instead of picking a card up. Moving more than 8 px during the wait is a
  // scroll and cancels the drag. Keyboard: Space lifts a card, arrows move
  // it, Space or Enter drops it, Escape cancels; Enter on a resting card
  // opens it.
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 8 } }),
    useSensor(KeyboardSensor, { keyboardCodes: { start: ["Space"], cancel: ["Escape"], end: ["Space", "Enter"] } }),
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
    <DndContext sensors={sensors} collisionDetection={collision} onDragStart={onDragStart} onDragCancel={() => { setDrag(null); }} onDragEnd={(e) => void onDragEnd(e)}>
      {/* Phones: a column is most of the screen wide and the board snaps to
          columns while swiping; snapping is off during a drag so it does not
          fight dnd-kit's auto-scroll. */}
      <div
        data-testid="board-scroller"
        className={cn(
          "flex h-full min-h-0 gap-3 overflow-x-auto overscroll-x-contain p-3 sm:p-4",
          drag === null && "snap-x snap-mandatory scroll-px-3 sm:snap-none",
        )}
      >
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
      {/* The lifted card follows the pointer above everything, so it is not
          clipped by its scrolling column and stays visible under a finger. */}
      <DragOverlay dropAnimation={null}>
        {drag ? <CardBody item={drag.item} members={members} className="rotate-1 shadow-xl ring-brand-500" /> : null}
      </DragOverlay>
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
        "flex min-h-0 w-[min(85vw,18rem)] shrink-0 snap-start flex-col rounded-xl bg-zinc-100 transition-opacity sm:w-72 dark:bg-zinc-900",
        isOver && !dimmed && "ring-2 ring-brand-500",
        dimmed && "opacity-40",
      )}
    >
      <h2 className="flex items-center justify-between gap-2 px-3 py-2 text-xs font-semibold tracking-wide text-zinc-600 uppercase dark:text-zinc-400">
        <span className="flex min-w-0 items-center gap-1.5">
          <span aria-hidden className={cn("size-2 shrink-0 rounded-full", CATEGORY_DOT[column.category])} />
          <span className="truncate">{column.name}</span>
        </span>
        <span className="rounded-full bg-zinc-200 px-1.5 text-2xs tabular-nums dark:bg-zinc-800">{count ?? "…"}</span>
      </h2>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-2 pb-2">
        {q.isPending && column.statusIds.length > 0 ? (
          <Spinner />
        ) : q.isError ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : shown.length === 0 ? (
          <p className="px-1 py-6 text-center text-xs text-zinc-500 dark:text-zinc-400">{t.columnEmpty}</p>
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
  // No transform here: the DragOverlay shows the moving copy, the original
  // stays in place, faded, as a placeholder.
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: item.id, disabled: !draggable, data: { item, column } });
  return (
    <CardBody
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      item={item}
      members={members}
      onClick={() => { onOpen(item.number); }}
      onKeyDown={(e) => {
        // Space and Enter are taken by the keyboard sensor while dragging.
        if (e.key === "Enter" && !isDragging) onOpen(item.number);
        listeners?.onKeyDown?.(e);
      }}
      className={cn(draggable && "select-none [-webkit-touch-callout:none]", isDragging && "opacity-40")}
    />
  );
}

function CardBody({ item, members, className, ref, ...rest }: HTMLAttributes<HTMLDivElement> & {
  item: Item; members: Map<string, Member>; ref?: Ref<HTMLDivElement>;
}) {
  const assignee = item.assignee_id ? members.get(item.assignee_id)?.display_name : undefined;
  return (
    <div
      ref={ref}
      {...rest}
      className={cn(
        "cursor-pointer rounded-lg bg-white p-3 text-sm shadow-sm ring-1 ring-zinc-200 dark:bg-zinc-800 dark:ring-zinc-700",
        className,
      )}
    >
      <p className="line-clamp-3 [overflow-wrap:anywhere]">{item.title}</p>
      {item.due_date !== null || (item.subtasks?.count ?? 0) > 0 || item.approval ? (
        <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
          {item.due_date && <DueChip date={item.due_date} overdue={item.overdue} />}
          <SubtaskBadge summary={item.subtasks} />
          <ApprovalBadge approval={item.approval} />
          <BlockedBadge blocked={item.blocked} />
        </div>
      ) : null}
      <div className="mt-2 flex items-center justify-between gap-2">
        <span className="font-mono text-xs text-zinc-500 dark:text-zinc-400">{item.key}</span>
        <span className="flex min-w-0 items-center gap-1.5">
          <StatusBadge name={item.status.name} category={item.status.category} />
          {assignee && <Avatar name={assignee} />}
        </span>
      </div>
    </div>
  );
}
