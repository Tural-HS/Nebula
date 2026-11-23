import { useState, type SyntheticEvent } from "react";

import { useCreateItem, useFields, useItemTypes, useMembers } from "../api/hooks";
import type { FieldValue, Item } from "../api/types";
import { FieldEditor } from "../components/FieldEditor";
import { useToast } from "../components/toast-context";
import { Button, Dialog, FieldError, Input, Label, Select } from "../components/ui";
import { t } from "../i18n";

export function CreateItemDialog({ ws, project, open, onClose, onCreated }: {
  ws: string; project: string; open: boolean; onClose: () => void; onCreated: (item: Item) => void;
}) {
  return (
    <Dialog open={open} onClose={onClose} title={t.newItem}>
      <CreateItemForm ws={ws} project={project} onClose={onClose} onCreated={onCreated} />
    </Dialog>
  );
}

function CreateItemForm({ ws, project, onClose, onCreated }: { ws: string; project: string; onClose: () => void; onCreated: (item: Item) => void }) {
  const types = useItemTypes(ws);
  const members = useMembers(ws);
  const fields = useFields(ws);
  const create = useCreateItem(ws, project);
  const toast = useToast();
  const [title, setTitle] = useState("");
  const [typeKey, setTypeKey] = useState("task");
  const [assignee, setAssignee] = useState("");
  const [due, setDue] = useState("");
  const [values, setValues] = useState<Record<string, FieldValue>>({});

  const type = types.data?.find((x) => x.key === typeKey) ?? types.data?.[0];
  const required = (type?.fields ?? []).filter((f) => f.required);
  const defs = new Map((fields.data ?? []).map((f) => [f.key, f]));

  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    const fieldValues = Object.fromEntries(Object.entries(values).filter(([k, v]) => v !== null && required.some((r) => r.key === k)));
    create.mutate(
      { title, type: type?.key, assignee_id: assignee || null, due_date: due || null, fields: fieldValues },
      {
        onSuccess: (item) => {
          toast("success", t.created(item.key));
          onCreated(item);
          onClose();
        },
      },
    );
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      <div>
        <Label htmlFor="new-title">{t.title}</Label>
        <Input id="new-title" value={title} onChange={(e) => { setTitle(e.target.value); }} required maxLength={500} autoFocus />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label htmlFor="new-type">{t.type}</Label>
          <Select id="new-type" value={type?.key ?? ""} onChange={(e) => { setTypeKey(e.target.value); setValues({}); }}>
            {types.data?.map((x) => (
              <option key={x.id} value={x.key}>
                {x.name}
              </option>
            ))}
          </Select>
        </div>
        <div>
          <Label htmlFor="new-assignee">{t.assignee}</Label>
          <Select id="new-assignee" value={assignee} onChange={(e) => { setAssignee(e.target.value); }}>
            <option value="">{t.unassigned}</option>
            {members.data?.map((m) => (
              <option key={m.user_id} value={m.user_id}>
                {m.display_name}
              </option>
            ))}
          </Select>
        </div>
        <div>
          <Label htmlFor="new-due">{t.dueDate}</Label>
          <Input id="new-due" type="date" min="1900-01-01" max="2200-12-31" value={due} onChange={(e) => { setDue(e.target.value); }} />
        </div>
      </div>
      {required.map((r) => {
        const def = defs.get(r.key);
        if (!def) return null;
        const id = `new-field-${r.key}`;
        return (
          <div key={r.key}>
            <Label htmlFor={id}>{def.name} *</Label>
            <FieldEditor
              id={id}
              def={def}
              value={values[r.key]}
              members={members.data ?? []}
              onCommit={(v) => { setValues((s) => ({ ...s, [r.key]: v })); }}
            />
          </div>
        );
      })}
      <FieldError error={create.error} />
      <div className="flex justify-end gap-2">
        <Button onClick={onClose}>{t.cancel}</Button>
        <Button type="submit" variant="primary" disabled={create.isPending}>
          {t.create}
        </Button>
      </div>
    </form>
  );
}
