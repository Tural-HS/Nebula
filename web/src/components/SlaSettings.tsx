import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, path } from "../api/client";
import { keys } from "../api/hooks";
import type { SlaPolicy } from "../api/types";
import { tsl } from "../i18n-sla";
import { useToast } from "./toast-context";
import { Button, ErrorState, FieldError, Input, Label, Spinner } from "./ui";

/** Project settings: SLA targets in working days (ADR-0045). Admins only. */
export function SlaSettings({ ws, project }: { ws: string; project: string }) {
  const policy = useQuery({
    queryKey: ["ws", ws, "p", project, "sla"],
    queryFn: () => api<SlaPolicy>("GET", path`/workspaces/${ws}/projects/${project}/sla`),
  });
  let body;
  if (policy.isPending) body = <Spinner />;
  else if (policy.isError) body = <ErrorState error={policy.error} onRetry={() => void policy.refetch()} />;
  else body = <SlaForm key={`${String(policy.data.response_days)}-${String(policy.data.resolution_days)}`} ws={ws} project={project} policy={policy.data} />;
  return (
    <section aria-labelledby="ps-sla" className="space-y-3 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <h2 id="ps-sla" className="text-sm font-semibold">{tsl.settings.heading}</h2>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{tsl.settings.intro}</p>
      {body}
    </section>
  );
}

const num = (v: string): number | null => (v.trim() === "" ? null : Number(v));

function SlaForm({ ws, project, policy }: { ws: string; project: string; policy: SlaPolicy }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [response, setResponse] = useState(policy.response_days?.toString() ?? "");
  const [resolution, setResolution] = useState(policy.resolution_days?.toString() ?? "");
  const save = useMutation({
    mutationFn: (body: { response_days: number | null; resolution_days: number | null }) =>
      api<SlaPolicy>("PUT", path`/workspaces/${ws}/projects/${project}/sla`, body),
    onSuccess: (p) => {
      toast("success", p.response_days === null && p.resolution_days === null ? tsl.settings.off : tsl.settings.saved);
      qc.setQueryData(["ws", ws, "p", project, "sla"], p);
      void qc.invalidateQueries({ queryKey: keys.items(ws, project) });
    },
  });
  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate({ response_days: num(response), resolution_days: num(resolution) });
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor="sla-response">{tsl.settings.response}</Label>
          <Input id="sla-response" type="number" inputMode="numeric" min={1} max={60} placeholder={tsl.settings.empty}
            aria-describedby="sla-response-hint" value={response} onChange={(e) => { setResponse(e.target.value); }} />
          <p id="sla-response-hint" className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{tsl.settings.responseHint}</p>
        </div>
        <div>
          <Label htmlFor="sla-resolution">{tsl.settings.resolution}</Label>
          <Input id="sla-resolution" type="number" inputMode="numeric" min={1} max={120} placeholder={tsl.settings.empty}
            aria-describedby="sla-resolution-hint" value={resolution} onChange={(e) => { setResolution(e.target.value); }} />
          <p id="sla-resolution-hint" className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{tsl.settings.resolutionHint}</p>
        </div>
      </div>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{tsl.settings.riskNote}</p>
      <FieldError error={save.error} />
      <Button type="submit" variant="primary" disabled={save.isPending}>{tsl.settings.save}</Button>
    </form>
  );
}
