import { useState } from "react";

import { useAdminApi, useDeliveries, useWebhooks, type Webhook } from "../../api/admin";
import { useProjects } from "../../api/hooks";
import { Button, ErrorState, Input, Select, Spinner } from "../../components/ui";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";
import { cn } from "../../lib/cn";
import { formatRelative } from "../../lib/format";
import { defaultEvents, looksLikeChatWebhook, WEBHOOK_FORMATS, type WebhookFormat } from "../../lib/webhooks";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

const parseEvents = (s: string) => s.split(",").map((x) => x.trim()).filter(Boolean);

export function WebhooksPage() {
  return <SettingsLayout title={ta.nav.webhooks}>{(ws) => <Webhooks ws={ws} />}</SettingsLayout>;
}

function Webhooks({ ws }: { ws: string }) {
  const q = useWebhooks(ws);
  const [secret, setSecret] = useState<string | null>(null);
  if (q.isPending) return <Spinner />;
  if (q.isError) return <ErrorState error={q.error} />;
  return (
    <div className="space-y-4">
      <NewWebhook ws={ws} onSecret={setSecret} />
      {secret && <SecretOnce secret={secret} onDismiss={() => { setSecret(null); }} />}
      {q.data.map((h) => <WebhookCard key={h.id} ws={ws} hook={h} onSecret={setSecret} />)}
    </div>
  );
}

/** The signing secret is shown exactly once, right after creation/rotation. */
function SecretOnce({ secret, onDismiss }: { secret: string; onDismiss: () => void }) {
  const [copied, setCopied] = useState(false);
  return (
    <div role="alert" className="rounded-xl bg-amber-50 p-4 text-sm text-amber-900 ring-1 ring-amber-300 dark:bg-amber-950 dark:text-amber-100 dark:ring-amber-800">
      <p className="mb-2">{ta.secretOnce}</p>
      <div className="flex flex-wrap items-center gap-2">
        <code className="rounded bg-white px-2 py-1 font-mono text-xs break-all select-all dark:bg-zinc-900">{secret}</code>
        <Button
          onClick={() => {
            void navigator.clipboard.writeText(secret).then(() => { setCopied(true); });
          }}
        >
          {copied ? ta.copied : ta.copy}
        </Button>
        <Button variant="ghost" onClick={onDismiss}>{t.close}</Button>
      </div>
    </div>
  );
}

function NewWebhook({ ws, onSecret }: { ws: string; onSecret: (s: string) => void }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const [url, setUrl] = useState("");
  const [format, setFormat] = useState<WebhookFormat>("nebula");
  const [events, setEvents] = useState(defaultEvents("nebula"));
  const [projects, setProjects] = useState<string[]>([]);
  const chooseFormat = (f: WebhookFormat) => {
    // Swap the events only while they are still the previous default.
    if (events === defaultEvents(format)) setEvents(defaultEvents(f));
    setFormat(f);
  };
  const suggestChat = format === "nebula" && looksLikeChatWebhook(url);
  return (
    <Card>
      <h2 className="mb-3 text-sm font-semibold">{ta.newWebhook}</h2>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          adm.createWebhook.mutate({ url: url.trim(), format, events: parseEvents(events), project_ids: projects }, {
            onSuccess: (r) => {
              setUrl("");
              if (r.secret) onSecret(r.secret);
              else fb.ok()();
            },
            onError: fb.fail,
          });
        }}
      >
        <label className="flex flex-col gap-1 text-xs text-zinc-500">
          {ta.format}
          <Select value={format} onChange={(e) => { chooseFormat(e.target.value as WebhookFormat); }} className="w-52">
            {WEBHOOK_FORMATS.map((f) => <option key={f} value={f}>{ta.formats[f]}</option>)}
          </Select>
        </label>
        <label className="flex min-w-[min(16rem,100%)] flex-1 flex-col gap-1 text-xs text-zinc-500">
          {ta.url}
          <Input
            required
            type="url"
            placeholder="https://"
            autoComplete="off"
            spellCheck={false}
            value={url}
            onChange={(e) => { setUrl(e.target.value); }}
          />
        </label>
        <label className="flex flex-col gap-1 text-xs text-zinc-500">
          <span>{ta.events} <span className="text-zinc-400">({ta.eventsHint})</span></span>
          <Input required value={events} onChange={(e) => { setEvents(e.target.value); }} className="w-64" />
        </label>
        <ProjectFilter ws={ws} value={projects} onChange={setProjects} />
        <Button type="submit" variant="primary" disabled={adm.createWebhook.isPending}>{t.create}</Button>
      </form>
      {format === "slack" && <p className="mt-2 text-xs text-zinc-500">{ta.slackHint}</p>}
      {suggestChat && (
        <p role="status" className="mt-2 flex flex-wrap items-center gap-2 text-xs text-amber-700 dark:text-amber-400">
          {ta.chatUrlDetected}
          <Button variant="ghost" onClick={() => { chooseFormat("slack"); }}>{ta.useChatFormat}</Button>
        </p>
      )}
    </Card>
  );
}

/** Which projects a webhook covers; none ticked = every workspace-visible project. */
function ProjectFilter({ ws, value, onChange }: { ws: string; value: string[]; onChange: (ids: string[]) => void }) {
  const projects = useProjects(ws);
  const list = projects.data ?? [];
  const summary = value.length === 0 ? ta.webhookProjects.all : ta.webhookProjects.selected(value.length);
  return (
    <details className="relative text-xs">
      <summary className="cursor-pointer list-none rounded-md px-2.5 py-1.5 ring-1 ring-zinc-300 dark:ring-zinc-700" aria-label={`${ta.webhookProjects.label}: ${summary}`}>
        {ta.webhookProjects.label}: {summary} ▾
      </summary>
      <div className="absolute z-10 mt-1 w-72 max-w-[calc(100vw-2rem)] space-y-1 rounded-md bg-white p-2 shadow-lg ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-700">
        {list.map((p) => (
          <label key={p.id} className="flex items-center gap-2 py-0.5">
            <input
              type="checkbox"
              className="accent-brand-600"
              checked={value.includes(p.id)}
              onChange={(e) => { onChange(e.target.checked ? [...value, p.id] : value.filter((x) => x !== p.id)); }}
            />
            <span className="font-mono text-zinc-500">{p.key}</span> {p.name} {p.visibility === "private" && "🔒"}
          </label>
        ))}
        <p className="border-t border-zinc-100 pt-1.5 text-zinc-500 dark:border-zinc-800">{ta.webhookProjects.hint}</p>
      </div>
    </details>
  );
}

function WebhookCard({ ws, hook, onSecret }: { ws: string; hook: Webhook; onSecret: (s: string) => void }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const [showLog, setShowLog] = useState(false);
  const [events, setEvents] = useState(hook.events.join(", "));
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-2">
        <span className={cn("size-2 rounded-full", hook.active ? "bg-emerald-500" : "bg-zinc-400")} aria-hidden />
        <code className="min-w-0 flex-1 truncate text-sm">{hook.url}</code>
        <span className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300">
          {ta.formatShort[hook.format] ?? hook.format}
        </span>
        <label className="flex items-center gap-1.5 text-xs">
          <input
            type="checkbox"
            className="accent-brand-600"
            checked={hook.active}
            onChange={(e) => { adm.updateWebhook.mutate({ id: hook.id, body: { active: e.target.checked } }, { onSuccess: fb.ok(), onError: fb.fail }); }}
          />
          {hook.active ? ta.active : ta.inactive}
        </label>
      </div>
      {(hook.failure_count > 0 || hook.disabled_reason) && (
        <p className="mt-1 text-xs text-amber-700 dark:text-amber-400">
          {ta.failures(hook.failure_count)}{hook.disabled_reason ? ` · ${hook.disabled_reason}` : ""}
        </p>
      )}
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Input
          aria-label={ta.events}
          value={events}
          className="w-72 text-xs"
          onChange={(e) => { setEvents(e.target.value); }}
          onBlur={() => {
            if (events !== hook.events.join(", ")) adm.updateWebhook.mutate({ id: hook.id, body: { events: parseEvents(events) } }, { onSuccess: fb.ok(), onError: fb.fail });
          }}
        />
        <ProjectFilter
          ws={ws}
          value={hook.project_ids}
          onChange={(ids) => { adm.updateWebhook.mutate({ id: hook.id, body: { project_ids: ids } }, { onSuccess: fb.ok(), onError: fb.fail }); }}
        />
        <Button onClick={() => { adm.pingWebhook.mutate(hook.id, { onSuccess: () => { fb.ok(hook.format === "slack" ? ta.pingedChat : ta.pinged)(); setShowLog(true); }, onError: fb.fail }); }}>{ta.ping}</Button>
        <Button variant="ghost" onClick={() => { setShowLog((v) => !v); }}>{ta.deliveries}</Button>
        {hook.format !== "slack" && (
          <Button
            variant="ghost"
            onClick={() => {
              if (window.confirm(ta.confirmRotate)) adm.rotateSecret.mutate(hook.id, { onSuccess: (r) => { onSecret(r.secret); }, onError: fb.fail });
            }}
          >
            {ta.rotate}
          </Button>
        )}
        <Button
          variant="ghost"
          className="ml-auto text-red-600"
          onClick={() => { if (window.confirm(ta.confirm)) adm.deleteWebhook.mutate(hook.id, { onSuccess: fb.ok(), onError: fb.fail }); }}
        >
          {t.delete}
        </Button>
      </div>
      {showLog && <DeliveryLog ws={ws} id={hook.id} />}
    </Card>
  );
}

function DeliveryLog({ ws, id }: { ws: string; id: string }) {
  const q = useDeliveries(ws, id);
  if (q.isPending) return <Spinner />;
  if (q.isError) return <ErrorState error={q.error} />;
  if (q.data.length === 0) return <p className="mt-3 text-xs text-zinc-500">{ta.noDeliveries}</p>;
  return (
    <div className="mt-3 overflow-x-auto">
      <table className="w-full text-left text-xs">
        <thead className="text-zinc-500">
          <tr><th className="py-1">#</th><th>HTTP</th><th>ms</th><th>try</th><th>error</th><th /></tr>
        </thead>
        <tbody>
          {q.data.map((d) => {
            const ok = d.status_code !== null && d.status_code >= 200 && d.status_code < 300;
            return (
              <tr key={d.id} className="border-t border-zinc-100 dark:border-zinc-800">
                <td className="py-1 font-mono">{d.event_id}</td>
                <td className={ok ? "text-emerald-600" : "text-red-600"}>{d.status_code ?? "—"}</td>
                <td>{d.duration_ms}</td>
                <td>{d.attempt}</td>
                <td className="max-w-64 truncate text-zinc-500" title={d.error ?? ""}>{d.error ?? ""}</td>
                <td className="text-zinc-500">{formatRelative(d.delivered_at)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
