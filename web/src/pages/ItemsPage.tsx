import { lazy, Suspense, useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { MAX_BULK, useOptimisticUpdate } from "../api/bulk";
import { ApiError } from "../api/client";
import { useAssignable, useFields, useItems, useItemSums, useItemTypes, useMembers, useProjects, useWorkspace, type DueFilter } from "../api/hooks";
import { roleAtLeast, type Item, type SavedView, type Status } from "../api/types";
import { useViews } from "../api/views";
import { Layout } from "../components/Layout";
import { useToast } from "../components/toast-context";
import { Button, Dialog, EmptyState, ErrorState, Input, Select, Spinner } from "../components/ui";
import { tv } from "../i18n-views";
import { cn } from "../lib/cn";
import { t } from "../i18n";
import { tap } from "../i18n-approvals";
import { tp } from "../i18n-project";
import { tr } from "../i18n-recur";
import { ti } from "../i18n-import";
import { tm } from "../i18n-mobile";
import { tc } from "../i18n-calendar";
import { monthOf, parseMonth, todayIn } from "../lib/calendar";
import { activeFilterCount, useCompact } from "../lib/responsive";
import { flattenPages } from "../lib/board";
import { emptyState, filtersOf, hasState, readState, sameState, stateFromView, sumFields, visibleColumns, writeState, type PageState } from "../lib/views";
import { CreateItemDialog } from "./CreateItemDialog";
import { BulkBar } from "./BulkBar";
import { ItemTable, type TableSelection } from "./ItemTable";
import type { InlineEdit } from "../lib/inline";
import { ColumnPicker, FilterPicker, Popover, ViewSwitcher } from "./ViewTools";

// The board (drag and drop) and the item panel (comments, activity) load on
// demand: the table needs neither for its first paint.
const Board = lazy(async () => ({ default: (await import("./BoardView")).Board }));
const ItemDrawer = lazy(async () => ({ default: (await import("./ItemDrawer")).ItemDrawer }));
const CalendarView = lazy(async () => ({ default: (await import("./CalendarView")).CalendarView }));

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
  const [sheet, setSheet] = useState(false);
  const compact = useCompact();
  // Rotated to a wider layout with the sheet open: it is gone, do not
  // reopen it on the next rotation.
  if (!compact && sheet) setSheet(false);
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
  const assignable = useAssignable(ws, project);
  const today = todayIn(workspace.data?.timezone);
  const month = parseMonth(params.get("cal")) ?? monthOf(today);

  // Bulk selection (ADR-0023): item numbers of the current filtered list. A
  // different filter or view is a different list, so the selection resets.
  const listKey = JSON.stringify([filters, view]);
  const [selection, setSelection] = useState<{ key: string; numbers: Set<number> }>({ key: listKey, numbers: new Set() });
  const selectedNumbers = selection.key === listKey ? selection.numbers : EMPTY;
  const selectedItems = useMemo(() => all.filter((it) => selectedNumbers.has(it.number)), [all, selectedNumbers]);
  const setSelected = (numbers: Set<number>) => { setSelection({ key: listKey, numbers }); };
  const [selectingAll, setSelectingAll] = useState(false);
  const selectAll = async () => {
    // Every row of the filtered result, loading further pages up to the cap.
    setSelectingAll(true);
    try {
      let list: Item[] = all;
      let more = items.hasNextPage;
      while (more && list.length < MAX_BULK) {
        const r = await items.fetchNextPage();
        list = flattenPages(r.data?.pages ?? []);
        more = r.hasNextPage;
      }
      setSelected(new Set(list.slice(0, MAX_BULK).map((it) => it.number)));
      if (list.length > MAX_BULK || more) toast("success", tc.bulkCap(MAX_BULK));
    } finally {
      setSelectingAll(false);
    }
  };
  const tableSelection: TableSelection | undefined = canEdit
    ? {
        selected: selectedNumbers,
        state: selectedNumbers.size === 0 ? "none" : selectedNumbers.size >= Math.min(all.length, MAX_BULK) && !items.hasNextPage ? "all" : "some",
        toggle: (n) => {
          const next = new Set(selectedNumbers);
          if (next.has(n)) next.delete(n);
          else if (next.size < MAX_BULK) next.add(n);
          else toast("error", tc.bulkCap(MAX_BULK));
          setSelected(next);
        },
        toggleAll: () => {
          if (selectingAll) return;
          if (selectedNumbers.size > 0) setSelected(new Set());
          else void selectAll();
        },
      }
    : undefined;

  const optimistic = useOptimisticUpdate(ws, project);
  const inline: InlineEdit | undefined = canEdit
    ? {
        ws,
        project,
        today,
        assignable,
        commit: (item, patch, next) => {
          optimistic.mutate(
            { item, patch, optimistic: next },
            { onError: (err) => { toast("error", err instanceof ApiError && err.isConflict ? t.conflict : err instanceof ApiError ? `${tc.saveFailed}: ${err.message}` : tc.saveFailed); } },
          );
        },
      }
    : undefined;

  // Toolbar controls, laid out in one row from md up and in a bottom sheet
  // on phones (only one layout is rendered at a time, ADR-0019).
  const base = `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project)}`;
  const links = [
    ...(canEdit ? [{ to: `${base}/recurring`, label: tr.nav }] : []),
    ...(canImport ? [{ to: `${base}/settings`, label: tp.settings }, { to: `${base}/import`, label: ti.importAction }] : []),
  ];
  const filterCount = activeFilterCount(state);
  const viewSwitcher = (
    <ViewSwitcher ws={ws} project={project} views={views.data ?? []} active={active} dirty={dirty} state={state} isAdmin={isAdmin} onSelect={selectView} />
  );
  const searchBox = (className: string) => (
    <Input
      type="search"
      aria-label={t.search}
      placeholder={t.search}
      value={search}
      onChange={(e) => { setSearch(e.target.value); }}
      className={className}
      maxLength={500}
    />
  );
  const assigneeSelect = (className: string) => (
    <Select aria-label={t.assignee} value={state.assignee} onChange={(e) => { applyState({ assignee: e.target.value }); }} className={className}>
      <option value="">{t.filterAll}</option>
      <option value="me">{t.filterMine}</option>
      <option value="none">{t.filterNone}</option>
      {/* A view may filter by a specific person. */}
      {state.assignee && state.assignee !== "me" && state.assignee !== "none" && (
        <option value={state.assignee}>{memberById.get(state.assignee)?.display_name ?? "…"}</option>
      )}
    </Select>
  );
  const dueSelect = (className: string) => (
    <Select aria-label={t.dueFilter} value={state.due} onChange={(e) => { applyState({ due: DUE_FILTERS.find((d) => d === e.target.value) ?? "" }); }} className={className}>
      <option value="">{t.dueAll}</option>
      <option value="overdue">{t.dueOverdue}</option>
      <option value="today">{t.dueToday}</option>
      <option value="week">{t.dueWeek}</option>
      <option value="none">{t.dueNone}</option>
    </Select>
  );
  const approvalSelect = (className: string) => (
    <Select aria-label={tap.filter.label} value={state.approval} onChange={(e) => { applyState({ approval: e.target.value === "pending" || e.target.value === "mine" ? e.target.value : "" }); }} className={className}>
      <option value="">{tap.filter.all}</option>
      <option value="pending">{tap.filter.pending}</option>
      <option value="mine">{tap.filter.mine}</option>
    </Select>
  );
  const filterPicker = (inline: boolean) => (
    <FilterPicker
      inline={inline}
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
  );
  const sortSelect = (className: string) => (
    <Select aria-label={tv.sort} value={state.sort} onChange={(e) => { applyState({ sort: e.target.value === "due" ? "due" : "number" }); }} className={className}>
      <option value="number">{tv.sortNumber}</option>
      <option value="due">{tv.sortDue}</option>
    </Select>
  );
  const columnPicker = (inline: boolean) => <ColumnPicker inline={inline} columns={columns} fields={fields} onChange={(c) => { applyState({ columns: c }); }} />;
  const groupSelect = (className: string) => (
    <Select aria-label={t.groupBy} value={groupBy} onChange={(e) => { applyState({ group: e.target.value === "category" ? "category" : "status" }); }} className={className}>
      <option value="status">{t.groupStatus}</option>
      <option value="category">{t.groupCategory}</option>
    </Select>
  );
  const viewTabs = (
    <div role="tablist" aria-label="view" className="inline-flex shrink-0 rounded-md ring-1 ring-zinc-300 dark:ring-zinc-700">
      {(["table", "board", "calendar"] as const).map((v) => (
        <button
          key={v}
          role="tab"
          type="button"
          aria-selected={view === v}
          onClick={() => { applyState({ mode: v }); }}
          className={cn(
            "px-3 py-1.5 text-sm first:rounded-l-md last:rounded-r-md pointer-coarse:min-h-11 max-md:px-2.5",
            view === v ? "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900" : "text-zinc-600 dark:text-zinc-300",
          )}
        >
          {v === "table" ? t.viewTable : v === "board" ? t.viewBoard : tc.viewCalendar}
        </button>
      ))}
    </div>
  );
  const readOnly = !canEdit && workspace.data ? <span className="text-xs text-zinc-500">{t.readOnly}</span> : null;

  return (
    <Layout
      crumbs={[
        { label: t.workspaces, to: "/" },
        { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` },
        { label: projectName },
      ]}
      actions={
        <>
          {compact ? (
            // Phones: the project links share one menu, so the header never
            // wraps or scrolls sideways.
            links.length > 0 && (
              <Popover label={<span aria-hidden>⋯</span>} summaryLabel={tm.moreActions} align="right">
                <nav aria-label={tm.moreActions} className="flex flex-col">
                  {links.map((l) => (
                    <Link key={l.to} to={l.to} className={cn(HEADER_LINK, "min-h-11 content-center")}>
                      {l.label}
                    </Link>
                  ))}
                </nav>
              </Popover>
            )
          ) : (
            links.map((l) => (
              <Link key={l.to} to={l.to} className={HEADER_LINK}>
                {l.label}
              </Link>
            ))
          )}
          {canEdit && (
            <Button variant="primary" aria-label={compact ? t.newItem : undefined} className="pointer-coarse:min-w-11" onClick={() => { setCreating(true); }}>
              {compact ? <span aria-hidden className="text-lg leading-none">+</span> : t.newItem}
            </Button>
          )}
        </>
      }
    >
      <div className="flex h-full flex-col">
        {compact ? (
          <div className="flex items-center gap-2 border-b border-zinc-200 bg-white px-3 py-2 dark:border-zinc-800 dark:bg-zinc-900">
            {searchBox("min-w-0 flex-1")}
            <Button aria-haspopup="dialog" onClick={() => { setSheet(true); }} className="shrink-0">
              {tv.filters}
              {filterCount > 0 && (
                <span className="rounded-full bg-brand-600 px-1.5 text-[11px] text-white tabular-nums">{filterCount}</span>
              )}
            </Button>
            {viewTabs}
          </div>
        ) : (
          <div className="flex flex-wrap items-center gap-2 border-b border-zinc-200 bg-white px-4 py-2 dark:border-zinc-800 dark:bg-zinc-900">
            {viewSwitcher}
            {searchBox("max-w-64")}
            {assigneeSelect("max-w-56")}
            {dueSelect("max-w-48")}
            {approvalSelect("max-w-56")}
            {filterPicker(false)}
            {view !== "calendar" && sortSelect("max-w-48")}
            {view === "table" && columnPicker(false)}
            {view === "board" && groupSelect("max-w-48")}
            <div className="ml-auto">{viewTabs}</div>
            {readOnly}
          </div>
        )}
        {compact && (
          <Dialog variant="sheet" open={sheet} onClose={() => { setSheet(false); }} title={tm.filtersSheet}>
            <div className="flex flex-col gap-4">
              <SheetGroup label={tv.views}>{viewSwitcher}</SheetGroup>
              <SheetGroup label={t.assignee}>{assigneeSelect("")}</SheetGroup>
              <SheetGroup label={t.dueFilter}>{dueSelect("")}</SheetGroup>
              <SheetGroup label={tap.filter.label}>{approvalSelect("")}</SheetGroup>
              {filterPicker(true)}
              {view !== "calendar" && <SheetGroup label={tv.sort}>{sortSelect("")}</SheetGroup>}
              {view === "board" && <SheetGroup label={t.groupBy}>{groupSelect("")}</SheetGroup>}
              {view === "table" && columnPicker(true)}
              {readOnly}
            </div>
          </Dialog>
        )}

        {view === "table" && !compact && selectedItems.length > 0 && (
          <BulkBar
            ws={ws}
            project={project}
            projectKey={project}
            selected={selectedItems}
            statuses={statuses}
            assignable={assignable}
            fields={fields}
            isAdmin={isAdmin}
            onClear={() => { setSelected(new Set()); }}
          />
        )}
        <div className="min-h-0 flex-1">
          {view === "calendar" ? (
            <Suspense fallback={<Spinner />}>
              <CalendarView
                ws={ws}
                project={project}
                filters={filters}
                month={month}
                today={today}
                onMonth={(m) => { setParam("cal", m === monthOf(today) ? null : m); }}
                members={memberById}
                canEdit={canEdit}
                compact={compact}
                onOpen={(n) => { setParam("item", String(n)); }}
              />
            </Suspense>
          ) : view === "board" ? (
            <Suspense fallback={<Spinner />}>
            <Board
              ws={ws}
              project={project}
              filters={filters}
              groupBy={groupBy}
              members={memberById}
              canEdit={canEdit}
              onOpen={(n) => { setParam("item", String(n)); }}
            />
            </Suspense>
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
              selection={tableSelection}
              inline={inline}
            />
          )}
        </div>
      </div>

      {openItem !== null && (
        <Suspense fallback={null}>
          <ItemDrawer ws={ws} project={project} n={openItem} canEdit={canEdit} isAdmin={isAdmin} onClose={() => { setParam("item", null); }} />
        </Suspense>
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
const EMPTY: ReadonlySet<number> = new Set();

const HEADER_LINK = "rounded-md px-3 py-1.5 text-sm whitespace-nowrap text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800";

/** A labelled group in the phone filter sheet; the controls keep their own accessible names. */
function SheetGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <p aria-hidden className="mb-1 text-xs font-medium text-zinc-600 dark:text-zinc-400">{label}</p>
      {children}
    </div>
  );
}
