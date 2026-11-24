import { useActivity, useFields, useItemTypes, useMembers } from "../api/hooks";
import { t } from "../i18n";
import { actorName, describe, type Lookup } from "../lib/activity";
import { flattenBy } from "../lib/collections";
import { formatDateTime, formatRelative } from "../lib/format";
import { Avatar, Button, ErrorState, Spinner } from "./ui";

export function ActivityFeed({ ws, project, n }: { ws: string; project: string; n: number }) {
  const q = useActivity(ws, project, n);
  const types = useItemTypes(ws);
  const members = useMembers(ws);
  const fields = useFields(ws);

  const statuses = new Map((types.data ?? []).flatMap((x) => x.statuses.map((s) => [s.id, s.name] as const)));
  const people = new Map((members.data ?? []).map((m) => [m.user_id, m.display_name] as const));
  const fieldNames = new Map((fields.data ?? []).map((f) => [f.key, f.name] as const));
  const lookup: Lookup = { status: (id) => statuses.get(id), person: (id) => people.get(id), field: (k) => fieldNames.get(k) };

  if (q.isPending) return <Spinner />;
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  const list = flattenBy(q.data.pages, (p) => p.activity, (a) => a.id);
  if (list.length === 0) return <p className="text-sm text-zinc-500 dark:text-zinc-400">{t.noActivity}</p>;

  return (
    <div>
      <ol className="space-y-3">
        {list.map((a) => {
          const who = actorName(a);
          return (
            <li key={a.id} className="flex gap-3 text-sm">
              <Avatar name={who} />
              <div className="min-w-0 flex-1">
                {describe(a, lookup).map((line, i) => (
                  <p key={i} className="break-words">
                    {i === 0 && <span className="font-medium">{who} </span>}
                    <span className="text-zinc-600 dark:text-zinc-400">{line.text}</span>
                    {line.from !== undefined && (
                      <span>
                        {": "}
                        <span className="text-zinc-500 dark:text-zinc-400 line-through decoration-zinc-400">{line.from}</span>
                        {" → "}
                        <span className="font-medium">{line.to}</span>
                      </span>
                    )}
                  </p>
                ))}
                <p className="text-xs text-zinc-500 dark:text-zinc-400" title={formatDateTime(a.occurred_at)}>
                  {formatRelative(a.occurred_at)}
                </p>
              </div>
            </li>
          );
        })}
      </ol>
      {q.hasNextPage && (
        <Button variant="ghost" className="mt-3" onClick={() => void q.fetchNextPage()} disabled={q.isFetchingNextPage}>
          {t.loadOlder}
        </Button>
      )}
    </div>
  );
}
