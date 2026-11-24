import type { ApprovalRequest } from "../api/approvals";
import { tap } from "../i18n-approvals";
import { tlg } from "../i18n-ledger";
import { cn } from "../lib/cn";
import { Avatar } from "./ui";

/**
 * Who has approved an open request and who is still awaited (ADR-0035).
 * "Awaited" are the people who may still decide (the server's approvers
 * list, never the requester); any one of them is enough per missing approval.
 */
export function ApproverRoster({ req }: { req: ApprovalRequest }) {
  const approved = req.decisions.filter((d) => d.decision === "approve").map((d) => d.user);
  const waiting = req.approvers.filter((a) => !req.decisions.some((d) => d.user.id === a.id));
  if (approved.length === 0 && waiting.length === 0) return <p className="text-xs text-zinc-500">{tap.noApprovers}</p>;
  return (
    <div className="text-xs">
      <p className="mb-1.5 text-zinc-500">{tap.approvers}</p>
      <ul className="flex flex-wrap gap-1.5">
        {approved.map((u) => (
          <Person key={u.id} name={u.display_name} done />
        ))}
        {waiting.map((u) => (
          <Person key={u.id} name={u.display_name} done={false} />
        ))}
      </ul>
    </div>
  );
}

function Person({ name, done }: { name: string; done: boolean }) {
  return (
    <li
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full py-0.5 pr-2.5 pl-0.5 ring-1",
        done
          ? "bg-emerald-50 text-emerald-800 ring-emerald-200 dark:bg-emerald-950 dark:text-emerald-200 dark:ring-emerald-900"
          : "bg-white text-zinc-600 ring-zinc-200 dark:bg-zinc-900 dark:text-zinc-400 dark:ring-zinc-700",
      )}
    >
      <Avatar name={name} />
      <span className="font-medium">{name}</span>
      <span className={done ? "" : "text-zinc-400 dark:text-zinc-500"}>
        <span aria-hidden>{done ? "✓ " : ""}</span>
        {done ? tlg.approvedBy : tlg.waitingFor}
      </span>
    </li>
  );
}
