// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { InboxEntry } from "../api/approvals";
import { ApprovalBadge, ApprovalInboxLink } from "../components/Approvals";
import { ApprovalsInboxPage } from "./ApprovalsInboxPage";

// The page chrome needs the auth context; its header is not under test.
vi.mock("../components/Layout", () => ({ Layout: ({ children }: { children: React.ReactNode }) => <>{children}</> }));

// Synthetic data only (ADR-0034 tests).
const entry = (n: number, patch: Partial<InboxEntry> = {}): InboxEntry => ({
  id: `req-${String(n)}`,
  item: { key: `FIN-${String(n)}`, number: n, project_key: "FIN", title: `Ödəniş ${String(n)}` },
  requested_by: { id: "u1", display_name: "Carol" },
  from_status: { id: "s1", name: "Yoxlamada", category: "in_progress" },
  to_status: { id: "s2", name: "Ödənildi", category: "done" },
  approvals: 0,
  min_approvals: 2,
  created_at: new Date().toISOString(),
  due_on: null,
  overdue: false,
  can_decide: true,
  ...patch,
});

let calls: string[] = [];

function json(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
}

beforeEach(() => {
  calls = [];
  vi.stubGlobal("fetch", vi.fn((url: string) => {
    calls.push(url);
    if (url.includes("/approvals/count")) return Promise.resolve(json({ count: 3, overdue: 1, cap: 100 }));
    if (url.includes("/approvals?")) {
      const mine = url.includes("mine=true");
      const list = mine ? [entry(1), entry(2, { overdue: true, due_on: "2031-03-20" })] : [entry(1), entry(2), entry(3, { can_decide: false })];
      return Promise.resolve(json({ approvals: list, next_cursor: null }));
    }
    if (url.endsWith("/workspaces/acme")) return Promise.resolve(json({ id: "w", slug: "acme", name: "Acme", role: "member" }));
    return Promise.resolve(json({}));
  }));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderAt(path: string, el: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/w/:ws/approvals" element={el} />
          <Route path="/w/:ws" element={el} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ApprovalsInboxPage", () => {
  it("lists my pending decisions, overdue first, each linking to the drawer's approval panel", async () => {
    renderAt("/w/acme/approvals", <ApprovalsInboxPage />);
    const rows = await screen.findAllByTestId("approval-inbox-row");
    expect(rows).toHaveLength(2);
    expect(rows[0]?.textContent).toContain("FIN-2");
    expect(rows[0]?.getAttribute("href")).toBe("/w/acme/p/FIN?item=2&focus=approval");
    expect(screen.getByTestId("approval-due").textContent).toContain("2031-03-20");
    expect(calls.some((u) => u.includes("/workspaces/acme/approvals?") && u.includes("state=pending") && u.includes("mine=true"))).toBe(true);
  });

  it("switches to all pending requests", async () => {
    renderAt("/w/acme/approvals", <ApprovalsInboxPage />);
    await screen.findAllByTestId("approval-inbox-row");
    fireEvent.click(screen.getByRole("tab", { name: "Bütün gözləyənlər" }));
    await waitFor(() => { expect(screen.getAllByTestId("approval-inbox-row")).toHaveLength(3); });
    expect(calls.some((u) => u.includes("mine=false"))).toBe(true);
  });
});

describe("approval badges", () => {
  it("shows the inbox count with an overdue colour", async () => {
    renderAt("/w/acme", <ApprovalInboxLink ws="acme" />);
    const link = await screen.findByRole("link", { name: /3 sorğu qərarınızı gözləyir/ });
    expect(link.getAttribute("href")).toBe("/w/acme/approvals");
    expect(link.getAttribute("aria-label")).toContain("1 sorğunun müddəti keçib");
  });

  it("marks an overdue item badge", () => {
    render(<ApprovalBadge approval={{ id: "r", to_status: { id: "s", name: "Ödənildi", category: "done" }, requested_by: "u", approvals: 1, min_approvals: 2, due_on: "2031-03-20", overdue: true }} />);
    const badge = screen.getByTestId("approval-badge");
    expect(badge.getAttribute("data-overdue")).toBe("true");
    expect(badge.getAttribute("aria-label")).toContain("2031-03-20");
  });
});
