import { useFields, useItemSums } from "../api/hooks";
import type { FieldDefinition, FieldSums, StatusCategory } from "../api/types";
import { trp } from "../i18n-reports";
import { isAmount } from "../lib/formula";
import { formatMoneyTotals } from "../lib/money";
import { amountText, withUnit } from "../lib/views";
import { Spinner } from "./ui";

const OPEN: readonly StatusCategory[] = ["todo", "in_progress"];
const CLOSED: readonly StatusCategory[] = ["done", "canceled"];

function total(def: FieldDefinition, s: FieldSums[string] | undefined): string {
  if (!s || s.count === 0) return "—";
  if (def.type === "money") return formatMoneyTotals(s.currencies ?? []) || "—";
  return withUnit(amountText(s.sum ?? "0", def), def);
}

/**
 * Money and number totals of one project (ADR-0050): what is still open
 * (e.g. Qalıq to collect) and what is done, now — not for a period. Uses the
 * table's sums endpoint, so the numbers match the table's total row.
 */
export function ReportAmounts({ ws, project, statuses }: {
  ws: string;
  project: string;
  statuses: readonly { id: string; category: StatusCategory }[];
}) {
  const fields = useFields(ws);
  const defs = (fields.data ?? []).filter((d) => !d.archived && isAmount(d));
  const keys = defs.map((d) => d.key);
  const ids = (cats: readonly StatusCategory[]) => statuses.filter((s) => cats.includes(s.category)).map((s) => s.id);
  const openIds = ids(OPEN);
  const closedIds = ids(CLOSED);
  // No status of a group in the project means no items in it: an empty status
  // list would mean "no filter" (every item), so the query is skipped instead.
  const open = useItemSums(ws, project, { q: "", assignee: "", status: openIds }, openIds.length > 0 ? keys : []);
  const closed = useItemSums(ws, project, { q: "", assignee: "", status: closedIds }, closedIds.length > 0 ? keys : []);
  if (keys.length === 0) return null;
  if ((openIds.length > 0 && open.isPending) || (closedIds.length > 0 && closed.isPending)) return <Spinner />;
  return (
    <table className="w-full text-sm tabular-nums">
      <caption className="sr-only">{trp.amounts}</caption>
      <thead className="text-left text-xs text-zinc-500 dark:text-zinc-400">
        <tr>
          <th scope="col" className="py-1 font-medium">{trp.amountField}</th>
          <th scope="col" className="py-1 text-right font-medium">{trp.amountOpen}</th>
          <th scope="col" className="py-1 text-right font-medium">{trp.amountClosed}</th>
        </tr>
      </thead>
      <tbody>
        {defs.map((d) => (
          <tr key={d.key} className="border-t border-zinc-200 dark:border-zinc-800">
            <th scope="row" className="py-1.5 text-left font-medium">{d.name}</th>
            <td className="py-1.5 text-right font-semibold">{total(d, open.data?.[d.key])}</td>
            <td className="py-1.5 text-right">{total(d, closed.data?.[d.key])}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
