import { useState } from "react";

import { useAdminApi, type FormulaInput } from "../../api/admin";
import { useFields } from "../../api/hooks";
import type { Choice, FieldDefinition } from "../../api/types";
import { Button, ErrorState, Input, Select, Spinner } from "../../components/ui";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";
import { cn } from "../../lib/cn";
import { slugKey } from "../../lib/slug";
import { tf } from "../../i18n-formulas";
import { FormulaEditor } from "./FormulaEditor";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

const TYPES = ["text", "long_text", "number", "date", "datetime", "boolean", "select", "multi_select", "user", "url", "email", "formula"];
const isChoice = (type: string) => type === "select" || type === "multi_select";

export function FieldsPage() {
  return <SettingsLayout title={ta.nav.fields}>{(ws) => <Fields ws={ws} />}</SettingsLayout>;
}

function Fields({ ws }: { ws: string }) {
  const fields = useFields(ws);
  if (fields.isPending) return <Spinner />;
  if (fields.isError) return <ErrorState error={fields.error} />;
  return (
    <div className="space-y-4">
      <NewField ws={ws} />
      {fields.data.map((f) => <FieldCard key={f.id} ws={ws} field={f} />)}
    </div>
  );
}

function NewField({ ws }: { ws: string }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [keyTouched, setKeyTouched] = useState(false);
  const [type, setType] = useState("text");
  const [choices, setChoices] = useState("");
  const [formula, setFormula] = useState<FormulaInput>({ expression: "" });

  return (
    <Card>
      <h2 className="mb-3 text-sm font-semibold">{ta.newField}</h2>
      <form
        className="grid gap-3 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault();
          const list = choices.split("\n").map((s) => s.trim()).filter(Boolean).map((label) => ({ label }));
          const options = isChoice(type) ? { options: { choices: list } } : type === "formula" ? { options: { formula } } : {};
          adm.createField.mutate(
            { name, key, type, ...options },
            { onSuccess: () => { setName(""); setKey(""); setKeyTouched(false); setChoices(""); setFormula({ expression: "" }); fb.ok()(); }, onError: fb.fail },
          );
        }}
      >
        <label className="text-xs text-zinc-500 dark:text-zinc-400">
          {t.name}
          <Input required maxLength={100} value={name} onChange={(e) => { setName(e.target.value); if (!keyTouched) setKey(slugKey(e.target.value)); }} />
        </label>
        <label className="text-xs text-zinc-500 dark:text-zinc-400">
          {ta.key} <span className="text-zinc-500 dark:text-zinc-400">({ta.fieldKeyHint})</span>
          <Input required pattern="[a-z][a-z0-9_]{0,62}" value={key} onChange={(e) => { setKey(e.target.value); setKeyTouched(true); }} />
        </label>
        <label className="text-xs text-zinc-500 dark:text-zinc-400">
          {ta.fieldType}
          <Select value={type} onChange={(e) => { setType(e.target.value); }}>
            {TYPES.map((x) => <option key={x} value={x}>{ta.fieldTypes[x]}</option>)}
          </Select>
        </label>
        {isChoice(type) && (
          <label className="text-xs text-zinc-500 dark:text-zinc-400 sm:row-span-2">
            {ta.choices} <span className="text-zinc-500 dark:text-zinc-400">({ta.choicesHint})</span>
            <textarea
              required
              rows={3}
              value={choices}
              onChange={(e) => { setChoices(e.target.value); }}
              className="w-full rounded-md bg-white px-2.5 py-1.5 text-sm ring-1 ring-zinc-300 dark:bg-zinc-900 dark:ring-zinc-700"
            />
          </label>
        )}
        {type === "formula" && (
          <div className="sm:col-span-2">
            <FormulaEditor ws={ws} value={formula} onChange={setFormula} />
          </div>
        )}
        <div className="flex items-end sm:col-span-2">
          <Button type="submit" variant="primary" disabled={adm.createField.isPending}>{t.create}</Button>
        </div>
      </form>
    </Card>
  );
}

function FieldCard({ ws, field }: { ws: string; field: FieldDefinition }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const [name, setName] = useState(field.name);
  const [newChoice, setNewChoice] = useState("");
  const choices = field.options.choices ?? [];
  const saved = field.options.formula;
  const [formula, setFormula] = useState<FormulaInput>({ expression: saved?.expression ?? "", ...(saved?.scale !== undefined ? { scale: saved.scale } : {}) });
  const formulaDirty = !!saved && (formula.expression.trim() !== saved.expression || (formula.scale ?? saved.scale) !== saved.scale);

  const saveChoices = (next: (Choice | { label: string })[]) => {
    adm.updateField.mutate(
      { key: field.key, body: { options: { choices: next.map((c) => ("id" in c ? { id: c.id, label: c.label, archived: c.archived ?? false } : { label: c.label })) } } },
      { onSuccess: fb.ok(), onError: fb.fail },
    );
  };

  return (
    <Card className={cn(field.archived && "opacity-60")}>
      <div className="flex flex-wrap items-center gap-3">
        <Input
          aria-label={t.name}
          value={name}
          maxLength={100}
          disabled={field.archived}
          className="max-w-xs font-medium"
          onChange={(e) => { setName(e.target.value); }}
          onBlur={() => { if (name.trim() && name !== field.name) adm.updateField.mutate({ key: field.key, body: { name } }, { onSuccess: fb.ok(), onError: fb.fail }); }}
        />
        <code className="text-xs text-zinc-500 dark:text-zinc-400">{field.key}</code>
        <span className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs dark:bg-zinc-800">
          {ta.fieldTypes[field.type]}
          {saved && ` → ${tf.resultTypes[saved.result_type] ?? saved.result_type}`}
        </span>
        {field.archived && <span className="text-xs text-zinc-500 dark:text-zinc-400">{ta.archived}</span>}
        <Button
          variant="ghost"
          className="ml-auto"
          onClick={() => { adm.updateField.mutate({ key: field.key, body: { archived: !field.archived } }, { onSuccess: fb.ok(), onError: fb.fail }); }}
        >
          {field.archived ? ta.restore : ta.archive}
        </Button>
      </div>

      {field.type === "formula" && !field.archived && (
        <form
          className="mt-3 space-y-2 border-t border-zinc-100 pt-3 dark:border-zinc-800"
          onSubmit={(e) => {
            e.preventDefault();
            adm.updateField.mutate({ key: field.key, body: { options: { formula } } }, { onSuccess: fb.ok(), onError: fb.fail });
          }}
        >
          <FormulaEditor ws={ws} fieldKey={field.key} value={formula} onChange={setFormula} />
          {formulaDirty && <Button type="submit" variant="primary" disabled={adm.updateField.isPending}>{tf.save}</Button>}
        </form>
      )}
      {field.type === "formula" && field.archived && saved && (
        <p className="mt-2 font-mono text-xs text-zinc-500 dark:text-zinc-400">{tf.title(saved.expression)}</p>
      )}

      {isChoice(field.type) && !field.archived && (
        <div className="mt-3 space-y-2 border-t border-zinc-100 pt-3 dark:border-zinc-800">
          <p className="text-xs font-medium text-zinc-500 dark:text-zinc-400">{ta.choices}</p>
          <ul className="flex flex-wrap gap-2">
            {choices.map((c) => (
              <li key={c.id} className={cn("flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs ring-1 ring-zinc-300 dark:ring-zinc-700", c.archived && "line-through opacity-60")}>
                {c.label}
                <button
                  type="button"
                  title={c.archived ? ta.restore : ta.archive}
                  className="text-zinc-400 hover:text-zinc-800 dark:hover:text-zinc-100"
                  onClick={() => { saveChoices(choices.map((x) => (x.id === c.id ? { ...x, archived: !x.archived } : x))); }}
                >
                  {c.archived ? "↺" : "×"}
                </button>
              </li>
            ))}
          </ul>
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (newChoice.trim()) saveChoices([...choices, { label: newChoice.trim() }]);
              setNewChoice("");
            }}
          >
            <Input value={newChoice} maxLength={100} placeholder={ta.addChoice} onChange={(e) => { setNewChoice(e.target.value); }} className="max-w-xs" />
            <Button type="submit">{ta.add}</Button>
          </form>
        </div>
      )}
    </Card>
  );
}
