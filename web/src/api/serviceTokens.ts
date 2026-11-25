// Workspace service tokens (ADR-0043): admin-only.
import { useQuery } from "@tanstack/react-query";

import { useAdminMutation } from "./admin";
import { api, path } from "./client";
import type { CreatedToken, ServiceToken, TokenInput } from "../lib/serviceTokens";

const key = (ws: string) => ["ws", ws, "service-tokens"] as const;
const base = (ws: string) => path`/workspaces/${ws}` + "/service-tokens";

export const useServiceTokens = (ws: string) =>
  useQuery({ queryKey: key(ws), queryFn: () => api<{ tokens: ServiceToken[] }>("GET", base(ws)).then((r) => r.tokens) });

export function useServiceTokenApi(ws: string) {
  return {
    create: useAdminMutation((b: TokenInput) => api<CreatedToken>("POST", base(ws), b), [key(ws)]),
    revoke: useAdminMutation((id: string) => api<undefined>("DELETE", base(ws) + path`/${id}`), [key(ws)]),
  };
}
