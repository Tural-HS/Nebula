import { useState, type ReactNode } from "react";

import { useProjects } from "../../api/hooks";
import { useServiceTokenApi, useServiceTokens } from "../../api/serviceTokens";
import { Button, Dialog, EmptyState, ErrorState, FieldError, Input, Select, Spinner } from "../../components/ui";
import { tp } from "../../i18n-project";
import { ttk } from "../../i18n-tokens";
import { cn } from "../../lib/cn";
import { formatDateTime, formatRelative } from "../../lib/format";
import {
  curlExample, EXPIRY_PRESETS, isWriteScope, scopesForRole, TOKEN_ROLES, TOKEN_SCOPES, tokenProblems, tokenState,
  type CreatedToken, type ServiceToken, type TokenInput, type TokenRole, type TokenScope,
} from "../../lib/serviceTokens";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

/** Settings → API tokens: service tokens for integrations (ADR-0043). Admin only. */
export function TokensPage() {
  return <SettingsLayout title={ttk.title}>{(ws) => <Tokens ws={ws} />}</SettingsLayout>;
}

function Tokens({ ws }: { ws: string }) {
  const q = useServiceTokens(ws);
  const [creating, setCreating] = useState(false);
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <p className="max-w-2xl text-sm text-zinc-600 dark:text-zinc-400">{ttk.intro}</p>
        <Button variant="primary" onClick={() => { setCreating(true); }}>{ttk.create}</Button>
      </div>
      <CreateDialog ws={ws} open={creating} onClose={() => { setCreating(false); }} />
      {q.isPending ? (
        <Spinner />
      ) : q.isError ? (
        <ErrorState error={q.error} onRetry={() => void q.refetch()} />
      ) : q.data.length === 0 ? (
        <EmptyState>{ttk.empty}</EmptyState>
      ) : (
        <ul className="space-y-3" aria-label={ttk.title}>
          {q.data.map((tok) => <TokenCard key={tok.id} ws={ws} tok={tok} />)}
        </ul>
      )}
    </div>
  );
}

const stateStyle = {
  active: "bg-emerald-50 text-emerald-800 ring-emerald-200 dark:bg-emerald-950 dark:text-emerald-200 dark:ring-emerald-800",
  expired: "bg-amber-50 text-amber-800 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-800",
  revoked: "bg-zinc-100 text-zinc-600 ring-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:ring-zinc-700",
} as const;

function TokenCard({ ws, tok }: { ws: string; tok: ServiceToken }) {
  const tokenApi = useServiceTokenApi(ws);
  const fb = useFeedback();
  const [confirm, setConfirm] = useState(false);
  const state = tokenState(tok);
  return (
    <li>
      <Card label={tok.name} className={cn(state !== "active" && "opacity-75")}>
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="min-w-0 flex-1 text-sm font-semibold break-words">{tok.display_name}</h2>
          <span className={cn("rounded-full px-2 py-0.5 text-xs ring-1", stateStyle[state])}>{ttk.states[state]}</span>
          {state !== "revoked" && (
            <Button variant="dangerGhost" onClick={() => { setConfirm(true); }}>{ttk.revoke}</Button>
          )}
        </div>
        <dl className="mt-3 grid gap-x-4 gap-y-2 text-sm sm:grid-cols-2">
          <Row label={ttk.role}>{ttk.roles[tok.role]}</Row>
          <Row label={ttk.scopes}>
            <span className="flex flex-wrap gap-1">
              {tok.scopes.map((s) => <code key={s} className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs dark:bg-zinc-800">{s}</code>)}
            </span>
          </Row>
          <Row label={ttk.projects}>{tok.projects.length > 0 ? tok.projects.map((p) => p.key).join(", ") : tok.project_restricted ? ttk.noProjects : ttk.allProjects}</Row>
          <Row label={ttk.cols.createdBy}>
            {tok.created_by_name ?? "—"} <span className="text-xs text-zinc-500 dark:text-zinc-400">({formatDateTime(tok.created_at)})</span>
          </Row>
          <Row label={ttk.cols.expires}>{formatDateTime(tok.expires_at)}</Row>
          <Row label={ttk.cols.lastUsed}>
            <span data-testid="token-last-used">
              {tok.last_used_at ? `${formatRelative(tok.last_used_at)}${tok.last_used_ip ? ` · ${tok.last_used_ip}` : ""}` : ttk.cols.never}
            </span>
          </Row>
        </dl>
      </Card>
      <Dialog open={confirm} onClose={() => { setConfirm(false); }} title={ttk.revokeTitle}>
        <p className="text-sm">{ttk.revokeBody(tok.name)}</p>
        <FieldError error={tokenApi.revoke.error} />
        <div className="mt-4 flex flex-wrap justify-end gap-2">
          <Button onClick={() => { setConfirm(false); }}>{ttk.cancel}</Button>
          <Button
            variant="danger"
            disabled={tokenApi.revoke.isPending}
            onClick={() => {
              tokenApi.revoke.mutate(tok.id, { onSuccess: () => { setConfirm(false); fb.ok(ttk.revoked)(); } });
            }}
          >
            {ttk.revokeConfirm}
          </Button>
        </div>
      </Dialog>
    </li>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-zinc-500 dark:text-zinc-400">{label}</dt>
      <dd className="break-words">{children}</dd>
    </div>
  );
}

const emptyInput = (): TokenInput => ({ name: "", role: "member", scopes: ["items:read"], project_ids: [], expires_in_days: 90 });

function CreateDialog({ ws, open, onClose }: { ws: string; open: boolean; onClose: () => void }) {
  const tokenApi = useServiceTokenApi(ws);
  const [created, setCreated] = useState<CreatedToken | null>(null);
  const close = () => {
    setCreated(null);
    tokenApi.create.reset();
    onClose();
  };
  return (
    <Dialog open={open} onClose={close} title={created ? ttk.secretTitle : ttk.create}>
      {created ? (
        <SecretOnce ws={ws} created={created} onDone={close} />
      ) : (
        <CreateForm ws={ws} onCancel={close} onCreated={setCreated} />
      )}
    </Dialog>
  );
}

function CreateForm({ ws, onCancel, onCreated }: { ws: string; onCancel: () => void; onCreated: (t: CreatedToken) => void }) {
  const tokenApi = useServiceTokenApi(ws);
  const projects = useProjects(ws);
  const [input, setInput] = useState<TokenInput>(emptyInput);
  const [touched, setTouched] = useState(false);
  const problems = tokenProblems(input);
  const set = (patch: Partial<TokenInput>) => { setInput((v) => ({ ...v, ...patch })); };
  const toggleScope = (s: TokenScope, on: boolean) => {
    set({ scopes: scopesForRole(input.role, on ? [...input.scopes, s] : input.scopes.filter((x) => x !== s)) });
  };
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        setTouched(true);
        if (problems.length > 0) return;
        tokenApi.create.mutate({ ...input, name: input.name.trim() }, { onSuccess: onCreated });
      }}
    >
      <label className="block text-sm">
        <span className="mb-1 block text-zinc-600 dark:text-zinc-400">{ttk.name}</span>
        <Input required maxLength={80} autoComplete="off" placeholder={ttk.namePlaceholder} value={input.name} onChange={(e) => { set({ name: e.target.value }); }} />
      </label>
      <label className="block text-sm">
        <span className="mb-1 block text-zinc-600 dark:text-zinc-400">{ttk.role}</span>
        <Select
          value={input.role}
          onChange={(e) => {
            const role = e.target.value as TokenRole;
            set({ role, scopes: scopesForRole(role, input.scopes) });
          }}
        >
          {TOKEN_ROLES.map((r) => <option key={r} value={r}>{ttk.roles[r]}</option>)}
        </Select>
      </label>
      <fieldset>
        <legend className="mb-1 text-sm text-zinc-600 dark:text-zinc-400">{ttk.scopes}</legend>
        <div className="space-y-1.5">
          {TOKEN_SCOPES.map((s) => {
            const disabled = input.role === "guest" && isWriteScope(s);
            return (
              <label key={s} className={cn("flex items-start gap-2 text-sm", disabled && "opacity-60")}>
                <input
                  type="checkbox"
                  className="mt-0.5 accent-brand-600 pointer-coarse:size-5"
                  checked={input.scopes.includes(s)}
                  disabled={disabled}
                  onChange={(e) => { toggleScope(s, e.target.checked); }}
                />
                <span>
                  {ttk.scopeNames[s]} <code className="text-xs text-zinc-500 dark:text-zinc-400">{s}</code>
                </span>
              </label>
            );
          })}
        </div>
        {input.role === "guest" && <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{ttk.guestReadOnly}</p>}
      </fieldset>
      <fieldset>
        <legend className="mb-1 text-sm text-zinc-600 dark:text-zinc-400">{ttk.projects}</legend>
        <div className="max-h-40 space-y-1 overflow-y-auto">
          {(projects.data ?? []).map((p) => (
            <label key={p.id} className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="accent-brand-600 pointer-coarse:size-5"
                checked={input.project_ids.includes(p.id)}
                onChange={(e) => {
                  set({ project_ids: e.target.checked ? [...input.project_ids, p.id] : input.project_ids.filter((x) => x !== p.id) });
                }}
              />
              <span className="font-mono text-xs text-zinc-500 dark:text-zinc-400">{p.key}</span>
              <span className="min-w-0 truncate">{p.name}</span>
              {p.visibility === "private" && <span role="img" aria-label={tp.privateProject}>🔒</span>}
            </label>
          ))}
        </div>
        <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
          {input.project_ids.length === 0 ? ttk.allProjects + ". " : ""}{ttk.projectsHint}
        </p>
      </fieldset>
      <fieldset>
        <legend className="mb-1 text-sm text-zinc-600 dark:text-zinc-400">{ttk.expires}</legend>
        <div className="flex flex-wrap gap-2">
          {EXPIRY_PRESETS.map((d) => (
            <Button key={d} type="button" aria-pressed={input.expires_in_days === d} variant={input.expires_in_days === d ? "primary" : "secondary"} onClick={() => { set({ expires_in_days: d }); }}>
              {ttk.days(d)}
            </Button>
          ))}
        </div>
      </fieldset>
      {touched && problems.length > 0 && (
        <ul role="alert" className="space-y-1 text-sm text-red-600 dark:text-red-400">
          {problems.map((p) => <li key={p}>{ttk.problems[p]}</li>)}
        </ul>
      )}
      <FieldError error={tokenApi.create.error} />
      <div className="flex flex-wrap justify-end gap-2">
        <Button type="button" onClick={onCancel}>{ttk.cancel}</Button>
        <Button type="submit" variant="primary" disabled={tokenApi.create.isPending}>{ttk.submit}</Button>
      </div>
    </form>
  );
}

/** The secret exists only here, once; closing the dialog forgets it. */
function SecretOnce({ ws, created, onDone }: { ws: string; created: CreatedToken; onDone: () => void }) {
  const [copied, setCopied] = useState(false);
  const project = created.projects[0]?.key ?? "KEY";
  return (
    <div className="space-y-3 text-sm">
      <p role="alert" className="rounded-lg bg-amber-50 p-3 text-amber-900 ring-1 ring-amber-300 dark:bg-amber-950 dark:text-amber-100 dark:ring-amber-800">
        {ttk.secretWarning}
      </p>
      <code data-testid="token-secret" className="block rounded bg-zinc-100 px-2 py-1.5 font-mono text-xs break-all select-all dark:bg-zinc-800">
        {created.token}
      </code>
      <div className="flex flex-wrap gap-2">
        <Button onClick={() => { void navigator.clipboard.writeText(created.token).then(() => { setCopied(true); }); }}>
          {copied ? ttk.copied : ttk.copy}
        </Button>
      </div>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{ttk.usage}</p>
      <code className="block rounded bg-zinc-100 px-2 py-1.5 font-mono text-2xs break-all dark:bg-zinc-800">
        {curlExample(window.location.origin, ws, project)}
      </code>
      <div className="flex justify-end">
        <Button variant="primary" onClick={onDone}>{ttk.done}</Button>
      </div>
    </div>
  );
}
