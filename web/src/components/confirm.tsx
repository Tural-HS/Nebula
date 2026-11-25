import { useCallback, useRef, useState, type ReactNode } from "react";

import { t } from "../i18n";
import { ConfirmContext, type ConfirmOptions } from "./confirm-context";
import { Button, Dialog } from "./ui";

/**
 * One app-wide confirmation dialog in place of window.confirm (ADR-0050): it
 * names the object, follows the theme, keeps focus inside, and Esc cancels.
 * The action button is red for destructive actions.
 */
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<ConfirmOptions | null>(null);
  const resolver = useRef<((ok: boolean) => void) | null>(null);
  const confirm = useCallback((o: ConfirmOptions) => {
    resolver.current?.(false); // a second question replaces the first
    setPending(o);
    return new Promise<boolean>((resolve) => { resolver.current = resolve; });
  }, []);
  const finish = (ok: boolean) => {
    resolver.current?.(ok);
    resolver.current = null;
    setPending(null);
  };
  return (
    <ConfirmContext value={confirm}>
      {children}
      <Dialog open={pending !== null} onClose={() => { finish(false); }} title={pending?.title ?? ""}>
        {pending?.body && <p className="text-sm text-zinc-700 dark:text-zinc-300">{pending.body}</p>}
        <div className="mt-5 flex justify-end gap-2">
          <Button onClick={() => { finish(false); }}>{t.cancel}</Button>
          <Button variant={pending?.danger ? "danger" : "primary"} onClick={() => { finish(true); }}>
            {pending?.action}
          </Button>
        </div>
      </Dialog>
    </ConfirmContext>
  );
}
