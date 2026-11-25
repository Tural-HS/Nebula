// Archive, restore and delete projects and workspaces (ADR-0039): danger
// zones in settings, the archived banner and states, the archived lists.
import { useId, useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { ApiError } from "../api/client";
import { useWorkspace } from "../api/hooks";
import { useArchivedProjects, useArchivedWorkspaces, useProjectLifecycle, useWorkspaceLifecycle } from "../api/lifecycle";
import type { Project, Role, Workspace } from "../api/types";
import { t } from "../i18n";
import { tlc } from "../i18n-lifecycle";
import { archivedDate, confirmMatches, deleteOutcome, projectActions, workspaceActions, type ProjectGone } from "../lib/lifecycle";
import { formatDate } from "../lib/format";
import { useToast } from "./toast-context";
import { Button, Dialog, EmptyState, FieldError, Input, Label } from "./ui";

const sectionCls = "space-y-4 rounded-xl bg-white p-4 ring-1 ring-red-200 dark:bg-zinc-900 dark:ring-red-900";

/** The red-ringed "Təhlükəli zona" section of a settings page. */
function DangerZone({ children }: { children: ReactNode }) {
  const id = useId();
  return (
    <section aria-labelledby={id} className={sectionCls}>
      <h2 id={id} className="text-sm font-semibold text-red-700 dark:text-red-400">{tlc.dangerZone}</h2>
      {children}
    </section>
  );
}

function Row({ title, hint, children }: { title: string; hint: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col gap-3 border-t border-zinc-100 pt-4 first-of-type:border-t-0 first-of-type:pt-0 sm:flex-row sm:items-start sm:justify-between dark:border-zinc-800">
      <div className="min-w-0 space-y-1">
        <h3 className="text-sm font-medium">{title}</h3>
        <p className="text-xs text-zinc-500 dark:text-zinc-400">{hint}</p>
      </div>
      {children && <div className="shrink-0">{children}</div>}
    </div>
  );
}

function errorText(err: unknown): string {
  return err instanceof ApiError ? err.message : t.errorGeneric;
}

/** A confirmation dialog for a reversible action (archive), naming the object. */
function ConfirmDialog({ open, onClose, title, hint, action, pending, error, onConfirm }: {
  open: boolean; onClose: () => void; title: string; hint: string; action: string; pending: boolean; error: unknown; onConfirm: () => void;
}) {
  return (
    <Dialog open={open} onClose={onClose} title={title}>
      <p className="text-sm text-zinc-600 dark:text-zinc-400">{hint}</p>
      <FieldError error={error} />
      <div className="mt-4 flex flex-wrap justify-end gap-2">
        <Button onClick={onClose}>{tlc.cancel}</Button>
        <Button variant="primary" disabled={pending} onClick={onConfirm}>{action}</Button>
      </div>
    </Dialog>
  );
}

/** The permanent-delete dialog: the object's key or slug must be typed. */
function TypedDeleteDialog({ open, onClose, title, consequences, prompt, expected, caseInsensitive, action, pending, error, extra, ready, onConfirm }: {
  open: boolean; onClose: () => void; title: string; consequences: string; prompt: string; expected: string; action: string;
  caseInsensitive?: boolean; pending: boolean; error: unknown; extra?: ReactNode; ready?: boolean; onConfirm: (typed: string) => void;
}) {
  const [typed, setTyped] = useState("");
  const inputId = useId();
  const ok = confirmMatches(typed, expected, caseInsensitive) && ready !== false;
  return (
    <Dialog open={open} onClose={() => { setTyped(""); onClose(); }} title={title}>
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (ok) onConfirm(typed.trim());
        }}
      >
        <p className="text-sm text-red-800 dark:text-red-300">{consequences}</p>
        {extra}
        <div>
          <Label htmlFor={inputId}>{prompt}</Label>
          <Input id={inputId} autoComplete="off" spellCheck={false} value={typed} onChange={(e) => { setTyped(e.target.value); }} />
        </div>
        <FieldError error={error} />
        <div className="flex flex-wrap justify-end gap-2">
          <Button onClick={() => { setTyped(""); onClose(); }}>{tlc.cancel}</Button>
          <Button type="submit" variant="danger" disabled={!ok || pending} className="disabled:opacity-50">{action}</Button>
        </div>
      </form>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Workspace
// ---------------------------------------------------------------------------

/** Workspace settings → General: archive, restore, delete. */
export function WorkspaceDangerZone({ ws, workspace }: { ws: string; workspace: Workspace }) {
  const archived = Boolean(workspace.archived_at);
  const can = workspaceActions(workspace.role, archived);
  const life = useWorkspaceLifecycle(ws);
  const toast = useToast();
  const navigate = useNavigate();
  const [dialog, setDialog] = useState<"archive" | "delete" | null>(null);
  const [ack, setAck] = useState(false);
  const ackId = useId();
  const close = () => { setDialog(null); setAck(false); life.archive.reset(); life.remove.reset(); };
  const fail = (err: unknown) => { toast("error", errorText(err)); };

  return (
    <DangerZone>
      {!archived && (
        <Row title={tlc.workspace.archiveTitle} hint={can.archive ? tlc.workspace.archiveHint : tlc.workspace.ownerOnly}>
          {can.archive && <Button variant="dangerGhost" onClick={() => { setDialog("archive"); }}>{tlc.workspace.archive}</Button>}
        </Row>
      )}
      {archived && (
        <Row title={tlc.restore} hint={tlc.workspace.bannerAdmin}>
          {can.restore && (
            <Button
              disabled={life.restore.isPending}
              onClick={() => { life.restore.mutate(undefined, { onSuccess: () => { toast("success", tlc.workspace.restored); }, onError: fail }); }}
            >
              {tlc.restore}
            </Button>
          )}
        </Row>
      )}
      <Row title={tlc.workspace.deleteTitle} hint={archived ? tlc.workspace.deleteHint : tlc.workspace.deleteNeedsArchive}>
        {can.delete && <Button variant="danger" onClick={() => { setDialog("delete"); }}>{tlc.workspace.deleteButton}</Button>}
      </Row>

      <ConfirmDialog
        open={dialog === "archive"}
        onClose={close}
        title={tlc.workspace.archiveConfirm(workspace.name)}
        hint={tlc.workspace.archiveConfirmHint}
        action={tlc.workspace.archive}
        pending={life.archive.isPending}
        error={life.archive.error}
        onConfirm={() => { life.archive.mutate(undefined, { onSuccess: () => { close(); toast("success", tlc.workspace.archived); } }); }}
      />
      <TypedDeleteDialog
        open={dialog === "delete"}
        onClose={close}
        title={tlc.workspace.deleteConfirm(workspace.name)}
        consequences={tlc.workspace.deleteHint}
        prompt={tlc.workspace.typeSlug(workspace.slug)}
        expected={workspace.slug}
        caseInsensitive
        action={tlc.workspace.deleteButton}
        pending={life.remove.isPending}
        error={life.remove.error}
        ready={ack}
        extra={
          <div role="note" className="space-y-2 rounded-md bg-amber-50 p-3 text-sm text-amber-900 ring-1 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-900">
            <p className="font-medium">{tlc.workspace.evidenceTitle}</p>
            <p className="text-xs">{tlc.workspace.evidenceHint}</p>
            <Link to={`/w/${encodeURIComponent(ws)}/settings/audit`} className="inline-block text-xs font-medium underline">
              {tlc.workspace.evidenceLink}
            </Link>
            <label htmlFor={ackId} className="flex items-start gap-2 text-xs">
              <input id={ackId} type="checkbox" className="mt-0.5 accent-brand-600 pointer-coarse:size-5" checked={ack} onChange={(e) => { setAck(e.target.checked); }} />
              <span>{tlc.workspace.evidenceAck}</span>
            </label>
          </div>
        }
        onConfirm={(confirm) => {
          life.remove.mutateAsync({ confirm, evidence_acknowledged: ack }).then((res) => {
            toast("success", deleteOutcome(res) === "deleted" ? tlc.workspace.deleted : tlc.workspace.deleting);
            void navigate("/");
          }, () => undefined); // the dialog shows the error
        }}
      />
    </DangerZone>
  );
}

/** Under the header of every page of an archived workspace. */
export function ArchivedWorkspaceBanner() {
  const { ws = "" } = useParams();
  const workspace = useWorkspace(ws);
  const life = useWorkspaceLifecycle(ws);
  const toast = useToast();
  if (!ws || !workspace.data?.archived_at) return null;
  const can = workspaceActions(workspace.data.role, true);
  return (
    <div role="status" className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-amber-200 bg-amber-50 px-4 py-2 text-sm text-amber-900 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-200">
      <span className="min-w-0 flex-1">{can.restore ? tlc.workspace.bannerAdmin : tlc.workspace.banner}</span>
      {can.restore && (
        <Button
          disabled={life.restore.isPending}
          onClick={() => {
            life.restore.mutate(undefined, {
              onSuccess: () => { toast("success", tlc.workspace.restored); },
              onError: (err) => { toast("error", errorText(err)); },
            });
          }}
        >
          {tlc.restore}
        </Button>
      )}
    </div>
  );
}

/** Home page: archived workspaces the user administers, with Restore. */
export function ArchivedWorkspaces() {
  const list = useArchivedWorkspaces();
  if (!list.data || list.data.length === 0) return null;
  return (
    <section aria-labelledby="home-archived" data-testid="archived-workspaces" className="space-y-2">
      <h2 id="home-archived" className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">{tlc.home.title}</h2>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{tlc.home.hint}</p>
      <ul className="divide-y divide-zinc-100 rounded-xl bg-white ring-1 ring-zinc-200 dark:divide-zinc-800 dark:bg-zinc-900 dark:ring-zinc-800">
        {list.data.map((w) => <ArchivedWorkspaceRow key={w.id} w={w} />)}
      </ul>
    </section>
  );
}

function ArchivedWorkspaceRow({ w }: { w: Workspace }) {
  const life = useWorkspaceLifecycle(w.slug);
  const toast = useToast();
  const date = archivedDate(w.archived_at);
  return (
    <li className="flex flex-wrap items-center gap-3 px-4 py-3 text-sm">
      <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">
        <span className="block font-medium">{w.name}</span>
        <span className="block text-xs text-zinc-500 dark:text-zinc-400">{w.slug}{date ? ` · ${tlc.archivedOn(formatDate(date))}` : ""}</span>
      </span>
      <Link to={`/w/${encodeURIComponent(w.slug)}`} className="rounded-md px-3 py-1.5 text-sm font-medium ring-1 ring-zinc-300 hover:bg-zinc-50 pointer-coarse:min-h-11 dark:ring-zinc-700 dark:hover:bg-zinc-800">
        {tlc.open}
      </Link>
      <Button
        disabled={life.restore.isPending}
        aria-label={`${tlc.restore}: ${w.name}`}
        onClick={() => {
          life.restore.mutate(undefined, {
            onSuccess: () => { toast("success", tlc.workspace.restored); },
            onError: (err) => { toast("error", errorText(err)); },
          });
        }}
      >
        {tlc.restore}
      </Button>
    </li>
  );
}

// ---------------------------------------------------------------------------
// Project
// ---------------------------------------------------------------------------

/** Project settings: archive (admins). Deleting needs an archived project. */
export function ProjectDangerZone({ ws, project, role }: { ws: string; project: Project; role: Role | undefined }) {
  const can = projectActions(role, false);
  const life = useProjectLifecycle(ws, project.key);
  const toast = useToast();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const close = () => { setOpen(false); life.archive.reset(); };
  return (
    <DangerZone>
      <Row title={tlc.project.archiveTitle} hint={tlc.project.archiveHint}>
        {can.archive && <Button variant="dangerGhost" onClick={() => { setOpen(true); }}>{tlc.project.archive}</Button>}
      </Row>
      <Row title={tlc.project.deleteTitle} hint={tlc.project.deleteHint} />
      <ConfirmDialog
        open={open}
        onClose={close}
        title={tlc.project.archiveConfirm(project.name)}
        hint={tlc.project.archiveHint}
        action={tlc.project.archive}
        pending={life.archive.isPending}
        error={life.archive.error}
        onConfirm={() => {
          // mutateAsync: once the caches refresh, this page is replaced by
          // the archived state; the toast and navigation must still happen.
          life.archive.mutateAsync().then(() => {
            toast("success", tlc.project.archived);
            void navigate(`/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project.key)}`);
          }, () => undefined); // the dialog shows the error
        }}
      />
    </DangerZone>
  );
}

/** What a deep link to an archived (or deleting) project shows. */
export function ArchivedProjectState({ ws, projectKey, gone, role }: {
  ws: string; projectKey: string; gone: ProjectGone; role: Role | undefined;
}) {
  const can = projectActions(role, true);
  const life = useProjectLifecycle(ws, projectKey);
  // Admins can list archived projects: that gives the name for the heading.
  const archived = useArchivedProjects(ws, can.restore && gone === "archived");
  const project = archived.data?.find((p) => p.key === projectKey);
  const toast = useToast();
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState(false);
  if (gone === "missing") return <EmptyState>{t.notFound}</EmptyState>;
  if (gone === "deleting") {
    return (
      <div role="status" className="mx-auto max-w-xl p-6 text-center">
        <h1 className="text-lg font-semibold">{tlc.project.deletingTitle}</h1>
        <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">{tlc.project.deleting}</p>
      </div>
    );
  }
  const name = project?.name ?? projectKey;
  return (
    <div className="mx-auto max-w-xl space-y-4 p-6" data-testid="archived-project">
      <div className="rounded-xl bg-white p-5 ring-1 ring-amber-200 dark:bg-zinc-900 dark:ring-amber-900">
        <h1 className="text-lg font-semibold">{tlc.project.stateTitle}</h1>
        <p className="mt-1 text-sm [overflow-wrap:anywhere]">
          <span className="font-mono text-xs">{projectKey}</span>
          {project && ` · ${project.name}`}
        </p>
        <p className="mt-2 text-sm text-zinc-600 dark:text-zinc-400">{tlc.project.stateHint}</p>
        {!can.restore && <p className="mt-2 text-sm text-zinc-600 dark:text-zinc-400">{tlc.project.stateAskAdmin}</p>}
        {(can.restore || can.delete) && (
          <div className="mt-4 flex flex-wrap gap-2">
            {can.restore && (
              <Button
                variant="primary"
                disabled={life.restore.isPending}
                onClick={() => {
                  life.restore.mutate(undefined, {
                    onSuccess: () => { toast("success", tlc.project.restored); },
                    onError: (err) => { toast("error", errorText(err)); },
                  });
                }}
              >
                {tlc.restore}
              </Button>
            )}
            {can.delete && <Button variant="dangerGhost" onClick={() => { setDeleting(true); }}>{tlc.project.deleteTitle}</Button>}
          </div>
        )}
      </div>
      <TypedDeleteDialog
        open={deleting}
        onClose={() => { setDeleting(false); life.remove.reset(); }}
        title={tlc.project.deleteConfirm(name)}
        consequences={tlc.project.deleteConsequences}
        prompt={tlc.project.typeKey(projectKey)}
        expected={projectKey}
        action={tlc.project.deleteButton}
        pending={life.remove.isPending}
        error={life.remove.error}
        onConfirm={(confirm) => {
          // mutateAsync: the state below may change (the project is gone)
          // before the caches are refreshed; the navigation must still happen.
          life.remove.mutateAsync({ confirm }).then((res) => {
            toast("success", deleteOutcome(res) === "deleted" ? tlc.project.deleted : tlc.project.deleting);
            void navigate(`/w/${encodeURIComponent(ws)}`);
          }, () => undefined); // the dialog shows the error
        }}
      />
    </div>
  );
}

/** Workspace page: archived projects (admins), each opening its archived state. */
export function ArchivedProjectsList({ ws, enabled }: { ws: string; enabled: boolean }) {
  const list = useArchivedProjects(ws, enabled);
  if (!enabled || !list.data || list.data.length === 0) return null;
  return (
    <section aria-labelledby="archived-projects" data-testid="archived-projects" className="mt-8 space-y-2">
      <h2 id="archived-projects" className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">{tlc.project.archivedList}</h2>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{tlc.project.archivedListHint}</p>
      <ul className="divide-y divide-zinc-100 rounded-xl bg-white ring-1 ring-zinc-200 dark:divide-zinc-800 dark:bg-zinc-900 dark:ring-zinc-800">
        {list.data.map((p) => {
          const date = archivedDate(p.archived_at);
          return (
            <li key={p.id}>
              <Link
                to={`/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(p.key)}`}
                className="flex flex-wrap items-baseline gap-x-2 px-4 py-3 text-sm hover:bg-zinc-50 pointer-coarse:min-h-11 dark:hover:bg-zinc-800"
              >
                <span className="font-mono text-xs text-zinc-600 dark:text-zinc-400">{p.key}</span>
                <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">{p.name}</span>
                {date && <span className="text-xs text-zinc-500 dark:text-zinc-400">{tlc.archivedOn(formatDate(date))}</span>}
              </Link>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
