import { describe, expect, it } from "vitest";

import { addressDomain, applyPreset, formProblems, initialForm, PRESETS, toInput, type EmailPolicy, type EmailSettings } from "./emailSettings";

const prod: EmailPolicy = { allow_plaintext: false, allow_private: false, ports: [25, 465, 587, 2525] };
const dev: EmailPolicy = { allow_plaintext: true, allow_private: true, ports: [25, 465, 587, 1025, 2525] };

const saved: EmailSettings = {
  host: "mail.example.com",
  port: 465,
  tls_mode: "tls",
  username: "finance@example.com",
  password_set: true,
  from_address: "Finance <finance@example.com>",
  reply_to: null,
  enabled: true,
  updated_by: null,
  updated_by_name: "Alice",
  updated_at: "2026-11-24T10:00:00Z",
};

describe("toInput: the password is write-only", () => {
  it("omits a kept password", () => {
    const body = toInput(initialForm(saved, prod), true);
    expect("password" in body).toBe(false);
    expect(body).toMatchObject({ host: "mail.example.com", port: 465, tls_mode: "tls", reply_to: null, enabled: true });
  });
  it("sends a replaced password", () => {
    const f = { ...initialForm(saved, prod), passwordAction: "set" as const, password: "new pass" };
    expect(toInput(f, true).password).toBe("new pass");
  });
  it("sends null to clear a stored password", () => {
    expect(toInput({ ...initialForm(saved, prod), passwordAction: "clear" }, true).password).toBeNull();
    expect(toInput({ ...initialForm(saved, prod), passwordAction: "set", password: "" }, true).password).toBeNull();
  });
  it("sends nothing when there never was a password", () => {
    expect("password" in toInput(initialForm(null, prod), false)).toBe(false);
  });
  it("trims fields and turns an empty Reply-To into null", () => {
    const f = { ...initialForm(null, prod), host: " smtp.gmail.com ", from_address: " a@example.com ", reply_to: "  " };
    expect(toInput(f, false)).toMatchObject({ host: "smtp.gmail.com", from_address: "a@example.com", reply_to: null });
  });
});

describe("initialForm", () => {
  it("shows a stored password as kept, never its value", () => {
    const f = initialForm(saved, prod);
    expect(f.passwordAction).toBe("keep");
    expect(f.password).toBe("");
  });
  it("defaults to 587 STARTTLS", () => {
    expect(initialForm(null, prod)).toMatchObject({ port: "587", tls_mode: "starttls", enabled: true });
  });
});

describe("presets", () => {
  it("fill host, port and TLS from the providers' docs", () => {
    expect(PRESETS.google).toEqual({ host: "smtp.gmail.com", port: 587, tls_mode: "starttls" });
    expect(PRESETS.microsoft).toEqual({ host: "smtp.office365.com", port: 587, tls_mode: "starttls" });
  });
  it("derives the cPanel host from the From domain", () => {
    const f = applyPreset({ ...initialForm(null, prod), from_address: "Team <Team@Acme.Example>" }, "cpanel");
    expect(f).toMatchObject({ host: "mail.acme.example", port: "465", tls_mode: "tls" });
    expect(applyPreset(initialForm(null, prod), "cpanel").host).toBe("");
  });
  it("keeps the other fields", () => {
    const f = applyPreset({ ...initialForm(saved, prod), username: "u" }, "google");
    expect(f.username).toBe("u");
    expect(f.passwordAction).toBe("keep");
  });
  it("parses address domains", () => {
    expect(addressDomain("a@b.example")).toBe("b.example");
    expect(addressDomain("nope")).toBe("");
  });
});

describe("formProblems", () => {
  it("flags ports outside the server's list", () => {
    expect(formProblems({ ...initialForm(null, prod), port: "1025" }, prod, false)).toEqual(["port"]);
    expect(formProblems({ ...initialForm(null, dev), port: "1025" }, dev, false)).toEqual([]);
  });
  it("refuses plaintext unless the server allows it", () => {
    expect(formProblems({ ...initialForm(null, prod), tls_mode: "none" }, prod, false)).toContain("tls_mode");
    expect(formProblems({ ...initialForm(null, dev), port: "1025", tls_mode: "none" }, dev, false)).toEqual([]);
  });
  it("never sends credentials without TLS", () => {
    const f = { ...initialForm(saved, dev), tls_mode: "none" as const, username: "" };
    expect(formProblems(f, dev, true)).toContain("credentials_without_tls"); // the kept password
    expect(formProblems({ ...f, passwordAction: "clear" }, dev, true)).not.toContain("credentials_without_tls");
  });
});
