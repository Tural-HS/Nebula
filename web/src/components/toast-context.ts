import { createContext, use } from "react";

export type Kind = "success" | "error";

export const ToastContext = createContext<(kind: Kind, text: string) => void>(() => undefined);

export function useToast() {
  return use(ToastContext);
}
