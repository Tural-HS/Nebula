// Runtime configuration, loaded from /config.json so one build runs in every
// environment (the file is mounted per environment instead of rebuilding).
export interface AppConfig {
  oidcAuthority: string;
  oidcClientId: string;
}

function isConfig(v: unknown): v is AppConfig {
  if (typeof v !== "object" || v === null) return false;
  const c = v as Record<string, unknown>;
  return typeof c.oidcAuthority === "string" && typeof c.oidcClientId === "string";
}

export async function loadConfig(): Promise<AppConfig> {
  const res = await fetch("/config.json", { cache: "no-store" });
  if (!res.ok) throw new Error(`config.json: HTTP ${res.status}`);
  const data: unknown = await res.json();
  if (!isConfig(data)) throw new Error("config.json: missing oidcAuthority or oidcClientId");
  return data;
}
