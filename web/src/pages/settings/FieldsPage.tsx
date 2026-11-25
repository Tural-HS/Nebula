import { useState } from "react";

import { useAdminApi, type FormulaInput } from "../../api/admin";
import { parseCurrencyList } from "../../lib/money";
import { MoneySettings } from "./MoneySettings";
import { useFields } from "../../api/hooks";
import type { Choice, FieldDefinition } from "../../api/types";
import { Button, ErrorState, Input, Select, Spinner } from "../../components/ui";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";
import { cn } from "../../lib/cn";
import { slugKey } from "../../lib/slug";
import { tf } from "../../i18n-formulas";
import { tm } from "../../i18n-money";
import { FormulaEditor } from "./FormulaEditor";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

const TYPES = ["text", "long_text", "number", "date", "datetime", "boolean", "select", "multi_select", "user", "url", "email", "formula", "money"];
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
  const [money, setMoney] = useState({ currencies: "AZN, USD, EUR", default_currency: "AZN", scale: 2 });
  const [moneyError, setMoneyError] = useState("");
  const [unit, setUnit] = useState("");

  return (
    <Card>
      <h2 className="mb-3 text-sm font-semibold">{ta.newField}</h2>
      <form
        className="grid gap-3 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault();
          const list = choices.split("\n").map((s) => s.trim()).filter(Boolean).map((label) => ({ label }));
          let moneyOpts = null;
          if (type === "money") {
            const parsed = parseCurrencyList(money.currencies);
            if (!parsed.ok) { setMoneyError(parsed.error); return; }
            setMoneyError("");
            moneyOpts = { currencies: parsed.codes, default_currency: parsed.codes.includes(money.default_currency) ? money.default_currency : (parsed.codes[0] ?? "AZN"), scale: money.scale };
          }
          const options = isChoice(type) ? { options: { choices: list } } : type === "formula" ? { options: { formula } } : moneyOpts ? { options: { money: moneyOpts } } : type === "number" && unit.trim() ? { options: { unit } } : {};
          adm.createField.mutate(
            { name, key, type, ...options },
            { onSuccess: () => { setName(""); setKey(""); setKeyTouched(false); setChoices(""); setFormula({ expression: "" }); setUnit(""); fb.ok()(); }, onError: fb.fail },
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
        {type === "number" && (
          <label className="text-xs text-zinc-500 dark:text-zinc-400">
            {ta.unit} <span className="text-zinc-500 dark:text-zinc-400">({ta.unitHint})</span>
            <Input maxLength={8} value={unit} onChange={(e) => { setUnit(e.target.value); }} />
          </label>
        )}
        {type === "formula" && (
          <div className="sm:col-span-2">
            <FormulaEditor ws={ws} value={formula} onChange={setFormula} />
          </div>
        )}
        {type === "money" && (
          <div className="sm:col-span-2">
            <MoneySettings value={money} error={moneyError} onChange={(v) => { setMoney(v); setMoneyError(""); }} />
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

      {field.type === "money" && !field.archived && <MoneyFieldCard ws={ws} field={field} />}
      {field.type === "number" && !field.archived && <UnitFieldCard ws={ws} field={field} />}

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

/** Allowed and default currencies of an existing money field; the scale is fixed (ADR-0044). */
function MoneyFieldCard({ ws, field }: { ws: string; field: FieldDefinition }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const saved = field.options.money ?? { currencies: ["AZN", "USD", "EUR"], default_currency: "AZN", scale: 2 };
  const [value, setValue] = useState({ currencies: saved.currencies.join(", "), default_currency: saved.default_currency, scale: saved.scale });
  const [error, setError] = useState("");
  const dirty = value.currencies !== saved.currencies.join(", ") || value.default_currency !== saved.default_currency;
  return (
    <form
      className="mt-3 space-y-2 border-t border-zinc-100 pt-3 dark:border-zinc-800"
      onSubmit={(e) => {
        e.preventDefault();
        const parsed = parseCurrencyList(value.currencies);
        if (!parsed.ok) { setError(parsed.error); return; }
        setError("");
        const def = parsed.codes.includes(value.default_currency) ? value.default_currency : (parsed.codes[0] ?? "AZN");
        adm.updateField.mutate({ key: field.key, body: { options: { money: { currencies: parsed.codes, default_currency: def } } } }, { onSuccess: fb.ok(), onError: fb.fail });
      }}
    >
      <MoneySettings value={value} error={error} scaleFixed onChange={(v) => { setValue(v); setError(""); }} />
      {dirty && <Button type="submit" variant="primary" disabled={adm.updateField.isPending}>{tm.saveCurrencies}</Button>}
    </form>
  );
}

/** Display unit of a number field ("₼", "%"): only how the value is shown, never stored with it. */
function UnitFieldCard({ ws, field }: { ws: string; field: FieldDefinition }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const saved = field.options.unit ?? "";
  const [unit, setUnit] = useState(saved);
  return (
    <form
      className="mt-3 flex flex-wrap items-end gap-2 border-t border-zinc-100 pt-3 dark:border-zinc-800"
      onSubmit={(e) => {
        e.preventDefault();
        adm.updateField.mutate({ key: field.key, body: { options: { unit: unit.trim() } } }, { onSuccess: fb.ok(), onError: fb.fail });
      }}
    >
      <label className="text-xs text-zinc-500 dark:text-zinc-400">
        {ta.unit} <span className="text-zinc-500 dark:text-zinc-400">({ta.unitHint})</span>
        <Input maxLength={8} value={unit} className="max-w-32" onChange={(e) => { setUnit(e.target.value); }} />
      </label>
      {unit.trim() !== saved && <Button type="submit" variant="primary" disabled={adm.updateField.isPending}>{ta.saveUnit}</Button>}
    </form>
  );
}
