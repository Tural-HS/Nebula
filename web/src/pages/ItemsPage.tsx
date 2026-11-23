import { DndContext, KeyboardSensor, PointerSensor, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { useQueryClient } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useMemo, useRef, useState } from "react";
import { useParams, useSearchParams } from "react-router";

import { api, ApiError, path } from "../api/client";
import { keys, useItems, useItemTypes, useMembers, useProjects, useUpdateItem, useWorkspace, type ItemFilters } from "../api/hooks";
import { roleAtLeast, type Item, type Member, type Status, type StatusCategory } from "../api/types";
import { Layout } from "../components/Layout";
import { useToast } from "../components/toast-context";
import { Avatar, Button, EmptyState, ErrorState, Input, Select, Spinner, StatusBadge } from "../components/ui";
import { cn } from "../lib/cn";
import { t } from "../i18n";
import { CATEGORIES, flattenPages, groupByCategory, pickTargetStatus } from "../lib/board";
import { formatDateTime, formatRelative } from "../lib/format";
import { CreateItemDialog } from "./CreateItemDialog";
import { ItemDrawer } from "./ItemDrawer";

const BOARD_LIMIT = 500;

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
  const openItem = Number(params.get("item")) || null;
  const assigneeParam = params.get("assignee");
  const assignee: ItemFilters["assignee"] = assigneeParam === "me" || assigneeParam === "none" ? assigneeParam : "";
  const [search, setSearch] = useState(params.get("q") ?? "");
  const q = useDebounced(search, 300);
  const [creating, setCreating] = useState(false);

  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const members = useMembers(ws);
  const items = useItems(ws, project, { q, assignee });
  const canEdit = roleAtLeast(workspace.data?.role, "member");
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
        canEdit && (
          <Button variant="primary" onClick={() => { setCreating(true); }}>
            {t.newItem}
          </Button>
        )
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
          {items.isPending ? (
            <Spinner />
          ) : items.isError ? (
            <ErrorState error={items.error} onRetry={() => void items.refetch()} />
          ) : all.length === 0 ? (
            <EmptyState>{t.noItems}</EmptyState>
          ) : view === "table" ? (
            <ItemTable
              items={all}
              members={memberById}
              hasMore={items.hasNextPage}
              loadingMore={items.isFetchingNextPage}
              loadMore={() => void items.fetchNextPage()}
              onOpen={(n) => { setParam("item", String(n)); }}
            />
          ) : (
            <Board
              ws={ws}
              project={project}
              items={all}
              members={memberById}
              canEdit={canEdit}
              hasMore={items.hasNextPage}
              loadMore={() => void items.fetchNextPage()}
              onOpen={(n) => { setParam("item", String(n)); }}
            />
          )}
        </div>
      </div>

      {openItem !== null && (
        <ItemDrawer ws={ws} project={project} n={openItem} canEdit={canEdit} onClose={() => { setParam("item", null); }} />
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
                    <span role="cell" className="truncate">{it.title}</span>
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
// Board: columns by status category; drag a card to change its status
// ---------------------------------------------------------------------------

function Board({ ws, project, items, members, canEdit, hasMore, loadMore, onOpen }: {
  ws: string; project: string; items: Item[]; members: Map<string, Member>; canEdit: boolean;
  hasMore: boolean; loadMore: () => void; onOpen: (n: number) => void;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const types = useItemTypes(ws);
  const update = useUpdateItem(ws, project);
  // Optimistic placement while the server confirms the move.
  const [pending, setPending] = useState<Record<string, Status>>({});

  useEffect(() => {
    if (hasMore && items.length < BOARD_LIMIT) loadMore();
  }, [hasMore, items.length, loadMore]);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }), // small moves stay clicks
    useSensor(KeyboardSensor),
  );

  const shown = items.slice(0, BOARD_LIMIT).map((it) => {
    const p = pending[it.id];
    return p ? { ...it, status: p } : it;
  });
  const columns = groupByCategory(shown);

  const onDragEnd = async ({ active, over }: DragEndEvent) => {
    if (!over || !canEdit) return;
    const item = items.find((i) => i.id === active.id);
    const category = over.id as StatusCategory;
    if (!item || item.status.category === category) return;
    try {
      const allowed = await qc.query({
        queryKey: keys.transitions(ws, project, item.number),
        queryFn: async () => (await api<{ statuses: Status[] }>("GET", path`/workspaces/${ws}/projects/${project}/items/${item.number}/transitions`)).statuses,
        staleTime: 0,
      });
      const target = pickTargetStatus(item, category, types.data ?? [], allowed);
      if (!target) {
        toast("error", t.transitionNotAllowed);
        return;
      }
      setPending((p) => ({ ...p, [item.id]: target }));
      await update.mutateAsync({ n: item.number, patch: { version: item.version, status_id: target.id } });
    } catch (err) {
      toast("error", err instanceof ApiError && err.isConflict ? t.conflict : err instanceof ApiError ? err.message : t.errorGeneric);
    } finally {
      setPending((p) => Object.fromEntries(Object.entries(p).filter(([id]) => id !== item.id)));
    }
  };

  return (
    <DndContext sensors={sensors} onDragEnd={(e) => void onDragEnd(e)}>
      <div className="flex h-full flex-col">
        {items.length > BOARD_LIMIT && <p className="px-4 pt-3 text-xs text-zinc-500">{t.boardLimit(BOARD_LIMIT)}</p>}
        <div className="flex min-h-0 flex-1 gap-3 overflow-x-auto p-4">
          {CATEGORIES.map((c) => (
            <Column key={c} category={c} count={columns[c].length}>
              {columns[c].map((it) => (
                <Card key={it.id} item={it} members={members} draggable={canEdit} onOpen={onOpen} />
              ))}
            </Column>
          ))}
        </div>
      </div>
    </DndContext>
  );
}

function Column({ category, count, children }: { category: StatusCategory; count: number; children: React.ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id: category });
  return (
    <section
      ref={setNodeRef}
      aria-label={t.category[category]}
      className={cn("flex min-h-0 w-72 min-w-64 flex-1 shrink-0 flex-col rounded-xl bg-zinc-100 dark:bg-zinc-900", isOver && "ring-2 ring-brand-500")}
    >
      <h2 className="flex items-center justify-between px-3 py-2 text-xs font-semibold tracking-wide text-zinc-600 uppercase dark:text-zinc-400">
        {t.category[category]}
        <span className="rounded-full bg-zinc-200 px-1.5 text-[11px] dark:bg-zinc-800">{count}</span>
      </h2>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-2 pb-2">{children}</div>
    </section>
  );
}

function Card({ item, members, draggable, onOpen }: { item: Item; members: Map<string, Member>; draggable: boolean; onOpen: (n: number) => void }) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: item.id, disabled: !draggable });
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
        isDragging && "z-10 opacity-80 shadow-lg",
      )}
    >
      <p className="line-clamp-3">{item.title}</p>
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
