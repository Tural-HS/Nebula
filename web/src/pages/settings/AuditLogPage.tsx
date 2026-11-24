import { useState } from "react";

import { useAuditLog } from "../../api/admin";
import { EvidencePanel } from "../../components/Evidence";
import { Button, ErrorState, Input, Spinner } from "../../components/ui";
import { ta } from "../../i18n-admin";
import { actorName } from "../../lib/activity";
import { formatDateTime } from "../../lib/format";
import { Card, SettingsLayout } from "./SettingsLayout";

export function AuditLogPage() {
  return <SettingsLayout title={ta.nav.audit}>{(ws) => <Audit ws={ws} />}</SettingsLayout>;
}

function summary(data: Record<string, unknown>): string {
  const s = JSON.stringify(data);
  return s === "{}" ? "" : s.length > 160 ? s.slice(0, 160) + "…" : s;
}

function Audit({ ws }: { ws: string }) {
  const [filter, setFilter] = useState("");
  const [action, setAction] = useState("");
  const [cursors, setCursors] = useState<number[]>([0]); // stack of "before" values for back/forward
  const before = cursors.at(-1) ?? 0;
  const q = useAuditLog(ws, action, before);

  return (
    <div className="space-y-3">
      <EvidencePanel ws={ws} />
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setAction(filter.trim());
          setCursors([0]);
        }}
      >
        <Input value={filter} maxLength={100} placeholder={ta.auditFilter} onChange={(e) => { setFilter(e.target.value); }} className="max-w-sm" />
        <Button type="submit">{ta.auditApply}</Button>
      </form>
      {q.isPending ? (
        <Spinner />
      ) : q.isError ? (
        <ErrorState error={q.error} />
      ) : (
        <Card className="overflow-x-auto p-0">
          <table className="w-full min-w-[40rem] text-left text-sm">
            <thead className="text-xs text-zinc-500">
              <tr className="border-b border-zinc-200 dark:border-zinc-800">
                <th className="px-4 py-2">#</th><th>{ta.auditCols.time}</th><th>{ta.auditCols.who}</th><th>{ta.auditCols.action}</th><th>{ta.auditCols.data}</th>
              </tr>
            </thead>
            <tbody>
              {q.data.activity.map((a) => (
                <tr key={a.id} className="border-b border-zinc-100 align-top dark:border-zinc-800/60">
                  <td className="px-4 py-2 font-mono text-xs text-zinc-500">{a.id}</td>
                  <td className="py-2 text-xs whitespace-nowrap">{formatDateTime(a.occurred_at)}</td>
                  <td className="py-2 text-xs">{actorName(a)}</td>
                  <td className="py-2"><code className="text-xs">{a.action}</code></td>
                  <td className="py-2 pr-4 font-mono text-[11px] break-all text-zinc-500">{summary(a.data)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
      <div className="flex gap-2">
        <Button disabled={cursors.length <= 1} onClick={() => { setCursors([0]); }}>{ta.newer}</Button>
        <Button
          disabled={!q.data?.next_before}
          onClick={() => {
            const next = q.data?.next_before;
            if (next) setCursors((c) => [...c, next]);
          }}
        >
          {ta.older}
        </Button>
      </div>
    </div>
  );
}
