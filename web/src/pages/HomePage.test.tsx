// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Home, HomeItem, HomeWorkspace } from "../api/home";
import { HomePage } from "./HomePage";

// The page chrome needs the auth context; its header is not under test.
vi.mock("../components/Layout", () => ({ Layout: ({ children, actions }: { children: React.ReactNode; actions?: React.ReactNode }) => <main>{actions}{children}</main> }));

// Synthetic data only (ADR-0037).
const ws = (slug: string, patch: Partial<HomeWorkspace> = {}): HomeWorkspace => ({
  id: slug, slug, name: slug.toUpperCase(), role: "member", accent: null, timezone: "Asia/Baku", today: "2031-03-10",
  projects: 1, assigned: 0, overdue: 0, week: 0, approvals: 0, last_activity: null, ...patch,
});
const row = (key: string, reason: HomeItem["reason"], slug = "acme"): HomeItem => ({
  reason, workspace: { slug, name: slug.toUpperCase(), accent: null }, project: { key: key.split("-")[0] ?? "", name: "Maliyyə" },
  key, number: Number(key.split("-")[1]), title: `İş ${key}`, status: { id: "s", name: "İcrada", category: "in_progress" },
  due_date: reason === "approval" ? null : "2031-03-08", assigned: reason !== "approval",
  approval: reason === "approval" ? { requested_at: "2031-03-01T08:00:00Z", due_on: null, overdue: false } : null,
});

let home: Home;
let calls: string[] = [];
const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });

beforeEach(() => {
  calls = [];
  home = {
    counters: { overdue: 1, approvals: 1, week: 1, assigned: 2 },
    show: "attention",
    items: [row("FIN-1", "overdue", "beta"), row("FIN-2", "approval"), row("FIN-3", "week")],
    total: 3,
    workspaces: [ws("acme", { week: 1, assigned: 1 }), ws("beta", { overdue: 1, assigned: 1 })],
    watching: [],
    recent_projects: [],
  };
  vi.stubGlobal("fetch", vi.fn((url: string) => {
    calls.push(url);
    if (url.includes("/me/home")) return Promise.resolve(json(home));
    if (url.includes("/me/notifications")) return Promise.resolve(json({ notifications: [], next_cursor: null }));
    if (url.includes("/me/requests")) return Promise.resolve(json({ requests: [], next_cursor: null, unseen: 0 }));
    if (url.endsWith("/me")) return Promise.resolve(json({ id: "u", email: "erin@example.com", display_name: "Erin Synthetic" }));
    return Promise.resolve(json({}));
  }));
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderAt(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes><Route path="/" element={<HomePage />} /></Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("HomePage", () => {
  it("greets, counts and lists in the server's order with drawer links", async () => {
    renderAt("/");
    expect(await screen.findByRole("heading", { level: 1, name: "Salam, Erin" })).toBeTruthy();
    const counters = screen.getByRole("navigation", { name: "Sayğaclar" });
    expect(within(counters).getByRole("link", { name: "Gecikən: 1" }).getAttribute("href")).toBe("/?show=overdue");
    // Two workspaces: approvals open the combined list, not one inbox.
    expect(within(counters).getByRole("link", { name: "Təsdiqimi gözləyən: 1" }).getAttribute("href")).toBe("/?show=approval");
    const rows = within(screen.getByTestId("home-attention")).getAllByTestId("home-row");
    expect(rows.map((r) => r.getAttribute("data-reason"))).toEqual(["overdue", "approval", "week"]);
    expect(rows[0]?.getAttribute("href")).toBe("/w/beta/p/FIN?item=1");
    expect(rows[1]?.getAttribute("href")).toBe("/w/acme/p/FIN?item=2&focus=approval");
    expect(calls.some((u) => u.includes("/me/home?show=attention&limit=15"))).toBe(true);
    // Cards: the one with an overdue item first; the old accessible names stay.
    const cards = within(screen.getByTestId("workspace-cards")).getAllByRole("link");
    expect(cards.map((c) => c.getAttribute("href"))).toEqual(["/w/beta", "/w/acme"]);
    expect(screen.getByRole("heading", { name: "Workspace-lər" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Yeni workspace" })).toBeTruthy();
  });

  it("links approvals to the inbox when there is one workspace, and shows a full list", async () => {
    home.workspaces = [ws("acme")];
    home.show = "approval";
    home.items = [row("FIN-2", "approval")];
    home.total = 1;
    renderAt("/?show=approval");
    expect(await screen.findByRole("heading", { name: "Qərarımı gözləyən təsdiqlər" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Təsdiqimi gözləyən: 1" }).getAttribute("href")).toBe("/w/acme/approvals");
    expect(screen.getByRole("link", { name: "Təsdiqimi gözləyən: 1" }).getAttribute("aria-current")).toBe("page");
    expect(calls.some((u) => u.includes("/me/home?show=approval&limit=200"))).toBe(true);
  });

  it("is calm when nothing needs attention, with recent projects", async () => {
    home = { ...home, counters: { overdue: 0, approvals: 0, week: 0, assigned: 3 }, items: [], total: 0,
      recent_projects: [{ workspace: { slug: "acme", name: "ACME", accent: "forest" }, project: { key: "FIN", name: "Maliyyə" }, last_activity: "2031-03-09T10:00:00Z" }] };
    renderAt("/");
    expect(await screen.findByText("Hazırda diqqətinizi tələb edən iş yoxdur")).toBeTruthy();
    expect(screen.getByRole("link", { name: /Maliyyə · ACME/ }).getAttribute("href")).toBe("/w/acme/p/FIN");
  });

  it("explains invitations and offers a template when there is no workspace", async () => {
    home = { ...home, counters: { overdue: 0, approvals: 0, week: 0, assigned: 0 }, items: [], total: 0, workspaces: [] };
    renderAt("/");
    expect(await screen.findByText("Hələ workspace-iniz yoxdur")).toBeTruthy();
    expect(screen.getByText(/keçid e-poçtunuzdadır/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Şablonla workspace yarat" })).toBeTruthy();
    expect(screen.queryByRole("navigation", { name: "Sayğaclar" })).toBeNull();
  });
});
