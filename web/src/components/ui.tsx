import { useEffect, useId, useRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";

import { ApiError } from "../api/client";
import type { StatusCategory } from "../api/types";
import { initials } from "../lib/board";
import { cn } from "../lib/cn";
import { formatDate } from "../lib/format";
import { daysLate, LATE_CHIP, lateLevel, localToday } from "../lib/overdue";
import { t } from "../i18n";


type Variant = "primary" | "secondary" | "ghost" | "danger" | "dangerGhost";

const variants: Record<Variant, string> = {
  primary: "bg-brand-600 text-white hover:bg-brand-700 disabled:bg-brand-600/50",
  secondary:
    "bg-white text-zinc-800 ring-1 ring-zinc-300 hover:bg-zinc-50 dark:bg-zinc-900 dark:text-zinc-100 dark:ring-zinc-700 dark:hover:bg-zinc-800",
  ghost: "text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800",
  danger: "bg-red-600 text-white hover:bg-red-700",
  // A destructive action that is not the primary one (e.g. Delete in a panel
  // header): red text in both themes. cn() does not merge classes, so a
  // ghost button with an extra text colour lost to dark:text-zinc-300.
  dangerGhost: "text-red-700 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950",
};

export function Button({ variant = "secondary", className, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return (
    <button
      type="button"
      className={cn(
        // pointer-coarse: 44 px tap targets on touch screens, compact for mice.
        "inline-flex items-center justify-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium whitespace-nowrap transition-colors disabled:cursor-not-allowed pointer-coarse:min-h-11",
        variants[variant],
        className,
      )}
      {...props}
    />
  );
}

// On touch screens fields are 44 px tall and use 16 px text: iOS Safari
// zooms the page into any focused field with smaller text.
export const touchField = "pointer-coarse:min-h-11 pointer-coarse:text-base";

const fieldBase =
  touchField +
  " rounded-md border-0 bg-white px-2.5 py-1.5 text-sm text-zinc-900 ring-1 ring-zinc-500 placeholder:text-zinc-500 focus:ring-2 focus:ring-brand-500 dark:bg-zinc-900 dark:text-zinc-100 dark:placeholder:text-zinc-400";

// Full width unless the caller sets an explicit width (w-*), which would
// otherwise lose to w-full depending on stylesheet order.
const width = (className?: string) => (className && /(^|\s)(sm:|md:|lg:)?w-/.test(className) ? "" : "w-full");

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(fieldBase, width(className), className)} {...props} />;
}

export function Select({ className, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cn(fieldBase, width(className), "pr-8", className)} {...props} />;
}

export function Label({ children, htmlFor }: { children: ReactNode; htmlFor: string }) {
  return (
    <label htmlFor={htmlFor} className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
      {children}
    </label>
  );
}

export function Spinner({ label = t.loading }: { label?: string }) {
  return (
    <div role="status" className="flex items-center gap-2 p-6 text-sm text-zinc-500 dark:text-zinc-400">
      <span className="size-4 animate-spin rounded-full motion-reduce:animate-none border-2 border-zinc-300 border-t-brand-600" aria-hidden />
      {label}
    </div>
  );
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const msg = error instanceof ApiError ? (error.status === 404 ? t.notFound : error.message) : t.errorGeneric;
  return (
    <div role="alert" className="m-6 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-800 dark:border-red-900 dark:bg-red-950 dark:text-red-200">
      <p>{msg}</p>
      {onRetry && (
        <Button className="mt-3" onClick={onRetry}>
          {t.retry}
        </Button>
      )}
    </div>
  );
}

export function EmptyState({ children }: { children: ReactNode }) {
  return <div className="p-10 text-center text-sm text-zinc-500 dark:text-zinc-400">{children}</div>;
}

const categoryStyle: Record<StatusCategory, string> = {
  todo: "bg-zinc-100 text-zinc-700 ring-zinc-300 dark:bg-zinc-800 dark:text-zinc-300 dark:ring-zinc-700",
  in_progress: "bg-blue-50 text-blue-700 ring-blue-200 dark:bg-blue-950 dark:text-blue-300 dark:ring-blue-900",
  done: "bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950 dark:text-emerald-300 dark:ring-emerald-900",
  canceled: "bg-zinc-50 text-zinc-500 dark:text-zinc-400 line-through ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800",
};

export function StatusBadge({ name, category }: { name: string; category: StatusCategory }) {
  return (
    <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset", categoryStyle[category])}>
      {name}
    </span>
  );
}

/**
 * A due date. When the item is overdue it shows the days late and takes the
 * band's colour (ADR-0057): yellow up to 10 days, orange up to 20, then red.
 */
export function DueChip({ date, overdue, today = localToday() }: { date: string; overdue: boolean; today?: string }) {
  const label = formatDate(date);
  const late = overdue ? Math.max(1, daysLate(date, today)) : 0;
  const level = lateLevel(late);
  return (
    // role=img: aria-label is not allowed on a plain span (screen readers skip it).
    <span
      role="img"
      aria-label={`${t.dueDate}: ${label}${late ? ` (${t.lateBy(late)})` : ""}`}
      title={late ? t.lateBy(late) : undefined}
      data-late={level || undefined}
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-2xs whitespace-nowrap tabular-nums",
        level ? LATE_CHIP[level] : "text-zinc-500 dark:text-zinc-400",
      )}
    >
      <span aria-hidden>{late ? "⏰" : "📅"}</span>
      {label}
      {late > 0 && <span aria-hidden>· {late}</span>}
    </span>
  );
}

export function Avatar({ name, size = "sm" }: { name: string; size?: "sm" | "md" }) {
  return (
    <span
      title={name}
      role="img"
      aria-label={name}
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-full bg-brand-100 font-semibold text-brand-700 dark:bg-brand-700 dark:text-brand-50",
        size === "sm" ? "size-6 text-2xs" : "size-8 text-xs",
      )}
    >
      {initials(name)}
    </span>
  );
}

/**
 * Accessible modal built on the native <dialog> element (focus trap, Esc).
 * It never grows past the screen: long content scrolls inside it. A "sheet"
 * slides up from the bottom on phones (filters, options) and has its own
 * close button; on wider screens it is an ordinary centred dialog.
 */
export function Dialog({ open, onClose, title, children, variant = "dialog" }: {
  open: boolean; onClose: () => void; title: string; children: ReactNode; variant?: "dialog" | "sheet";
}) {
  const ref = useRef<HTMLDialogElement>(null);
  // Unique per instance: dialogs stay mounted while closed, so a fixed id
  // made every dialog on a page take the first one's title as its name.
  const titleId = useId();
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);
  const sheet = variant === "sheet";
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      aria-labelledby={titleId}
      className={cn(
        "m-auto w-[calc(100%-2rem)] max-w-md overflow-y-auto overscroll-contain rounded-xl bg-white p-0 text-zinc-900 shadow-2xl backdrop:bg-zinc-950/40 dark:bg-zinc-900 dark:text-zinc-100",
        "max-h-[calc(100dvh-2rem)]",
        sheet && "max-md:mb-0 max-md:w-full max-md:max-w-none max-md:rounded-b-none max-md:max-h-[85dvh]",
      )}
    >
      <div className={cn("p-5", sheet && "pb-[max(1.25rem,env(safe-area-inset-bottom))]")}>
        <div className="mb-4 flex items-center justify-between gap-2">
          <h2 id={titleId} className="text-base font-semibold">
            {title}
          </h2>
          {sheet && (
            <Button variant="ghost" className="-my-1.5 -mr-2" onClick={onClose}>
              {t.close}
            </Button>
          )}
        </div>
        {open && children}
      </div>
    </dialog>
  );
}

export function FieldError({ error, field }: { error: unknown; field?: string }) {
  if (!(error instanceof ApiError)) return error ? <p role="alert" className="mt-2 text-sm text-red-600 dark:text-red-400">{t.errorGeneric}</p> : null;
  if (field && error.field !== field) return null;
  return (
    <p role="alert" className="mt-2 text-sm text-red-600 dark:text-red-400">
      {error.field ? `${error.field}: ` : ""}
      {error.message}
    </p>
  );
}
