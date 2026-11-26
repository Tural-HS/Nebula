import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, path } from "../api/client";
import { useFields } from "../api/hooks";
import { tp } from "../i18n-project";
import { formatDate } from "../lib/format";
import { useToast } from "./toast-context";
import { Button, ErrorState, FieldError, Label, Select, Spinner } from "./ui";

/** GET/PUT /projects/{p}/date-reminder (ADR-0054). */
interface DateReminder {
  field: string | null;
  last_sent: string | null;
  hour: number;
}

/** Project settings: yearly reminder on a date field, e.g. customers'
 * birthdays (ADR-0054). Admins only. */
export function DateReminderSettings({ ws, project }: { ws: string; project: string }) {
  const key = ["ws", ws, "p", project, "date-reminder"];
  const reminder = useQuery({ queryKey: key, queryFn: () => api<DateReminder>("GET", path`/workspaces/${ws}/projects/${project}/date-reminder`) });
  const fields = useFields(ws);
  let body;
  if (reminder.isPending || fields.isPending) body = <Spinner />;
  else if (reminder.isError) body = <ErrorState error={reminder.error} onRetry={() => void reminder.refetch()} />;
  else if (fields.isError) body = <ErrorState error={fields.error} onRetry={() => void fields.refetch()} />;
  else {
    const dates = fields.data.filter((f) => f.type === "date" && !f.archived);
    body = dates.length === 0 && !reminder.data.field
      ? <p className="text-sm text-zinc-600 dark:text-zinc-300">{tp.reminder.noDate}</p>
      : <ReminderForm key={reminder.data.field ?? ""} ws={ws} project={project} queryKey={key} current={reminder.data} dates={dates} />;
  }
  return (
    <section aria-labelledby="ps-reminder" className="space-y-3 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <h2 id="ps-reminder" className="text-sm font-semibold">🎂 {tp.reminder.title}</h2>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{tp.reminder.hint}</p>
      {body}
    </section>
  );
}

function ReminderForm({ ws, project, queryKey, current, dates }: {
  ws: string;
  project: string;
  queryKey: string[];
  current: DateReminder;
  dates: { key: string; name: string }[];
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const [field, setField] = useState(current.field ?? "");
  const save = useMutation({
    mutationFn: (f: string | null) => api<DateReminder>("PUT", path`/workspaces/${ws}/projects/${project}/date-reminder`, { field: f }),
    onSuccess: (r) => {
      toast("success", tp.reminder.saved);
      qc.setQueryData(queryKey, r);
    },
  });
  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate(field === "" ? null : field);
      }}
    >
      <div className="max-w-xs">
        <Label htmlFor="ps-reminder-field">{tp.reminder.field}</Label>
        <Select id="ps-reminder-field" value={field} onChange={(e) => { setField(e.target.value); }}>
          <option value="">{tp.reminder.off}</option>
          {dates.map((d) => <option key={d.key} value={d.key}>{d.name}</option>)}
        </Select>
      </div>
      {current.field && current.last_sent && (
        <p className="text-xs text-zinc-500 dark:text-zinc-400">{tp.reminder.last}: {formatDate(current.last_sent)} · {String(current.hour).padStart(2, "0")}:00</p>
      )}
      <FieldError error={save.error} />
      <Button type="submit" variant="primary" disabled={save.isPending || field === (current.field ?? "")}>{tp.reminder.saveLabel}</Button>
    </form>
  );
}
