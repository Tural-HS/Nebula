import type { ReactNode } from "react";
import { Link } from "react-router";

import { useAuth } from "../auth/context";
import { useMe } from "../api/hooks";
import { t } from "../i18n";
import { Avatar, Button } from "./ui";

export interface Crumb {
  label: string;
  to?: string;
}

export function Layout({ crumbs, actions, children }: { crumbs: Crumb[]; actions?: ReactNode; children: ReactNode }) {
  const { logout } = useAuth();
  const me = useMe();
  return (
    <div className="flex h-full flex-col">
      <header className="flex h-12 shrink-0 items-center gap-3 border-b border-zinc-200 bg-white px-4 dark:border-zinc-800 dark:bg-zinc-900">
        <Link to="/" className="flex items-center gap-2 font-semibold text-zinc-900 dark:text-zinc-50">
          <img src="/favicon.svg" alt="" className="size-6" />
          {t.appName}
        </Link>
        <nav aria-label="breadcrumb" className="flex min-w-0 items-center gap-1.5 text-sm text-zinc-500">
          {crumbs.map((c, i) => (
            <span key={`${c.label}-${String(i)}`} className="flex min-w-0 items-center gap-1.5">
              <span aria-hidden>/</span>
              {c.to ? (
                <Link to={c.to} className="truncate hover:text-zinc-900 dark:hover:text-zinc-100">
                  {c.label}
                </Link>
              ) : (
                <span className="truncate font-medium text-zinc-900 dark:text-zinc-100" aria-current="page">
                  {c.label}
                </span>
              )}
            </span>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-2">
          {actions}
          {me.data && (
            <span className="flex items-center gap-2 text-sm text-zinc-600 dark:text-zinc-400">
              <Avatar name={me.data.display_name} />
              <span className="hidden whitespace-nowrap lg:inline">{me.data.display_name}</span>
            </span>
          )}
          <Button variant="ghost" onClick={() => void logout()}>
            {t.signOut}
          </Button>
        </div>
      </header>
      <main className="min-h-0 flex-1">{children}</main>
    </div>
  );
}
