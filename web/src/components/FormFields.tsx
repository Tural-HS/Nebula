import type { PublicFormField } from "../api/forms";
import { tf } from "../i18n-forms";
import { cn } from "../lib/cn";
import { inputId, type Draft } from "../lib/forms";
import { Input, Select, touchField } from "./ui";

/**
 * The inputs of an intake form (ADR-0032), one per row, full width: built for
 * phones first (ADR-0019). Labels and help texts are the admin's own words
 * and are shown as they are. Errors are tied to their input with
 * aria-describedby / aria-invalid; the page shows the summary.
 */
export function FormFields({ fields, draft, onChange, errors, prefix, disabled }: {
  fields: readonly PublicFormField[];
  draft: Draft;
  onChange: (key: string, value: Draft[string]) => void;
  errors: Readonly<Record<string, string>>;
  prefix: string;
  disabled?: boolean;
}) {
  return (
    <div className="space-y-5">
      {fields.map((f) => {
        const id = inputId(prefix, f.key);
        const helpId = f.help ? `${id}-help` : undefined;
        const errId = errors[f.key] ? `${id}-error` : undefined;
        const describedBy = [helpId, errId].filter(Boolean).join(" ") || undefined;
        const common = { id, disabled, "aria-invalid": errId ? true : undefined, "aria-describedby": describedBy, "aria-required": f.required || undefined };
        const label = (
          <span className="text-sm font-medium text-zinc-800 dark:text-zinc-200">
            {f.label}
            {f.required && (
              <>
                <span aria-hidden className="ml-0.5 text-red-600">*</span>
                <span className="sr-only"> ({tf.requiredMark})</span>
              </>
            )}
          </span>
        );
        const str = typeof draft[f.key] === "string" ? (draft[f.key] as string) : "";
        let control;
        switch (f.type) {
          case "boolean":
            return (
              <div key={f.key}>
                <label htmlFor={id} className="flex items-start gap-3">
                  <input
                    {...common}
                    type="checkbox"
                    className="mt-0.5 size-5 shrink-0 accent-brand-600"
                    checked={draft[f.key] === true}
                    onChange={(e) => { onChange(f.key, e.target.checked); }}
                  />
                  {label}
                </label>
                <Hint helpId={helpId} help={f.help} errId={errId} error={errors[f.key]} />
              </div>
            );
          case "multi_select": {
            const selected = Array.isArray(draft[f.key]) ? (draft[f.key] as string[]) : [];
            return (
              <fieldset key={f.key} aria-describedby={describedBy} aria-invalid={errId ? true : undefined}>
                <legend id={id} tabIndex={-1} className="mb-1.5">{label}</legend>
                <div className="space-y-2">
                  {(f.choices ?? []).map((c) => (
                    <label key={c.id} className="flex items-center gap-3 text-sm">
                      <input
                        type="checkbox"
                        className="size-5 accent-brand-600"
                        disabled={disabled}
                        checked={selected.includes(c.id)}
                        onChange={(e) => { onChange(f.key, e.target.checked ? [...selected, c.id] : selected.filter((x) => x !== c.id)); }}
                      />
                      {c.label}
                    </label>
                  ))}
                </div>
                <Hint helpId={helpId} help={f.help} errId={errId} error={errors[f.key]} />
              </fieldset>
            );
          }
          case "select":
            control = (
              <Select {...common} value={str} onChange={(e) => { onChange(f.key, e.target.value); }}>
                <option value="">{tf.none}</option>
                {(f.choices ?? []).map((c) => (
                  <option key={c.id} value={c.id}>{c.label}</option>
                ))}
              </Select>
            );
            break;
          case "long_text":
            control = (
              <textarea
                {...common}
                rows={5}
                value={str}
                onChange={(e) => { onChange(f.key, e.target.value); }}
                className={cn(touchField, "w-full rounded-md bg-white px-2.5 py-1.5 text-sm ring-1 ring-zinc-300 focus:ring-2 focus:ring-brand-500 dark:bg-zinc-900 dark:ring-zinc-700", errId && "ring-red-500")}
              />
            );
            break;
          default:
            control = (
              <Input
                {...common}
                type={htmlType[f.type] ?? "text"}
                inputMode={f.type === "number" ? "decimal" : undefined}
                value={str}
                onChange={(e) => { onChange(f.key, e.target.value); }}
                className={errId ? "ring-red-500" : undefined}
              />
            );
        }
        return (
          <div key={f.key}>
            <label htmlFor={id} className="mb-1.5 block">{label}</label>
            {control}
            <Hint helpId={helpId} help={f.help} errId={errId} error={errors[f.key]} />
          </div>
        );
      })}
    </div>
  );
}

const htmlType: Partial<Record<PublicFormField["type"], string>> = {
  date: "date",
  datetime: "datetime-local",
  url: "url",
  email: "email",
};

function Hint({ helpId, help, errId, error }: { helpId?: string; help?: string; errId?: string; error?: string }) {
  return (
    <>
      {help && <p id={helpId} className="mt-1 text-xs whitespace-pre-line text-zinc-500 dark:text-zinc-400">{help}</p>}
      {error && <p id={errId} className="mt-1 text-sm text-red-600 dark:text-red-400">{error}</p>}
    </>
  );
}
