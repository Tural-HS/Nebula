// /w/:ws/approvals — the workspace-wide approval inbox (ADR-0034): pending
// requests of every project the viewer can see, oldest first. "Mine" (the
// default) lists only what the viewer may decide on now; the server decides
// that with the same rule as the decision itself, so nothing here can leak a
// private project. Each row opens the item drawer at its approval panel.
import { Link, useParams, useSearchParams } from "react-router";

import { useApprovalInbox, type InboxEntry } from "../api/approvals";
import { useWorkspace } from "../api/hooks";
import { DueChip } from "../components/Approvals";
import { Layout } from "../components/Layout";
import { Button, EmptyState, ErrorState, Spinner, StatusBadge } from "../components/ui";
import { t } from "../i18n";
import { tai } from "../i18n-approval-inbox";
import { tap } from "../i18n-approvals";
import { ageDays, inboxHref, overdueFirst } from "../lib/approvalInbox";
import { progress } from "../lib/approvals";
import { cn } from "../lib/cn";
import { flattenBy } from "../lib/collections";
import { formatDateTime } from "../lib/format";

export function ApprovalsInboxPage() {
  const { ws = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const mine = params.get("view") !== "all";
  const workspace = useWorkspace(ws);
  const q = useApprovalInbox(ws, mine);

  let body;
  if (q.isPending) body = <Spinner />;
  else if (q.isError) body = <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  else {
    const list = overdueFirst(flattenBy(q.data.pages, (p) => p.approvals, (e) => e.id));
    body = list.length === 0 ? (
      <EmptyState>{mine ? tai.empty.mine : tai.empty.all}</EmptyState>
    ) : (
      <>
        <ul data-testid="approval-inbox" className="divide-y divide-zinc-100 rounded-xl bg-white ring-1 ring-zinc-200 dark:divide-zinc-800 dark:bg-zinc-900 dark:ring-zinc-800">
          {list.map((e) => <Row key={e.id} ws={ws} e={e} />)}
        </ul>
        {q.hasNextPage && (
          <Button className="mt-4" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
            {q.isFetchingNextPage ? t.loadingMore : tai.more}
          </Button>
        )}
      </>
    );
  }

  const tab = (value: "mine" | "all") => (
    <button
      type="button"
      role="tab"
      aria-selected={mine === (value === "mine")}
      onClick={() => {
        setParams((prev) => {
          const next = new URLSearchParams(prev);
          if (value === "all") next.set("view", "all");
          else next.delete("view");
          return next;
        }, { replace: true });
      }}
      className={cn(
        "rounded-md px-3 py-1.5 text-sm pointer-coarse:min-h-11",
        mine === (value === "mine") ? "bg-brand-600 text-white" : "text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800",
      )}
    >
      {tai.tabs[value]}
    </button>
  );

  return (
    <Layout crumbs={[{ label: t.workspaces, to: "/" }, { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` }, { label: tai.title }]}>
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-3xl space-y-4 p-4 md:p-6">
          <div>
            <h1 className="text-lg font-semibold">{tai.title}</h1>
            <p className="text-sm text-zinc-500 dark:text-zinc-400">{tai.hint}</p>
          </div>
          <div role="tablist" aria-label={tai.title} className="flex flex-wrap gap-1">
            {tab("mine")}
            {tab("all")}
          </div>
          {body}
        </div>
      </div>
    </Layout>
  );
}

function Row({ ws, e }: { ws: string; e: InboxEntry }) {
  const p = progress(e.approvals, e.min_approvals);
  return (
    <li>
      <Link
        to={inboxHref(ws, e)}
        data-testid="approval-inbox-row"
        className={cn("flex flex-col gap-1.5 p-3 hover:bg-zinc-50 dark:hover:bg-zinc-800/50", e.overdue && "border-l-4 border-red-500")}
      >
        <span className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 font-mono text-xs text-zinc-500 dark:text-zinc-400">{e.item.key}</span>
          <span className="min-w-0 flex-1 truncate text-sm font-medium">{e.item.title}</span>
          <span className="shrink-0 text-xs text-zinc-500 dark:text-zinc-400 tabular-nums" aria-label={tap.progress(p.done, p.min)}>{p.done}/{p.min}</span>
        </span>
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-zinc-500 dark:text-zinc-400">
          {e.from_status && <StatusBadge name={e.from_status.name} category={e.from_status.category} />}
          <span aria-hidden>→</span>
          {e.to_status && <StatusBadge name={e.to_status.name} category={e.to_status.category} />}
          <span className="[overflow-wrap:anywhere]">{tai.requestedBy(e.requested_by.display_name)}</span>
          <span title={formatDateTime(e.created_at)}>· {tai.age(ageDays(e.created_at))}</span>
          <DueChip dueOn={e.due_on} overdue={e.overdue} />
          {e.can_decide && <span className="rounded bg-brand-50 px-1.5 py-0.5 font-medium text-brand-700 dark:bg-brand-900 dark:text-brand-100">{tai.yourTurn}</span>}
        </span>
      </Link>
    </li>
  );
}
