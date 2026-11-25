import { createContext, use } from "react";

/** A confirmation (ADR-0050): the question, the action's name and whether it is destructive. */
export interface ConfirmOptions {
  title: string;
  body?: string;
  action: string;
  danger?: boolean;
}

/** Resolves true when the action button was chosen; Cancel, Esc and closing resolve false. */
export const ConfirmContext = createContext<(o: ConfirmOptions) => Promise<boolean>>(() => Promise.resolve(false));

export function useConfirm() {
  return use(ConfirmContext);
}
