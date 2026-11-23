import type { ReactNode } from "react";
import { Link } from "react-router";

import { useAuth } from "../auth/context";
import { useMe } from "../api/hooks";
import { t } from "../i18n";
import { cn } from "../lib/cn";
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
      {/* Phones (ADR-0019): no app name, avatar or upper crumbs, so the
          header fits 375 px; safe-area padding clears notches in landscape. */}
      <header className="flex min-h-12 shrink-0 items-center gap-2 border-b border-zinc-200 bg-white pt-[env(safe-area-inset-top)] pr-[max(0.75rem,env(safe-area-inset-right))] pl-[max(0.75rem,env(safe-area-inset-left))] sm:gap-3 sm:pr-[max(1rem,env(safe-area-inset-right))] sm:pl-[max(1rem,env(safe-area-inset-left))] dark:border-zinc-800 dark:bg-zinc-900">
        <Link to="/" aria-label={t.appName} className="flex shrink-0 items-center gap-2 font-semibold text-brand-900 dark:text-zinc-50">
          <img src="/logo-mark.png" alt="" className="size-7" />
          <span className="hidden sm:inline">{t.appName}</span>
        </Link>
        <nav aria-label="breadcrumb" className="flex min-w-0 items-center gap-1.5 text-sm text-zinc-500">
          {crumbs.map((c, i) => (
            <span key={`${c.label}-${String(i)}`} className={cn("flex min-w-0 items-center gap-1.5", i < crumbs.length - 2 && "max-sm:hidden")}>
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
        <div className="ml-auto flex shrink-0 items-center gap-2">
          {actions}
          {me.data && (
            <span className="flex items-center gap-2 text-sm text-zinc-600 max-sm:hidden dark:text-zinc-400">
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
