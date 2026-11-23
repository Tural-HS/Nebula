import type { User } from "oidc-client-ts";
import { createContext, use } from "react";

export interface AuthState {
  user: User | null;
  login: (returnTo?: string) => Promise<void>;
  logout: () => Promise<void>;
}

export const AuthContext = createContext<AuthState | null>(null);

export function useAuth(): AuthState {
  const ctx = use(AuthContext);
  if (!ctx) throw new Error("useAuth outside AuthProvider");
  return ctx;
}
