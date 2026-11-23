import { DndContext, KeyboardSensor, PointerSensor, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent, type DragStartEvent } from "@dnd-kit/core";
import { useQueryClient } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { api, ApiError, path } from "../api/client";
import { keys, useColumnItems, useItemCounts, useItems, useItemTypes, useMembers, useProjects, useUpdateItem, useWorkspace, type DueFilter, type ItemFilters } from "../api/hooks";
import { roleAtLeast, type Item, type Member, type Status, type StatusCategory } from "../api/types";
import { Layout } from "../components/Layout";
import { useToast } from "../components/toast-context";
import { Avatar, Button, DueChip, EmptyState, ErrorState, Input, Select, Spinner, StatusBadge } from "../components/ui";
import { cn } from "../lib/cn";
import { t } from "../i18n";
import { tr } from "../i18n-recur";
import { ti } from "../i18n-import";
import { buildColumns, columnCount, dropTargets, flattenPages, otherColumn, pickTargetStatus, type BoardColumn, type GroupBy } from "../lib/board";
import { formatDateTime, formatRelative } from "../lib/format";
import { CreateItemDialog } from "./CreateItemDialog";
import { ItemDrawer } from "./ItemDrawer";

const DUE_FILTERS: readonly DueFilter[] = ["overdue", "today", "week", "none"];

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => { setV(value); }, ms);
    return () => { clearTimeout(id); };
  }, [value, ms]);
  return v;
}

export function ItemsPage() {
  const { ws = "", project = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const view = params.get("view") === "board" ? "board" : "table";
  const groupBy: GroupBy = params.get("group") === "category" ? "category" : "status";
  const openItem = Number(params.get("item")) || null;
  const assigneeParam = params.get("assignee");
  const assignee: ItemFilters["assignee"] = assigneeParam === "me" || assigneeParam === "none" ? assigneeParam : "";
  const dueParam = params.get("due");
  const due: DueFilter = DUE_FILTERS.find((d) => d === dueParam) ?? "";
  const [search, setSearch] = useState(params.get("q") ?? "");
  const q = useDebounced(search, 300);
  const [creating, setCreating] = useState(false);

  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const members = useMembers(ws);
  const filters = useMemo<ItemFilters>(() => ({ q, assignee, due }), [q, assignee, due]);
  const items = useItems(ws, project, filters, view === "table");
  const canEdit = roleAtLeast(workspace.data?.role, "member");
  const canImport = roleAtLeast(workspace.data?.role, "admin");
  const projectName = projects.data?.find((p) => p.key === project)?.name ?? project;

  const setParam = (k: string, v: string | null) => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (v) next.set(k, v);
        else next.delete(k);
        return next;
      },
      { replace: k !== "item" },
    );
  };

  useEffect(() => {
    setParam("q", q || null);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- sync only when the debounced query changes
  }, [q]);

  const memberById = useMemo(() => new Map((members.data ?? []).map((m) => [m.user_id, m])), [members.data]);
  const all = useMemo(() => flattenPages(items.data?.pages ?? []), [items.data]);

  return (
    <Layout
      crumbs={[
        { label: t.workspaces, to: "/" },
        { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` },
        { label: projectName },
      ]}
      actions={
        <>
          {canEdit && (
            <Link
              to={`/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project)}/recurring`}
              className="rounded-md px-3 py-1.5 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              {tr.nav}
            </Link>
          )}
          {canImport && (
            <Link
              to={`/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project)}/import`}
              className="rounded-md px-3 py-1.5 text-sm text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              {ti.importAction}
            </Link>
          )}
          {canEdit && (
            <Button variant="primary" onClick={() => { setCreating(true); }}>
              {t.newItem}
            </Button>
          )}
        </>
      }
    >
      <div className="flex h-full flex-col">
        <div className="flex flex-wrap items-center gap-2 border-b border-zinc-200 bg-white px-4 py-2 dark:border-zinc-800 dark:bg-zinc-900">
          <Input
            type="search"
            aria-label={t.search}
            placeholder={t.search}
            value={search}
            onChange={(e) => { setSearch(e.target.value); }}
            className="max-w-64"
            maxLength={500}
          />
          <Select aria-label={t.assignee} value={assignee} onChange={(e) => { setParam("assignee", e.target.value || null); }} className="max-w-56">
            <option value="">{t.filterAll}</option>
            <option value="me">{t.filterMine}</option>
            <option value="none">{t.filterNone}</option>
          </Select>
          <Select aria-label={t.dueFilter} value={due} onChange={(e) => { setParam("due", e.target.value || null); }} className="max-w-48">
            <option value="">{t.dueAll}</option>
            <option value="overdue">{t.dueOverdue}</option>
            <option value="today">{t.dueToday}</option>
            <option value="week">{t.dueWeek}</option>
            <option value="none">{t.dueNone}</option>
          </Select>
          {view === "board" && (
            <Select aria-label={t.groupBy} value={groupBy} onChange={(e) => { setParam("group", e.target.value === "category" ? "category" : null); }} className="max-w-48">
              <option value="status">{t.groupStatus}</option>
              <option value="category">{t.groupCategory}</option>
            </Select>
          )}
          <div role="tablist" aria-label="view" className="ml-auto inline-flex rounded-md ring-1 ring-zinc-300 dark:ring-zinc-700">
            {(["table", "board"] as const).map((v) => (
              <button
                key={v}
                role="tab"
                type="button"
                aria-selected={view === v}
                onClick={() => { setParam("view", v === "table" ? null : v); }}
                className={cn("px-3 py-1.5 text-sm first:rounded-l-md last:rounded-r-md", view === v ? "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900" : "text-zinc-600 dark:text-zinc-300")}
              >
                {v === "table" ? t.viewTable : t.viewBoard}
              </button>
            ))}
          </div>
          {!canEdit && workspace.data && <span className="text-xs text-zinc-500">{t.readOnly}</span>}
        </div>

        <div className="min-h-0 flex-1">
          {view === "board" ? (
            <Board
              ws={ws}
              project={project}
              filters={filters}
              groupBy={groupBy}
              members={memberById}
              canEdit={canEdit}
              onOpen={(n) => { setParam("item", String(n)); }}
            />
          ) : items.isPending ? (
            <Spinner />
          ) : items.isError ? (
            <ErrorState error={items.error} onRetry={() => void items.refetch()} />
          ) : all.length === 0 ? (
            <EmptyState>{t.noItems}</EmptyState>
          ) : (
            <ItemTable
              items={all}
              members={memberById}
              hasMore={items.hasNextPage}
              loadingMore={items.isFetchingNextPage}
              loadMore={() => void items.fetchNextPage()}
              onOpen={(n) => { setParam("item", String(n)); }}
            />
          )}
        </div>
      </div>

      {openItem !== null && (
        <ItemDrawer ws={ws} project={project} n={openItem} canEdit={canEdit} isAdmin={roleAtLeast(workspace.data?.role, "admin")} onClose={() => { setParam("item", null); }} />
      )}
      <CreateItemDialog
        ws={ws}
        project={project}
        open={creating}
        onClose={() => { setCreating(false); }}
        onCreated={(it) => { setParam("item", String(it.number)); }}
      />
    </Layout>
  );
}

// ---------------------------------------------------------------------------
// Table: virtualized, so only visible rows are in the DOM (100k rows stay smooth)
// ---------------------------------------------------------------------------

const ROW_HEIGHT = 44;
// Narrow screens keep key, title and status; wider ones add type, assignee, time.
const GRID =
  "grid grid-cols-[5.5rem_minmax(8rem,1fr)_8rem] md:grid-cols-[6.5rem_minmax(12rem,1fr)_7rem_9rem_11rem] lg:grid-cols-[6.5rem_minmax(12rem,1fr)_7rem_9rem_11rem_8rem] items-center gap-3 px-4";

function ItemTable({ items, members, hasMore, loadingMore, loadMore, onOpen }: {
  items: Item[]; members: Map<string, Member>; hasMore: boolean; loadingMore: boolean; loadMore: () => void; onOpen: (n: number) => void;
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

  return (
    <div className="flex h-full flex-col" role="table" aria-rowcount={items.length}>
      <div role="row" className={cn(GRID, "h-9 shrink-0 border-b border-zinc-200 text-xs font-medium tracking-wide text-zinc-500 uppercase dark:border-zinc-800")}>
        <span role="columnheader">#</span>
        <span role="columnheader">{t.title}</span>
        <span role="columnheader" className="hidden md:block">{t.type}</span>
        <span role="columnheader">{t.status}</span>
        <span role="columnheader" className="hidden md:block">{t.assignee}</span>
        <span role="columnheader" className="hidden lg:block">{t.updated}</span>
      </div>
      <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto">
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
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
                    className={cn(GRID, "h-full w-full border-b border-zinc-100 text-left text-sm hover:bg-zinc-100/70 dark:border-zinc-800/60 dark:hover:bg-zinc-800/60")}
                  >
                    <span role="cell" className="font-mono text-xs text-zinc-500">{it.key}</span>
                    <span role="cell" className="flex min-w-0 items-center gap-2">
                      <span className="truncate">{it.title}</span>
                      {it.due_date && <DueChip date={it.due_date} overdue={it.overdue} />}
                    </span>
                    <span role="cell" className="hidden truncate text-zinc-600 md:block dark:text-zinc-400">{it.type.name}</span>
                    <span role="cell"><StatusBadge name={it.status.name} category={it.status.category} /></span>
                    <span role="cell" className="hidden min-w-0 items-center gap-2 md:flex">
                      <AssigneeCell id={it.assignee_id} members={members} />
                    </span>
                    <span role="cell" className="hidden text-xs text-zinc-500 lg:block" title={formatDateTime(it.updated_at)}>{formatRelative(it.updated_at)}</span>
                  </button>
                ) : (
                  <div className="px-4 py-3 text-sm text-zinc-500">{t.loadingMore}</div>
                )}
              </div>
            );
          })}
        </div>
      </div>
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

function Board({ ws, project, filters, groupBy, members, canEdit, onOpen }: {
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
          const base = columnCount(c, counts.data?.by_status);
          const delta = pending.filter((m) => m.to === c.key).length - pending.filter((m) => m.from === c.key).length;
          return (
            <BoardColumnView
              key={c.key}
              ws={ws}
              project={project}
              filters={filters}
              column={c}
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
