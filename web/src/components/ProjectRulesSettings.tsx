import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, path } from "../api/client";
import { useFields, useProjectRules, type ProjectRules } from "../api/hooks";
import type { FieldDefinition } from "../api/types";
import { tp } from "../i18n-project";
import { useToast } from "./toast-context";
import { Button, ErrorState, FieldError, Spinner } from "./ui";

const MAX_COMPLETE = 5;
const MAX_LATE = 10;

/** Project settings: completion fields and overdue-coloured fields (ADR-0061). Admins only. */
export function ProjectRulesSettings({ ws, project }: { ws: string; project: string }) {
  const rules = useProjectRules(ws, project);
  const fields = useFields(ws);
  let body;
  if (rules.isPending || fields.isPending) body = <Spinner />;
  else if (rules.isError) body = <ErrorState error={rules.error} onRetry={() => void rules.refetch()} />;
  else if (fields.isError) body = <ErrorState error={fields.error} onRetry={() => void fields.refetch()} />;
  else {
    const live = fields.data.filter((f) => !f.archived);
    body = <RulesForm key={JSON.stringify(rules.data)} ws={ws} project={project} current={rules.data} fields={live} />;
  }
  return (
    <section aria-labelledby="ps-rules" className="space-y-3 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <h2 id="ps-rules" className="text-sm font-semibold">🔒 {tp.rules.title}</h2>
      {body}
    </section>
  );
}

function Picker({ legend, hint, options, picked, max, onChange }: {
  legend: string; hint: string; options: FieldDefinition[]; picked: string[]; max: number; onChange: (next: string[]) => void;
}) {
  return (
    <fieldset>
      <legend className="text-sm font-medium">{legend} ({picked.length}/{max})</legend>
      <p className="mb-1 text-xs text-zinc-500 dark:text-zinc-400">{hint}</p>
      {options.length === 0 ? (
        <p className="text-sm text-zinc-600 dark:text-zinc-300">{tp.rules.noFields}</p>
      ) : (
        <div className="flex flex-wrap gap-x-4 gap-y-1">
          {options.map((f) => {
            const checked = picked.includes(f.key);
            return (
              <label key={f.key} className="flex items-center gap-1.5 text-sm">
                <input
                  type="checkbox"
                  className="accent-brand-600"
                  checked={checked}
                  disabled={!checked && picked.length >= max}
                  onChange={() => { onChange(checked ? picked.filter((k) => k !== f.key) : [...picked, f.key]); }}
                />
                {f.name}
              </label>
            );
          })}
        </div>
      )}
    </fieldset>
  );
}

function RulesForm({ ws, project, current, fields }: { ws: string; project: string; current: ProjectRules; fields: FieldDefinition[] }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [complete, setComplete] = useState<string[]>(current.complete_fields);
  const [late, setLate] = useState<string[]>(current.late_fields);
  const save = useMutation({
    mutationFn: (r: ProjectRules) => api<ProjectRules>("PUT", path`/workspaces/${ws}/projects/${project}/rules`, r),
    onSuccess: (r) => {
      toast("success", tp.rules.saved);
      qc.setQueryData(["ws", ws, "p", project, "rules"], r);
    },
  });
  const same = (a: string[], b: string[]) => a.length === b.length && a.every((k) => b.includes(k));
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate({ complete_fields: complete, late_fields: late });
      }}
    >
      <Picker legend={tp.rules.complete} hint={tp.rules.completeHint} options={fields.filter((f) => f.type === "boolean")}
        picked={complete} max={MAX_COMPLETE} onChange={setComplete} />
      <Picker legend={tp.rules.late} hint={tp.rules.lateHint} options={fields} picked={late} max={MAX_LATE} onChange={setLate} />
      <FieldError error={save.error} />
      <Button type="submit" variant="primary" disabled={save.isPending || (same(complete, current.complete_fields) && same(late, current.late_fields))}>
        {tp.rules.saveLabel}
      </Button>
    </form>
  );
}
