import { createContext, use } from "react";

export type Kind = "success" | "error";

/** An optional button in a success toast, e.g. "Geri qaytar" after a one-click edit. */
export interface ToastAction {
  label: string;
  run: () => void;
}

export const ToastContext = createContext<(kind: Kind, text: string, action?: ToastAction) => void>(() => undefined);

export function useToast() {
  return use(ToastContext);
}
