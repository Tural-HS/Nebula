import { useEffect, useRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";

import { ApiError } from "../api/client";
import type { StatusCategory } from "../api/types";
import { initials } from "../lib/board";
import { cn } from "../lib/cn";
import { formatDate } from "../lib/format";
import { t } from "../i18n";


type Variant = "primary" | "secondary" | "ghost" | "danger";

const variants: Record<Variant, string> = {
  primary: "bg-brand-600 text-white hover:bg-brand-700 disabled:bg-brand-600/50",
  secondary:
    "bg-white text-zinc-800 ring-1 ring-zinc-300 hover:bg-zinc-50 dark:bg-zinc-900 dark:text-zinc-100 dark:ring-zinc-700 dark:hover:bg-zinc-800",
  ghost: "text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800",
  danger: "bg-red-600 text-white hover:bg-red-700",
};

export function Button({ variant = "secondary", className, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return (
    <button
      type="button"
      className={cn(
        "inline-flex items-center justify-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium whitespace-nowrap transition disabled:cursor-not-allowed",
        variants[variant],
        className,
      )}
      {...props}
    />
  );
}

const fieldBase =
  "rounded-md border-0 bg-white px-2.5 py-1.5 text-sm text-zinc-900 ring-1 ring-zinc-300 placeholder:text-zinc-400 focus:ring-2 focus:ring-brand-500 dark:bg-zinc-900 dark:text-zinc-100 dark:ring-zinc-700";

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
    <div role="status" className="flex items-center gap-2 p-6 text-sm text-zinc-500">
      <span className="size-4 animate-spin rounded-full border-2 border-zinc-300 border-t-brand-600" aria-hidden />
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
  return <div className="p-10 text-center text-sm text-zinc-500">{children}</div>;
}

const categoryStyle: Record<StatusCategory, string> = {
  todo: "bg-zinc-100 text-zinc-700 ring-zinc-300 dark:bg-zinc-800 dark:text-zinc-300 dark:ring-zinc-700",
  in_progress: "bg-blue-50 text-blue-700 ring-blue-200 dark:bg-blue-950 dark:text-blue-300 dark:ring-blue-900",
  done: "bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950 dark:text-emerald-300 dark:ring-emerald-900",
  canceled: "bg-zinc-50 text-zinc-500 line-through ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800",
};

export function StatusBadge({ name, category }: { name: string; category: StatusCategory }) {
  return (
    <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset", categoryStyle[category])}>
      {name}
    </span>
  );
}

/** A due date; red with a clock when the item is overdue. */
export function DueChip({ date, overdue }: { date: string; overdue: boolean }) {
  const label = formatDate(date);
  return (
    <span
      aria-label={`${t.dueDate}: ${label}${overdue ? ` (${t.overdue})` : ""}`}
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[11px] whitespace-nowrap tabular-nums",
        overdue ? "bg-red-50 font-medium text-red-700 ring-1 ring-red-200 dark:bg-red-950 dark:text-red-300 dark:ring-red-900" : "text-zinc-500 dark:text-zinc-400",
      )}
    >
      <span aria-hidden>{overdue ? "⏰" : "📅"}</span>
      {label}
    </span>
  );
}

export function Avatar({ name, size = "sm" }: { name: string; size?: "sm" | "md" }) {
  return (
    <span
      title={name}
      aria-label={name}
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-full bg-brand-100 font-semibold text-brand-700 dark:bg-brand-700 dark:text-brand-50",
        size === "sm" ? "size-6 text-[10px]" : "size-8 text-xs",
      )}
    >
      {initials(name)}
    </span>
  );
}

/** Accessible modal built on the native <dialog> element (focus trap, Esc). */
export function Dialog({ open, onClose, title, children }: { open: boolean; onClose: () => void; title: string; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      aria-labelledby="dialog-title"
      className="m-auto w-full max-w-md rounded-xl bg-white p-0 text-zinc-900 shadow-2xl backdrop:bg-zinc-950/40 dark:bg-zinc-900 dark:text-zinc-100"
    >
      <div className="p-5">
        <h2 id="dialog-title" className="mb-4 text-base font-semibold">
          {title}
        </h2>
        {open && children}
      </div>
    </dialog>
  );
}

export function FieldError({ error, field }: { error: unknown; field?: string }) {
  if (!(error instanceof ApiError)) return error ? <p className="mt-2 text-sm text-red-600">{t.errorGeneric}</p> : null;
  if (field && error.field !== field) return null;
  return (
    <p role="alert" className="mt-2 text-sm text-red-600 dark:text-red-400">
      {error.field ? `${error.field}: ` : ""}
      {error.message}
    </p>
  );
}
