import { useState, type ReactNode } from "react";

import { useEmailApi, useEmailSettings, type EmailTestResult } from "../../api/email";
import { Button, ErrorState, FieldError, Input, Select, Spinner } from "../../components/ui";
import { tem } from "../../i18n-email";
import { applyPreset, formProblems, initialForm, toInput, type EmailForm, type EmailSettingsView, type PresetId, type TLSMode } from "../../lib/emailSettings";
import { formatDateTime } from "../../lib/format";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

/** Settings → Email: the workspace's own SMTP server (ADR-0038). Admin only. */
export function EmailPage() {
  return <SettingsLayout title={tem.title}>{(ws) => <Email ws={ws} />}</SettingsLayout>;
}

function Email({ ws }: { ws: string }) {
  const q = useEmailSettings(ws);
  if (q.isPending) return <Spinner />;
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  return (
    <div className="space-y-4">
      <StatusNote view={q.data} />
      {/* Remount on save so the form reflects what the server stored. */}
      <SettingsForm key={q.data.settings?.updated_at ?? "none"} ws={ws} view={q.data} />
      <TestCard ws={ws} />
    </div>
  );
}

function StatusNote({ view }: { view: EmailSettingsView }) {
  const s = view.settings;
  let text = tem.usingOwn;
  if (!s) text = view.instance_configured ? tem.fromServer : tem.noServer;
  else if (!s.enabled) text = view.instance_configured ? tem.disabledNote : tem.noServer;
  return (
    <p role="status" className="rounded-lg bg-zinc-100 p-3 text-sm text-zinc-700 dark:bg-zinc-900 dark:text-zinc-300">
      {text}
      {s?.updated_by_name && <span className="mt-1 block text-xs text-zinc-500">{tem.updated(s.updated_by_name, formatDateTime(s.updated_at))}</span>}
    </p>
  );
}

function Field({ label, hint, children }: { label: ReactNode; hint?: string; children: ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block text-zinc-600 dark:text-zinc-400">{label}</span>
      {children}
      {hint && <span className="mt-1 block text-xs text-zinc-500">{hint}</span>}
    </label>
  );
}

function SettingsForm({ ws, view }: { ws: string; view: EmailSettingsView }) {
  const emailApi = useEmailApi(ws);
  const fb = useFeedback();
  const hadPassword = view.settings?.password_set ?? false;
  const [form, setForm] = useState<EmailForm>(() => initialForm(view.settings, view.policy));
  const [preset, setPreset] = useState<PresetId | null>(null);
  const set = (patch: Partial<EmailForm>) => { setForm((f) => ({ ...f, ...patch })); };
  const problems = formProblems(form, view.policy, hadPassword);
  const tlsModes: TLSMode[] = view.policy.allow_plaintext || form.tls_mode === "none" ? ["starttls", "tls", "none"] : ["starttls", "tls"];

  return (
    <Card label={tem.title}>
      <p className="mb-4 text-sm text-zinc-600 dark:text-zinc-400">{tem.intro}</p>
      <form
        className="max-w-xl space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (problems.length > 0) return;
          emailApi.save.mutate(toInput(form, hadPassword), { onSuccess: fb.ok(), onError: fb.fail });
        }}
      >
        <fieldset>
          <legend className="mb-1 text-sm text-zinc-600 dark:text-zinc-400">{tem.presets}</legend>
          <div className="flex flex-wrap gap-2">
            {(["google", "microsoft", "cpanel"] as const).map((id) => (
              <Button key={id} type="button" aria-pressed={preset === id} onClick={() => { setForm((f) => applyPreset(f, id)); setPreset(id); }}>
                {tem.presetNames[id]}
              </Button>
            ))}
          </div>
          {preset && <p className="mt-2 text-xs text-zinc-500">{tem.presetHints[preset]}</p>}
        </fieldset>

        <Field label={tem.from} hint={tem.fromHint}>
          <Input required maxLength={400} autoComplete="off" placeholder="Finance <finance@example.com>" value={form.from_address} onChange={(e) => { set({ from_address: e.target.value }); }} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-[1fr_7rem]">
          <Field label={tem.host}>
            <Input required maxLength={253} autoComplete="off" inputMode="url" placeholder="mail.example.com" value={form.host} onChange={(e) => { set({ host: e.target.value }); }} />
          </Field>
          <Field label={tem.port}>
            <Input required type="number" min={1} max={65535} value={form.port} onChange={(e) => { set({ port: e.target.value }); }} />
          </Field>
        </div>
        <Field label={tem.tls}>
          <Select value={form.tls_mode} onChange={(e) => { set({ tls_mode: e.target.value as TLSMode }); }}>
            {tlsModes.map((m) => <option key={m} value={m}>{tem.tlsModes[m]}</option>)}
          </Select>
        </Field>
        <Field label={tem.username}>
          <Input maxLength={320} autoComplete="off" value={form.username} onChange={(e) => { set({ username: e.target.value }); }} />
        </Field>
        <PasswordField form={form} hadPassword={hadPassword} set={set} />
        <Field label={<>{tem.replyTo} <span className="text-xs">({tem.optional})</span></>}>
          <Input maxLength={400} autoComplete="off" value={form.reply_to} onChange={(e) => { set({ reply_to: e.target.value }); }} />
        </Field>
        <label className="flex items-start gap-2 text-sm">
          <input type="checkbox" className="mt-0.5 accent-brand-600 pointer-coarse:size-5" checked={form.enabled} onChange={(e) => { set({ enabled: e.target.checked }); }} />
          <span>
            {tem.enabled}
            <span className="block text-xs text-zinc-500">{tem.enabledHint}</span>
          </span>
        </label>

        {problems.length > 0 && (
          <ul role="alert" className="space-y-1 text-sm text-red-600 dark:text-red-400">
            {problems.map((p) => <li key={p}>{p === "port" ? tem.problems.port(view.policy.ports.join(", ")) : tem.problems[p]}</li>)}
          </ul>
        )}
        <FieldError error={emailApi.save.error} />
        <div className="flex flex-wrap gap-2">
          <Button type="submit" variant="primary" disabled={emailApi.save.isPending || problems.length > 0}>{tem.save}</Button>
          {view.settings && (
            <Button
              type="button"
              disabled={emailApi.remove.isPending}
              onClick={() => { if (window.confirm(tem.removeConfirm)) emailApi.remove.mutate(undefined, { onSuccess: fb.ok(tem.removed), onError: fb.fail }); }}
            >
              {tem.remove}
            </Button>
          )}
        </div>
      </form>
    </Card>
  );
}

/** Write-only: a stored password shows as "•••• (saved)" with a Change toggle. */
function PasswordField({ form, hadPassword, set }: { form: EmailForm; hadPassword: boolean; set: (p: Partial<EmailForm>) => void }) {
  if (hadPassword && form.passwordAction !== "set") {
    return (
      <div className="text-sm">
        <span className="mb-1 block text-zinc-600 dark:text-zinc-400">{tem.password}</span>
        <div className="flex flex-wrap items-center gap-2">
          <span className={form.passwordAction === "clear" ? "text-zinc-400 line-through" : ""}>{tem.passwordSaved}</span>
          <Button type="button" onClick={() => { set({ passwordAction: "set", password: "" }); }}>{tem.passwordChange}</Button>
          {form.passwordAction === "keep"
            ? <Button type="button" onClick={() => { set({ passwordAction: "clear" }); }}>{tem.passwordClear}</Button>
            : <Button type="button" onClick={() => { set({ passwordAction: "keep" }); }}>{tem.passwordCancel}</Button>}
        </div>
      </div>
    );
  }
  return (
    <Field label={tem.password} hint={tem.passwordHint}>
      <div className="flex gap-2">
        <Input type="password" maxLength={1024} autoComplete="new-password" value={form.password} onChange={(e) => { set({ password: e.target.value }); }} />
        {hadPassword && <Button type="button" onClick={() => { set({ passwordAction: "keep", password: "" }); }}>{tem.passwordCancel}</Button>}
      </div>
    </Field>
  );
}

function TestCard({ ws }: { ws: string }) {
  const emailApi = useEmailApi(ws);
  const [to, setTo] = useState("");
  const [result, setResult] = useState<EmailTestResult | null>(null);
  return (
    <Card label={tem.testTitle}>
      <h2 className="text-sm font-medium">{tem.testTitle}</h2>
      <p className="mt-1 mb-3 text-xs text-zinc-500">{tem.testHint}</p>
      <form
        className="flex max-w-xl flex-col gap-2 sm:flex-row sm:items-end"
        onSubmit={(e) => {
          e.preventDefault();
          setResult(null);
          emailApi.test.mutate(to.trim(), { onSuccess: setResult });
        }}
      >
        <Field label={tem.testTo}>
          <Input type="email" autoComplete="off" value={to} onChange={(e) => { setTo(e.target.value); }} />
        </Field>
        <Button type="submit" disabled={emailApi.test.isPending}>{tem.testSend}</Button>
      </form>
      <div aria-live="polite">
        {emailApi.test.isPending && <Spinner />}
        {result?.ok === true && <p className="mt-2 text-sm text-emerald-700 dark:text-emerald-400">{tem.testOk(result.to, tem.sources[result.source])}</p>}
        {result?.ok === false && <p role="alert" className="mt-2 text-sm text-red-600 dark:text-red-400">{tem.testFail(result.error ?? "")}</p>}
        <FieldError error={emailApi.test.error} />
      </div>
    </Card>
  );
}
