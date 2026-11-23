import { DndContext, KeyboardSensor, PointerSensor, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent, type DragStartEvent } from "@dnd-kit/core";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { api, ApiError, path } from "../api/client";
import { keys, useColumnItems, useFields, useItemCounts, useItems, useItemSums, useItemTypes, useMembers, useProjects, useUpdateItem, useWorkspace, type DueFilter, type ItemFilters } from "../api/hooks";
import { roleAtLeast, type Item, type Member, type SavedView, type Status, type StatusCategory } from "../api/types";
import { useViews } from "../api/views";
import { Layout } from "../components/Layout";
import { useToast } from "../components/toast-context";
import { Avatar, Button, DueChip, EmptyState, ErrorState, Input, Select, Spinner, StatusBadge } from "../components/ui";
import { tv } from "../i18n-views";
import { cn } from "../lib/cn";
import { t } from "../i18n";
import { tr } from "../i18n-recur";
import { ti } from "../i18n-import";
import { buildColumns, columnCount, dropTargets, flattenPages, otherColumn, pickTargetStatus, type BoardColumn, type GroupBy } from "../lib/board";
import { emptyState, filtersOf, hasState, readState, sameState, stateFromView, sumFields, visibleColumns, writeState, type PageState } from "../lib/views";
import { CreateItemDialog } from "./CreateItemDialog";
import { ItemDrawer } from "./ItemDrawer";
import { ItemTable } from "./ItemTable";
import { ColumnPicker, FilterPicker, ViewSwitcher } from "./ViewTools";

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
  // Filters, columns, sort and grouping live in the URL (lib/views), so any
  // state is a shareable link; ?saved=<id> names the saved view it came from.
  const state = useMemo(() => readState(params), [params]);
  const view = state.mode;
  const groupBy = state.group;
  const savedId = params.get("saved");
  const openItem = Number(params.get("item")) || null;
  const [search, setSearch] = useState(state.q);
  const q = useDebounced(search, 300);
  // The URL's q changed from outside the search box (a view was applied,
  // back/forward): show it. Typing changes it to the debounced value itself.
  const [urlQ, setUrlQ] = useState(state.q);
  if (state.q !== urlQ) {
    setUrlQ(state.q);
    if (state.q !== q) setSearch(state.q);
  }
  const [creating, setCreating] = useState(false);
  const toast = useToast();

  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const members = useMembers(ws);
  const fieldDefs = useFields(ws);
  const types = useItemTypes(ws);
  const views = useViews(ws, project);
  const filters = useMemo(() => filtersOf(state, q), [state, q]);
  const items = useItems(ws, project, filters, view === "table");
  const canEdit = roleAtLeast(workspace.data?.role, "member");
  const isAdmin = roleAtLeast(workspace.data?.role, "admin");
  const canImport = isAdmin;
  const projectName = projects.data?.find((p) => p.key === project)?.name ?? project;

  const fields = useMemo(() => new Map((fieldDefs.data ?? []).map((f) => [f.key, f])), [fieldDefs.data]);
  const columns = useMemo(() => visibleColumns(state.columns, fields), [state.columns, fields]);
  const sumKeys = useMemo(() => (view === "table" ? sumFields(columns, fields) : []), [view, columns, fields]);
  const sums = useItemSums(ws, project, filters, sumKeys);
  const statuses = useMemo(() => {
    const seen = new Map<string, Status>();
    for (const it of types.data ?? []) for (const st of it.statuses) if (!seen.has(st.id)) seen.set(st.id, st);
    return [...seen.values()];
  }, [types.data]);
  const active = views.data?.find((v) => v.id === savedId);
  const dirty = active !== undefined && !sameState(state, stateFromView(active));

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
  /** Replaces the page state, keeping ?item and (unless given) ?saved. */
  const applyState = (patch: Partial<PageState>, saved?: string | null) => {
    setParams(
      (prev) => {
        const next = writeState({ ...readState(prev), ...patch }, prev);
        if (saved === null) next.delete("saved");
        else if (saved) next.set("saved", saved);
        return next;
      },
      { replace: saved === undefined },
    );
  };
  const selectView = (v: SavedView | null) => {
    applyState(v ? stateFromView(v) : emptyState(), v ? v.id : null);
  };

  useEffect(() => {
    setParam("q", q || null);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- sync only when the debounced query changes
  }, [q]);

  // A bare ?saved=<id> link (bookmark, shared URL) applies the view's state;
  // a view that is gone or not shared with this user is dropped with a note.
  useEffect(() => {
    if (!savedId || !views.data) return;
    const v = views.data.find((x) => x.id === savedId);
    if (!v) {
      toast("error", tv.viewMissing);
      setParam("saved", null);
    } else if (!hasState(params)) {
      applyState(stateFromView(v));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run when the view list or the selected view changes
  }, [savedId, views.data]);

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
          <ViewSwitcher
            ws={ws}
            project={project}
            views={views.data ?? []}
            active={active}
            dirty={dirty}
            state={state}
            isAdmin={isAdmin}
            onSelect={selectView}
          />
          <Input
            type="search"
            aria-label={t.search}
            placeholder={t.search}
            value={search}
            onChange={(e) => { setSearch(e.target.value); }}
            className="max-w-64"
            maxLength={500}
          />
          <Select aria-label={t.assignee} value={state.assignee} onChange={(e) => { applyState({ assignee: e.target.value }); }} className="max-w-56">
            <option value="">{t.filterAll}</option>
            <option value="me">{t.filterMine}</option>
            <option value="none">{t.filterNone}</option>
            {/* A view may filter by a specific person. */}
            {state.assignee && state.assignee !== "me" && state.assignee !== "none" && (
              <option value={state.assignee}>{memberById.get(state.assignee)?.display_name ?? "…"}</option>
            )}
          </Select>
          <Select aria-label={t.dueFilter} value={state.due} onChange={(e) => { applyState({ due: DUE_FILTERS.find((d) => d === e.target.value) ?? "" }); }} className="max-w-48">
            <option value="">{t.dueAll}</option>
            <option value="overdue">{t.dueOverdue}</option>
            <option value="today">{t.dueToday}</option>
            <option value="week">{t.dueWeek}</option>
            <option value="none">{t.dueNone}</option>
          </Select>
          <FilterPicker
            statuses={statuses}
            fields={fields}
            status={state.status}
            values={state.fields}
            onStatus={(ids) => { applyState({ status: [...ids].sort() }); }}
            onField={(key, value) => {
              const rest = Object.fromEntries(Object.entries(state.fields).filter(([k]) => k !== key));
              applyState({ fields: value ? { ...rest, [key]: value } : rest });
            }}
            onClear={() => { applyState({ status: [], fields: {} }); }}
          />
          <Select aria-label={tv.sort} value={state.sort} onChange={(e) => { applyState({ sort: e.target.value === "due" ? "due" : "number" }); }} className="max-w-48">
            <option value="number">{tv.sortNumber}</option>
            <option value="due">{tv.sortDue}</option>
          </Select>
          {view === "table" && <ColumnPicker columns={columns} fields={fields} onChange={(c) => { applyState({ columns: c }); }} />}
          {view === "board" && (
            <Select aria-label={t.groupBy} value={groupBy} onChange={(e) => { applyState({ group: e.target.value === "category" ? "category" : "status" }); }} className="max-w-48">
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
                onClick={() => { applyState({ mode: v }); }}
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
              columns={columns}
              fields={fields}
              members={memberById}
              sums={sums.data}
              hasMore={items.hasNextPage}
              loadingMore={items.isFetchingNextPage}
              loadMore={() => void items.fetchNextPage()}
              onOpen={(n) => { setParam("item", String(n)); }}
            />
          )}
        </div>
      </div>

      {openItem !== null && (
        <ItemDrawer ws={ws} project={project} n={openItem} canEdit={canEdit} isAdmin={isAdmin} onClose={() => { setParam("item", null); }} />
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

const DUE_FILTERS: readonly DueFilter[] = ["overdue", "today", "week", "none"];

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
