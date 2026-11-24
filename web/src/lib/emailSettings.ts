// Pure logic of the workspace SMTP settings form (ADR-0038), kept out of the
// component so it can be unit tested.

export type TLSMode = "starttls" | "tls" | "none";

export interface EmailSettings {
  host: string;
  port: number;
  tls_mode: TLSMode;
  username: string;
  password_set: boolean;
  from_address: string;
  reply_to: string | null;
  enabled: boolean;
  updated_by: string | null;
  updated_by_name: string | null;
  updated_at: string;
}

export interface EmailPolicy {
  allow_plaintext: boolean;
  allow_private: boolean;
  ports: number[];
}

export interface EmailSettingsView {
  /** null: the workspace uses the server's (instance) configuration. */
  settings: EmailSettings | null;
  instance_configured: boolean;
  policy: EmailPolicy;
}

export interface EmailForm {
  host: string;
  port: string;
  tls_mode: TLSMode;
  username: string;
  from_address: string;
  reply_to: string;
  enabled: boolean;
  /** "keep": leave the stored password; "set": replace it with `password`; "clear": remove it. */
  passwordAction: "keep" | "set" | "clear";
  password: string;
}

/** The PUT body; `password` is omitted to keep, null to clear (never echoed back). */
export interface EmailSettingsInput {
  host: string;
  port: number;
  tls_mode: TLSMode;
  username: string;
  from_address: string;
  reply_to: string | null;
  enabled: boolean;
  password?: string | null;
}

export type PresetId = "google" | "microsoft" | "cpanel";

/**
 * Quick-fill values, checked against the providers' documentation:
 * Google: smtp.gmail.com, 587 STARTTLS (465 implicit TLS), app password.
 * Microsoft 365: smtp.office365.com, 587 STARTTLS, SMTP AUTH enabled on the mailbox.
 * cPanel: mail.<domain>, 465 SSL/TLS ("Secure SSL/TLS Settings").
 */
export const PRESETS: Record<PresetId, { host: string; port: number; tls_mode: TLSMode }> = {
  google: { host: "smtp.gmail.com", port: 587, tls_mode: "starttls" },
  microsoft: { host: "smtp.office365.com", port: 587, tls_mode: "starttls" },
  cpanel: { host: "mail.", port: 465, tls_mode: "tls" },
};

/** Domain of the address in "Name <a@b.example>" or "a@b.example" ("" if none). */
export function addressDomain(from: string): string {
  const m = /@([A-Za-z0-9.-]+)>?\s*$/.exec(from.trim());
  return m?.[1]?.toLowerCase() ?? "";
}

/** Applies a preset; cPanel's host is mail.<domain of the From address>. */
export function applyPreset(form: EmailForm, id: PresetId): EmailForm {
  const p = PRESETS[id];
  let host = p.host;
  if (id === "cpanel") {
    const d = addressDomain(form.from_address);
    host = d ? `mail.${d}` : "";
  }
  return { ...form, host, port: String(p.port), tls_mode: p.tls_mode };
}

export function initialForm(s: EmailSettings | null, policy: EmailPolicy): EmailForm {
  if (!s) {
    return {
      host: "",
      port: policy.ports.includes(587) ? "587" : String(policy.ports[0] ?? 587),
      tls_mode: "starttls",
      username: "",
      from_address: "",
      reply_to: "",
      enabled: true,
      passwordAction: "set",
      password: "",
    };
  }
  return {
    host: s.host,
    port: String(s.port),
    tls_mode: s.tls_mode,
    username: s.username,
    from_address: s.from_address,
    reply_to: s.reply_to ?? "",
    enabled: s.enabled,
    passwordAction: s.password_set ? "keep" : "set",
    password: "",
  };
}

/** Builds the PUT body. An empty new password means "no password" only for a
 *  setting that had none; when one is stored, "keep" omits the field. */
export function toInput(f: EmailForm, hadPassword: boolean): EmailSettingsInput {
  const body: EmailSettingsInput = {
    host: f.host.trim(),
    port: Number(f.port),
    tls_mode: f.tls_mode,
    username: f.username.trim(),
    from_address: f.from_address.trim(),
    reply_to: f.reply_to.trim() === "" ? null : f.reply_to.trim(),
    enabled: f.enabled,
  };
  if (f.passwordAction === "clear") {
    if (hadPassword) body.password = null;
  } else if (f.passwordAction === "set") {
    if (f.password !== "") body.password = f.password;
    else if (hadPassword) body.password = null;
  }
  return body;
}

export type FormProblem = "port" | "tls_mode" | "credentials_without_tls";

/** Client-side checks mirroring the server, for early feedback only. */
export function formProblems(f: EmailForm, policy: EmailPolicy, hadPassword: boolean): FormProblem[] {
  const out: FormProblem[] = [];
  const port = Number(f.port);
  if (!Number.isInteger(port) || !policy.ports.includes(port)) out.push("port");
  if (f.tls_mode === "none") {
    if (!policy.allow_plaintext) out.push("tls_mode");
    const willHavePassword = f.passwordAction === "set" ? f.password !== "" : f.passwordAction === "keep" && hadPassword;
    if (f.username.trim() !== "" || willHavePassword) out.push("credentials_without_tls");
  }
  return out;
}
