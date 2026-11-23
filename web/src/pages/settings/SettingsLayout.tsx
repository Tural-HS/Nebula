import type { ReactNode } from "react";
import { NavLink, useParams } from "react-router";

import { useWorkspace } from "../../api/hooks";
import { roleAtLeast } from "../../api/types";
import { Layout } from "../../components/Layout";
import { ErrorState, Spinner } from "../../components/ui";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";
import { cn } from "../../lib/cn";

const sections = [
  ["general", ta.nav.general],
  ["members", ta.nav.members],
  ["fields", ta.nav.fields],
  ["item-types", ta.nav.itemTypes],
  ["workflows", ta.nav.workflows],
  ["webhooks", ta.nav.webhooks],
  ["automations", ta.nav.automations],
  ["audit", ta.nav.audit],
] as const;

/**
 * Shell for workspace settings. Everything except the member list is
 * admin-only; the API enforces this too, the UI just avoids dead ends.
 */
export function SettingsLayout({ title, adminOnly = true, children }: { title: string; adminOnly?: boolean; children: (ws: string) => ReactNode }) {
  const { ws = "" } = useParams();
  const workspace = useWorkspace(ws);
  const isAdmin = roleAtLeast(workspace.data?.role, "admin");
  const base = `/w/${encodeURIComponent(ws)}/settings/`;

  return (
    <Layout
      crumbs={[
        { label: t.workspaces, to: "/" },
        { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` },
        { label: ta.settings },
      ]}
    >
      <div className="mx-auto flex max-w-6xl flex-col gap-6 p-4 md:flex-row md:p-6">
        <nav aria-label={ta.settings} className="flex shrink-0 gap-1 overflow-x-auto md:w-48 md:flex-col">
          {sections.map(([slug, label]) => (
            <NavLink
              key={slug}
              to={base + slug}
              className={({ isActive }) =>
                cn(
                  "rounded-md px-3 py-1.5 text-sm whitespace-nowrap pointer-coarse:py-3",
                  isActive ? "bg-zinc-200 font-medium dark:bg-zinc-800" : "text-zinc-600 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-900",
                )
              }
            >
              {label}
            </NavLink>
          ))}
        </nav>
        <section className="min-w-0 flex-1">
          <h1 className="mb-4 text-lg font-semibold">{title}</h1>
          {workspace.isPending ? (
            <Spinner />
          ) : workspace.isError ? (
            <ErrorState error={workspace.error} />
          ) : adminOnly && !isAdmin ? (
            <p className="rounded-lg bg-zinc-100 p-4 text-sm text-zinc-600 dark:bg-zinc-900 dark:text-zinc-400">{ta.adminOnly}</p>
          ) : (
            children(ws)
          )}
        </section>
      </div>
    </Layout>
  );
}

/** A bordered card used by every settings screen. With a label it becomes a
 *  named region, so assistive tech (and tests) can address it directly. */
export function Card({ children, className, label }: { children: ReactNode; className?: string; label?: string }) {
  const cls = cn("rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800", className);
  return label ? <section aria-label={label} className={cls}>{children}</section> : <div className={cls}>{children}</div>;
}
