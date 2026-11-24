// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PublicForm } from "../api/forms";
import { FormPage } from "./FormPage";

// The page chrome needs auth and a workspace; the form itself does not.
vi.mock("../components/Layout", () => ({ Layout: ({ children }: { children: ReactNode }) => <>{children}</> }));

const form: PublicForm = {
  id: "f1", slug: "payment-request", title: "Payment request", intro: "For vendors.", workspace: "Acme", open: true,
  max_files: 0, can_manage: false,
  fields: [
    { key: "vendor", label: "Vendor name", help: "As on the invoice", required: true, type: "text" },
    { key: "amount", label: "Amount", required: true, type: "number" },
  ],
};

let posted: unknown[] = [];
let reply: () => Response;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

beforeEach(() => {
  posted = [];
  reply = () => json({ id: "s1", key: "ACME-7", title: "Pay", created_at: "2026-11-24T10:00:00Z" }, 201);
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init: RequestInit) => {
      if (init.method === "POST") {
        posted.push(JSON.parse(init.body as string));
        return Promise.resolve(reply());
      }
      if (url.endsWith("/forms/payment-request")) return Promise.resolve(json({ form }));
      return Promise.resolve(json({ error: { code: "not_found", message: "resource not found" } }, 404));
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderAt(path: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/f/:ref" element={<FormPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("FormPage", () => {
  it("shows an accessible error summary instead of sending an incomplete form", async () => {
    renderAt("/f/payment-request");
    const vendor = await screen.findByLabelText(/Vendor name/);
    expect(vendor.getAttribute("aria-describedby")).toContain("ff-vendor-help");
    fireEvent.click(screen.getByRole("button", { name: "Göndər" }));
    const summary = await screen.findByRole("alert");
    expect(summary.textContent).toContain("Formada 2 xəta var");
    await waitFor(() => { expect(document.activeElement).toBe(summary); });
    expect(vendor.getAttribute("aria-invalid")).toBe("true");
    fireEvent.click(screen.getByRole("link", { name: /Vendor name/ }));
    expect(document.activeElement).toBe(vendor);
    expect(posted).toEqual([]);
  });

  it("puts a server validation error under its field", async () => {
    reply = () => json({ error: { code: "validation_failed", message: "must be between -1e15 and 1e15", field: "values.amount" } }, 422);
    renderAt("/f/payment-request");
    fireEvent.change(await screen.findByLabelText(/Vendor name/), { target: { value: "Vendor-A" } });
    fireEvent.change(screen.getByLabelText(/Amount/), { target: { value: "1e20" } });
    fireEvent.click(screen.getByRole("button", { name: "Göndər" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Amount: must be between");
    expect(posted).toEqual([{ values: { vendor: "Vendor-A", amount: 1e20 } }]);
  });

  it("confirms with the item key", async () => {
    renderAt("/f/payment-request");
    fireEvent.change(await screen.findByLabelText(/Vendor name/), { target: { value: "Vendor-A" } });
    fireEvent.change(screen.getByLabelText(/Amount/), { target: { value: "120,50" } });
    fireEvent.click(screen.getByRole("button", { name: "Göndər" }));
    expect((await screen.findByTestId("submission-key")).textContent).toBe("ACME-7");
    expect(posted).toEqual([{ values: { vendor: "Vendor-A", amount: 120.5 } }]);
    expect(screen.getByRole("link", { name: "Tələblərim" }).getAttribute("href")).toBe("/requests/s1");
  });

  it("says when the form is not available", async () => {
    renderAt("/f/elsewhere");
    expect(await screen.findByText(/Forma tapılmadı/)).toBeTruthy();
  });

  it("shows a closed form without inputs", async () => {
    form.open = false;
    try {
      renderAt("/f/payment-request");
      expect(await screen.findByText(/hazırda tələb qəbul etmir/)).toBeTruthy();
      expect(screen.queryByRole("button", { name: "Göndər" })).toBeNull();
    } finally {
      form.open = true;
    }
  });
});
