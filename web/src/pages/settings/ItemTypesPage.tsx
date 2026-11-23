import { useState } from "react";

import { useAdminApi, useWorkflows } from "../../api/admin";
import { useFields, useItemTypes } from "../../api/hooks";
import type { FieldDefinition, ItemType } from "../../api/types";
import { Button, ErrorState, Input, Select, Spinner } from "../../components/ui";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";
import { slugKey } from "../../lib/slug";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

export function ItemTypesPage() {
  return <SettingsLayout title={ta.nav.itemTypes}>{(ws) => <ItemTypes ws={ws} />}</SettingsLayout>;
}

function ItemTypes({ ws }: { ws: string }) {
  const types = useItemTypes(ws);
  const workflows = useWorkflows(ws);
  const fields = useFields(ws);
  if (types.isPending || workflows.isPending || fields.isPending) return <Spinner />;
  if (types.isError) return <ErrorState error={types.error} />;
  const wfName = new Map((workflows.data ?? []).map((w) => [w.id, w.name]));
  return (
    <div className="space-y-4">
      <NewType ws={ws} />
      {types.data.map((x) => (
        <TypeCard key={x.id} ws={ws} type={x} workflowName={wfName.get(x.workflow_id) ?? "?"} fields={fields.data ?? []} />
      ))}
    </div>
  );
}

function NewType({ ws }: { ws: string }) {
  const adm = useAdminApi(ws);
  const workflows = useWorkflows(ws);
  const fb = useFeedback();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [wf, setWf] = useState("");
  const workflowId = wf || workflows.data?.[0]?.id || "";
  return (
    <Card>
      <h2 className="mb-3 text-sm font-semibold">{ta.newType}</h2>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          adm.createType.mutate({ name, key, workflow_id: workflowId }, { onSuccess: () => { setName(""); setKey(""); fb.ok()(); }, onError: fb.fail });
        }}
      >
        <label className="text-xs text-zinc-500">
          {t.name}
          <Input required maxLength={100} value={name} onChange={(e) => { setName(e.target.value); setKey(slugKey(e.target.value)); }} />
        </label>
        <label className="text-xs text-zinc-500">
          {ta.key}
          <Input required pattern="[a-z][a-z0-9_]{0,62}" value={key} onChange={(e) => { setKey(e.target.value); }} className="w-40" />
        </label>
        <label className="text-xs text-zinc-500">
          {ta.workflow}
          <Select value={workflowId} onChange={(e) => { setWf(e.target.value); }} className="w-48">
            {workflows.data?.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
          </Select>
        </label>
        <Button type="submit" variant="primary" disabled={adm.createType.isPending || !workflowId}>{t.create}</Button>
      </form>
    </Card>
  );
}

function TypeCard({ ws, type, workflowName, fields }: { ws: string; type: ItemType; workflowName: string; fields: FieldDefinition[] }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const [name, setName] = useState(type.name);
  const [pick, setPick] = useState("");
  const [required, setRequired] = useState(false);
  const bound = [...type.fields].sort((a, b) => a.position - b.position);
  const byKey = new Map(fields.map((f) => [f.key, f]));
  const available = fields.filter((f) => !f.archived && !bound.some((b) => b.key === f.key));

  return (
    <Card>
      <div className="flex flex-wrap items-center gap-3">
        <Input
          aria-label={t.name}
          value={name}
          maxLength={100}
          className="max-w-xs font-medium"
          onChange={(e) => { setName(e.target.value); }}
          onBlur={() => { if (name.trim() && name !== type.name) adm.updateType.mutate({ key: type.key, body: { name } }, { onSuccess: fb.ok(), onError: fb.fail }); }}
        />
        <code className="text-xs text-zinc-500">{type.key}</code>
        <span className="text-xs text-zinc-500">{ta.workflow}: {workflowName}</span>
        <Button
          variant="ghost"
          className="ml-auto text-red-600"
          onClick={() => { if (window.confirm(ta.confirm)) adm.archiveType.mutate(type.key, { onSuccess: fb.ok(), onError: fb.fail }); }}
        >
          {ta.archive}
        </Button>
      </div>

      <div className="mt-3 border-t border-zinc-100 pt-3 dark:border-zinc-800">
        <p className="mb-2 text-xs font-medium text-zinc-500">{ta.boundFields}</p>
        {bound.length === 0 && <p className="text-xs text-zinc-400">{ta.noBoundFields}</p>}
        <ul className="space-y-1">
          {bound.map((b, i) => (
            <li key={b.key} className="flex items-center gap-3 text-sm">
              <span className="min-w-40">{byKey.get(b.key)?.name ?? b.key}</span>
              <label className="flex items-center gap-1.5 text-xs text-zinc-600 dark:text-zinc-400">
                <input
                  type="checkbox"
                  className="accent-brand-600"
                  checked={b.required}
                  onChange={(e) => { adm.bindField.mutate({ type: type.key, field: b.key, required: e.target.checked, position: i }, { onSuccess: fb.ok(), onError: fb.fail }); }}
                />
                {ta.required}
              </label>
              <Button variant="ghost" onClick={() => { adm.unbindField.mutate({ type: type.key, field: b.key }, { onSuccess: fb.ok(), onError: fb.fail }); }}>
                {ta.unbind}
              </Button>
            </li>
          ))}
        </ul>
        {available.length > 0 && (
          <form
            className="mt-2 flex flex-wrap items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              const field = pick || available[0]?.key;
              if (!field) return;
              adm.bindField.mutate({ type: type.key, field, required, position: bound.length }, { onSuccess: () => { setPick(""); setRequired(false); fb.ok()(); }, onError: fb.fail });
            }}
          >
            <Select aria-label={ta.bind} value={pick || available[0]?.key} onChange={(e) => { setPick(e.target.value); }} className="w-56">
              {available.map((f) => <option key={f.key} value={f.key}>{f.name} ({ta.fieldTypes[f.type]})</option>)}
            </Select>
            <label className="flex items-center gap-1.5 text-xs text-zinc-600 dark:text-zinc-400">
              <input type="checkbox" className="accent-brand-600" checked={required} onChange={(e) => { setRequired(e.target.checked); }} />
              {ta.required}
            </label>
            <Button type="submit">{ta.bind}</Button>
          </form>
        )}
      </div>
    </Card>
  );
}
