// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AppNotification } from "../api/notifications";
import { NotificationBell } from "./NotificationBell";

const note = (id: number, patch: Partial<AppNotification> = {}): AppNotification => ({
  id,
  kind: "mention",
  read: false,
  created_at: new Date().toISOString(),
  workspace_slug: "acme",
  workspace_name: "Acme",
  project_key: "QA",
  item_number: id,
  item_key: `QA-${String(id)}`,
  item_title: `Tapşırıq ${String(id)}`,
  actor_id: "u1",
  actor_name: "Alice",
  ...patch,
});

let unread = 2;
let calls: { method: string; url: string }[] = [];

function json(body: unknown, status = 200) {
  return new Response(status === 204 ? null : JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

beforeEach(() => {
  unread = 2;
  calls = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init: RequestInit) => {
      const method = init.method ?? "GET";
      calls.push({ method, url });
      if (url.endsWith("/unread-count")) return Promise.resolve(json({ unread, capped: false }));
      if (url.includes("/me/notifications?")) return Promise.resolve(json({ notifications: [note(1), note(2, { read: true, kind: "overdue", actor_name: null, actor_id: null })], next_cursor: null }));
      if (method === "POST") {
        unread = Math.max(0, unread - 1);
        return Promise.resolve(url.endsWith("/read-all") ? json({ marked: 1 }) : json(null, 204));
      }
      return Promise.resolve(json({}, 404));
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function Where() {
  const loc = useLocation();
  return <output data-testid="where">{loc.pathname + loc.search}</output>;
}

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/"]}>
        <NotificationBell />
        <Routes>
          <Route path="*" element={<Where />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("NotificationBell", () => {
  it("labels the button with the unread count and shows a badge", async () => {
    setup();
    const bell = await screen.findByRole("button", { name: "Bildirişlər, 2 oxunmamış" });
    expect(bell.textContent).toContain("2");
    expect(bell.getAttribute("aria-expanded")).toBe("false");
  });

  it("opens with keyboard focus in the list, moves with arrows, closes on Escape", async () => {
    setup();
    const bell = await screen.findByRole("button", { name: /2 oxunmamış/ });
    fireEvent.click(bell);
    const dialog = await screen.findByRole("dialog", { name: "Bildirişlər" });
    const links = await screen.findAllByRole("link");
    expect(links).toHaveLength(2);
    expect(links[0]?.textContent).toContain("Alice sizi qeyd etdi");
    expect(links[1]?.textContent).toContain("Son tarixi keçdi");
    await waitFor(() => {
      expect(document.activeElement).toBe(links[0]);
    });
    fireEvent.keyDown(dialog, { key: "ArrowDown" });
    expect(document.activeElement).toBe(links[1]);
    fireEvent.keyDown(dialog, { key: "ArrowDown" }); // wraps
    expect(document.activeElement).toBe(links[0]);
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(bell);
  });

  it("opening an unread notification marks it read and navigates to the item", async () => {
    setup();
    fireEvent.click(await screen.findByRole("button", { name: /oxunmamış/ }));
    const [first, second] = await screen.findAllByRole("link");
    expect(first?.getAttribute("href")).toBe("/w/acme/p/QA?item=1");
    await act(async () => {
      fireEvent.click(first as HTMLElement);
      await Promise.resolve();
    });
    expect(screen.getByTestId("where").textContent).toBe("/w/acme/p/QA?item=1");
    await waitFor(() => {
      expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/me/notifications/1/read"))).toBe(true);
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(second).toBeDefined();
    // The badge follows the server count.
    await screen.findByRole("button", { name: "Bildirişlər, 1 oxunmamış" });
  });

  it("marks all read", async () => {
    setup();
    fireEvent.click(await screen.findByRole("button", { name: /oxunmamış/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Hamısını oxunmuş et" }));
    await waitFor(() => {
      expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/me/notifications/read-all"))).toBe(true);
    });
  });
});
