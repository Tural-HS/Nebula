import type { ReactNode } from "react";
import { Link, useParams } from "react-router";

import { useAuth } from "../auth/context";
import { useMyRequestsUnseen } from "../api/forms";
import { useMe } from "../api/hooks";
import { t } from "../i18n";
import { tab } from "../i18n-absences";
import { tf } from "../i18n-forms";
import { cn } from "../lib/cn";
import { GlobalSearch } from "./GlobalSearch";
import { AbsenceBanner } from "./Absence";
import { ArchivedWorkspaceBanner } from "./Lifecycle";
import { LanguageMenu } from "./LanguageMenu";
import { NotificationBell } from "./NotificationBell";
import { ThemeMenu } from "./ThemeMenu";
import { Popover } from "../pages/ViewTools";
import { Avatar, Button } from "./ui";

export interface Crumb {
  label: string;
  to?: string;
}

export function Layout({ crumbs, actions, children }: { crumbs: Crumb[]; actions?: ReactNode; children: ReactNode }) {
  const { logout } = useAuth();
  const me = useMe();
  const unseen = useMyRequestsUnseen().data ?? 0;
  // Absences are per workspace (ADR-0042): the menu entry exists inside one.
  const { ws = "" } = useParams();
  const absenceLink = ws ? `/w/${encodeURIComponent(ws)}/absence` : "";
  return (
    <div className="flex h-full flex-col">
      {/* Phones (ADR-0019): no app name, avatar or upper crumbs, so the
          header fits 375 px; safe-area padding clears notches in landscape. */}
      <header className="flex min-h-12 shrink-0 items-center gap-2 border-b max-sm:flex-wrap max-sm:gap-y-0 border-zinc-200 bg-white pt-[env(safe-area-inset-top)] pr-[max(0.75rem,env(safe-area-inset-right))] pl-[max(0.75rem,env(safe-area-inset-left))] sm:gap-3 sm:pr-[max(1rem,env(safe-area-inset-right))] sm:pl-[max(1rem,env(safe-area-inset-left))] dark:border-zinc-800 dark:bg-zinc-900">
        <Link to="/" aria-label={t.appName} className="flex shrink-0 items-center gap-2 font-semibold text-brand-900 dark:text-zinc-50">
          <img src="/logo-mark.png" alt="" className="size-7" />
          <span className="hidden sm:inline">{t.appName}</span>
        </Link>
        {/* Phones: the crumbs take their own line under the buttons, so the page
            name is never squeezed to nothing (320 px, long translations). */}
        <nav aria-label="breadcrumb" className="flex min-w-0 items-center gap-1.5 text-sm text-zinc-500 max-sm:order-last max-sm:basis-full max-sm:pb-2 dark:text-zinc-400">
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
        <div className="ml-auto flex shrink-0 items-center gap-2 max-sm:gap-1">
          {actions}
          <GlobalSearch />
          {/* Requesters follow their intake-form submissions here (ADR-0032).
              Labels (this one and the user's name) only from xl: below that the
              header with project actions overflows a 1024 px tablet. */}
          <Link
            to="/requests"
            aria-label={unseen > 0 ? `${tf.myRequests}: ${tf.unseen(unseen)}` : tf.myRequests}
            className="relative inline-flex items-center gap-1 rounded-md px-2 py-1.5 text-sm text-zinc-700 hover:bg-zinc-100 max-sm:hidden pointer-coarse:min-h-11 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            <span aria-hidden>📨</span>
            <span className="hidden xl:inline">{tf.myRequests}</span>
            {unseen > 0 && (
              <span aria-hidden className="absolute -top-0.5 -right-0.5 min-w-4 rounded-full bg-brand-600 px-1 text-center text-2xs leading-4 font-semibold text-white">
                {unseen > 99 ? "99+" : unseen}
              </span>
            )}
          </Link>
          <NotificationBell />
          <LanguageMenu className="max-sm:hidden" />
          {/* Only from xl: a 1024 px tablet header is full (ADR-0019); smaller screens pick it on the workspace list. */}
          <ThemeMenu className="max-xl:hidden" />
          {me.data && absenceLink && (
            <span className="max-sm:hidden">
              <Popover
                align="right"
                summaryLabel={`${t.account}: ${me.data.display_name}`}
                label={
                  <>
                    <Avatar name={me.data.display_name} />
                    <span className="hidden whitespace-nowrap xl:inline">{me.data.display_name}</span>
                  </>
                }
              >
                <div className="flex flex-col gap-2">
                  <p className="font-medium [overflow-wrap:anywhere]">{me.data.display_name}</p>
                  <Link to={absenceLink} className="inline-flex items-center gap-2 rounded-md py-1.5 hover:underline">
                    <span aria-hidden>🌴</span>
                    {tab.nav}
                  </Link>
                </div>
              </Popover>
            </span>
          )}
          {me.data && !absenceLink && (
            <span className="flex items-center gap-2 text-sm text-zinc-600 max-sm:hidden dark:text-zinc-400">
              <Avatar name={me.data.display_name} />
              <span className="hidden whitespace-nowrap xl:inline">{me.data.display_name}</span>
            </span>
          )}
          <Button variant="ghost" className="max-sm:hidden" onClick={() => void logout()}>
            {t.signOut}
          </Button>
          {/* Phones (320 px): requests, language, theme and sign-out share one
              account menu, so the header keeps the page title visible. */}
          <span className="sm:hidden">
            <Popover
              align="right"
              summaryLabel={unseen > 0 ? `${t.account}: ${tf.unseen(unseen)}` : t.account}
              label={me.data ? <Avatar name={me.data.display_name} /> : <span aria-hidden>👤</span>}
            >
              <div className="flex flex-col gap-3">
                {me.data && <p className="font-medium [overflow-wrap:anywhere]">{me.data.display_name}</p>}
                <Link to="/requests" className="inline-flex items-center gap-2 rounded-md py-1.5 hover:underline pointer-coarse:min-h-11">
                  <span aria-hidden>📨</span>
                  {tf.myRequests}
                  {unseen > 0 && <span className="rounded-full bg-brand-600 px-1.5 text-2xs font-semibold text-white">{tf.unseen(unseen)}</span>}
                </Link>
                {absenceLink && (
                  <Link to={absenceLink} className="inline-flex items-center gap-2 rounded-md py-1.5 hover:underline pointer-coarse:min-h-11">
                    <span aria-hidden>🌴</span>
                    {tab.nav}
                  </Link>
                )}
                <LanguageMenu />
                <ThemeMenu />
                <Button className="self-start" onClick={() => void logout()}>
                  {t.signOut}
                </Button>
              </div>
            </Popover>
          </span>
        </div>
      </header>
      {/* The logo's navy → blue → teal gradient as a hairline under the header (ADR-0047); follows the workspace accent. */}
      <div aria-hidden className="h-0.5 shrink-0 bg-linear-to-r from-brand-900 via-brand-600 to-brand-400" />
      <ArchivedWorkspaceBanner />
      <AbsenceBanner />
      <main className="min-h-0 flex-1">{children}</main>
    </div>
  );
}
