// /w/:ws/absence — "Məzuniyyət və əvəzləmə" (ADR-0042): the viewer's own
// current and upcoming absences in this workspace, and the form to add one.
import { useState } from "react";
import { useParams } from "react-router";

import { useAbsences } from "../api/absences";
import { useMe, useWorkspace } from "../api/hooks";
import { AbsenceDialog, AbsenceList } from "../components/Absence";
import { Layout } from "../components/Layout";
import { Button, EmptyState, ErrorState, Spinner } from "../components/ui";
import { t } from "../i18n";
import { tab } from "../i18n-absences";

export function AbsencePage() {
  const { ws = "" } = useParams();
  const workspace = useWorkspace(ws);
  const me = useMe();
  const guest = workspace.data?.role === "guest";
  const list = useAbsences(ws, workspace.data !== undefined && !guest);
  const [adding, setAdding] = useState(false);
  const mine = (list.data ?? []).filter((a) => a.user.id === me.data?.id);

  let body;
  if (guest) body = <EmptyState>{tab.guest}</EmptyState>;
  else if (workspace.isPending || list.isPending) body = <Spinner />;
  else if (workspace.isError) body = <ErrorState error={workspace.error} onRetry={() => void workspace.refetch()} />;
  else if (list.isError) body = <ErrorState error={list.error} onRetry={() => void list.refetch()} />;
  else body = mine.length === 0 ? <EmptyState>{tab.none}</EmptyState> : <AbsenceList ws={ws} list={mine} />;

  return (
    <Layout crumbs={[{ label: t.workspaces, to: "/" }, { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` }, { label: tab.title }]}>
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-2xl space-y-4 p-4 md:p-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0 flex-1">
              <h1 className="text-lg font-semibold">{tab.title}</h1>
              <p className="text-sm text-zinc-500 dark:text-zinc-400">{tab.hint}</p>
            </div>
            {!guest && workspace.data && !workspace.data.archived_at && (
              <Button variant="primary" className="self-start" onClick={() => { setAdding(true); }}>{tab.add}</Button>
            )}
          </div>
          <section aria-labelledby="absence-list-heading" className="rounded-xl bg-white px-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
            <h2 id="absence-list-heading" className="pt-3 text-sm font-semibold">{tab.list}</h2>
            {body}
          </section>
        </div>
      </div>
      <AbsenceDialog ws={ws} open={adding} onClose={() => { setAdding(false); }} />
    </Layout>
  );
}
