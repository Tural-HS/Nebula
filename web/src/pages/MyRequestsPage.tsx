import { useEffect } from "react";
import { Link, useParams } from "react-router";

import { useMarkSeen, useMyRequest, useMyRequests, type MyRequest } from "../api/forms";
import { Layout } from "../components/Layout";
import { Button, EmptyState, ErrorState, Spinner, StatusBadge } from "../components/ui";
import { t } from "../i18n";
import { tf } from "../i18n-forms";
import { formatBytes, formatDateTime, formatRelative } from "../lib/format";

/** /requests — the requester's own submissions (ADR-0032). Works without any workspace membership. */
export function MyRequestsPage() {
  const q = useMyRequests();
  const list = q.data?.pages.flatMap((p) => p.requests) ?? [];
  const unseen = q.data?.pages[0]?.unseen ?? 0;
  let body;
  if (q.isPending) body = <Spinner />;
  else if (q.isError) body = <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  else if (list.length === 0) body = <EmptyState>{tf.noRequests}</EmptyState>;
  else
    body = (
      <>
        <ul className="divide-y divide-zinc-100 rounded-xl bg-white ring-1 ring-zinc-200 dark:divide-zinc-800 dark:bg-zinc-900 dark:ring-zinc-800">
          {list.map((r) => <Row key={r.id} r={r} />)}
        </ul>
        {q.hasNextPage && (
          <Button className="mt-4" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
            {q.isFetchingNextPage ? t.loadingMore : tf.loadMore}
          </Button>
        )}
      </>
    );
  return (
    <Layout crumbs={[{ label: tf.myRequests }]}>
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-3xl space-y-4 p-4 md:p-6">
          <div>
            <h1 className="text-lg font-semibold">
              {tf.myRequests}
              {unseen > 0 && <span className="ml-2 rounded-full bg-brand-600 px-2 py-0.5 align-middle text-xs text-white">{tf.unseen(unseen)}</span>}
            </h1>
            <p className="text-sm text-zinc-500 dark:text-zinc-400">{tf.myRequestsHint}</p>
          </div>
          {body}
        </div>
      </div>
    </Layout>
  );
}

function Row({ r }: { r: MyRequest }) {
  return (
    <li>
      <Link to={`/requests/${encodeURIComponent(r.id)}`} className="flex flex-col gap-1 p-3 hover:bg-zinc-50 sm:flex-row sm:items-center sm:gap-3 dark:hover:bg-zinc-800/50">
        <span className="flex min-w-0 flex-1 items-center gap-2">
          <span className="shrink-0 font-mono text-xs text-zinc-500 dark:text-zinc-400">{r.key}</span>
          <span className="truncate text-sm font-medium">{r.title}</span>
          {r.unseen && <span className="shrink-0 rounded-full bg-brand-100 px-1.5 text-2xs font-medium text-brand-700 dark:bg-brand-700 dark:text-brand-50">{tf.newBadge}</span>}
        </span>
        <span className="flex shrink-0 items-center gap-2 text-xs text-zinc-500 dark:text-zinc-400">
          <RequestStatus r={r} />
          <time dateTime={r.updated_at} title={formatDateTime(r.updated_at)}>{formatRelative(r.updated_at)}</time>
        </span>
      </Link>
    </li>
  );
}

function RequestStatus({ r }: { r: MyRequest }) {
  if (r.removed || !r.status) return <StatusBadge name={tf.removed} category="canceled" />;
  return <StatusBadge name={r.status.name} category={r.status.category} />;
}

/** /requests/:id — one request: what was sent, the status and the team's message. */
export function MyRequestPage() {
  const { id = "" } = useParams();
  const q = useMyRequest(id);
  const seen = useMarkSeen();
  const unseen = q.data?.unseen === true;
  const { mutate } = seen;
  useEffect(() => {
    if (unseen) mutate(id);
  }, [unseen, id, mutate]);

  let body;
  if (q.isPending) body = <Spinner />;
  else if (q.isError) body = <ErrorState error={q.error} />;
  else {
    const d = q.data;
    body = (
      <article aria-labelledby="req-title" className="space-y-5">
        <header className="space-y-1">
          <p className="font-mono text-xs text-zinc-500 dark:text-zinc-400">{d.key} · {d.form_title} · {d.workspace}</p>
          <h1 id="req-title" className="text-lg font-semibold">{d.title}</h1>
          <p className="flex flex-wrap items-center gap-2 text-xs text-zinc-500 dark:text-zinc-400">
            <RequestStatus r={d} />
            <span>{tf.submittedAt(formatDateTime(d.created_at))}</span>
            <span>{tf.colUpdated}: {formatRelative(d.updated_at)}</span>
          </p>
        </header>
        <section aria-labelledby="req-msg" className="rounded-xl bg-brand-50 p-4 ring-1 ring-brand-100 dark:bg-zinc-900 dark:ring-zinc-800">
          <h2 id="req-msg" className="text-sm font-semibold">{tf.messageFromTeam}</h2>
          {d.message ? (
            <>
              <p className="mt-1 text-sm whitespace-pre-line">{d.message}</p>
              {d.message_at && <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{formatDateTime(d.message_at)}</p>}
            </>
          ) : (
            <p className="mt-1 text-sm text-zinc-500 dark:text-zinc-400">{tf.noMessage}</p>
          )}
        </section>
        <section aria-labelledby="req-answers" className="rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
          <h2 id="req-answers" className="mb-2 text-sm font-semibold">{tf.answers}</h2>
          <dl className="space-y-3 text-sm">
            {d.answers.map((a) => (
              <div key={a.key}>
                <dt className="text-xs text-zinc-500 dark:text-zinc-400">{a.label}</dt>
                <dd className="whitespace-pre-line break-words">{a.display}</dd>
              </div>
            ))}
            {d.files.length > 0 && (
              <div>
                <dt className="text-xs text-zinc-500 dark:text-zinc-400">{tf.files}</dt>
                <dd>
                  <ul>
                    {d.files.map((f, i) => (
                      <li key={`${f.filename}-${String(i)}`}>{f.filename} <span className="text-xs text-zinc-500 dark:text-zinc-400">({formatBytes(f.size)})</span></li>
                    ))}
                  </ul>
                </dd>
              </div>
            )}
          </dl>
        </section>
      </article>
    );
  }
  return (
    <Layout crumbs={[{ label: tf.myRequests, to: "/requests" }, { label: q.data?.key ?? "…" }]}>
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-2xl space-y-4 p-4 md:p-6">
          <Link to="/requests" className="text-sm text-brand-700 hover:underline dark:text-brand-300">← {tf.back}</Link>
          {body}
        </div>
      </div>
    </Layout>
  );
}
