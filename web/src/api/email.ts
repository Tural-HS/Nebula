// Workspace SMTP settings (ADR-0038): admin-only.
import { useQuery } from "@tanstack/react-query";

import { useAdminMutation } from "./admin";
import { api, path } from "./client";
import type { EmailSettingsInput, EmailSettingsView } from "../lib/emailSettings";

export interface EmailTestResult {
  ok: boolean;
  source: "workspace" | "instance";
  to: string;
  /** Sanitized server reason when ok is false. */
  error?: string;
}

const key = (ws: string) => ["ws", ws, "email-settings"] as const;
const base = (ws: string) => path`/workspaces/${ws}` + "/email-settings";

export const useEmailSettings = (ws: string) =>
  useQuery({ queryKey: key(ws), queryFn: () => api<EmailSettingsView>("GET", base(ws)) });

export function useEmailApi(ws: string) {
  return {
    save: useAdminMutation((b: EmailSettingsInput) => api<EmailSettingsView>("PUT", base(ws), b), [key(ws)]),
    remove: useAdminMutation(() => api<undefined>("DELETE", base(ws)), [key(ws)]),
    test: useAdminMutation((to: string) => api<EmailTestResult>("POST", base(ws) + "/test", to ? { to } : {}), []),
  };
}
