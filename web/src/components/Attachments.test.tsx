// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Attachment } from "../api/attachments";
import { ta } from "../i18n-attach";
import { Attachments } from "./Attachments";
import { ToastProvider } from "./toast";

const file = (patch: Partial<Attachment> = {}): Attachment => ({
  id: "a1",
  item_id: "i1",
  comment_id: null,
  uploader: { id: "me", display_name: "Mən" },
  filename: "hesabat.pdf",
  content_type: "application/pdf",
  size: 2048,
  sha256: "00",
  created_at: new Date().toISOString(),
  ...patch,
});

let list: Attachment[] = [];
let calls: { method: string; url: string; body?: unknown }[] = [];

function json(body: unknown, status = 200) {
  return new Response(status === 204 ? null : JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

beforeEach(() => {
  list = [file(), file({ id: "a2", filename: "başqası.txt", uploader: { id: "other", display_name: "Digər" } })];
  calls = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init: RequestInit = {}) => {
      const method = init.method ?? "GET";
      calls.push({ method, url, body: init.body });
      if (method === "GET" && url.endsWith("/attachments")) {
        return Promise.resolve(json({ attachments: list, enabled: true, limits: { max_file_bytes: 10, max_per_item: 100 } }));
      }
      if (method === "POST") return Promise.resolve(json(file({ id: "new" }), 201));
      if (method === "DELETE") return Promise.resolve(json(null, 204));
      return Promise.resolve(json({}, 404));
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function setup(props: { canWrite?: boolean; isAdmin?: boolean } = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <Attachments ws="acme" project="QA" n={7} meId="me" canWrite={props.canWrite ?? true} isAdmin={props.isAdmin ?? false} />
      </ToastProvider>
    </QueryClientProvider>,
  );
}

describe("Attachments", () => {
  it("lists files; only the uploader (or an admin) may delete", async () => {
    setup();
    await screen.findByText("hesabat.pdf");
    expect(screen.getByText("başqası.txt")).toBeTruthy();
    expect(screen.getByRole("button", { name: `${ta.remove}: hesabat.pdf` })).toBeTruthy();
    expect(screen.queryByRole("button", { name: `${ta.remove}: başqası.txt` })).toBeNull();
  });

  it("admins may delete anyone's file", async () => {
    setup({ isAdmin: true });
    await screen.findByText("başqası.txt");
    expect(screen.getByRole("button", { name: `${ta.remove}: başqası.txt` })).toBeTruthy();
  });

  it("read-only users get no upload or delete controls", async () => {
    setup({ canWrite: false });
    await screen.findByText("hesabat.pdf");
    expect(screen.queryByRole("button", { name: new RegExp(ta.attach) })).toBeNull();
    expect(screen.queryByRole("button", { name: new RegExp(ta.remove) })).toBeNull();
  });

  it("refuses oversize files before uploading and uploads the rest as multipart", async () => {
    setup();
    await screen.findByText("hesabat.pdf");
    const input = screen.getByLabelText<HTMLInputElement>(ta.attach);
    const big = new File(["01234567890"], "böyük.bin");
    const small = new File(["ok"], "kiçik.txt", { type: "text/plain" });
    fireEvent.change(input, { target: { files: [big, small] } });
    await screen.findByText(ta.tooLarge("böyük.bin", 0));
    await waitFor(() => { expect(calls.filter((c) => c.method === "POST")).toHaveLength(1); });
    const post = calls.find((c) => c.method === "POST");
    expect(post?.url).toBe("/api/v1/workspaces/acme/projects/QA/items/7/attachments");
    const form = post?.body as FormData;
    expect((form.get("file") as File).name).toBe("kiçik.txt");
  });
});
