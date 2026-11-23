// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import type { SearchHit, SearchPage } from "../api/search";
import { GlobalSearch } from "./GlobalSearch";

const hit = (patch: Partial<SearchHit>): SearchHit => ({
  kind: "item",
  match: "text",
  project_key: "QA",
  project_name: "Keyfiyyət",
  item_number: 1,
  item_key: "QA-1",
  item_title: "Ödəniş xətası",
  status_name: "To Do",
  status_category: "todo",
  title: [{ text: "Ödəniş", match: true }, { text: " xətası" }],
  snippet: null,
  comment_id: null,
  author: null,
  created_at: null,
  ...patch,
});

const page: SearchPage = {
  hits: [
    hit({}),
    hit({ kind: "comment", comment_id: "c1", item_number: 3, item_key: "QA-3", item_title: "Giriş", title: [{ text: "Giriş" }],
      snippet: [{ text: "<img src=x onerror=alert(1)> " }, { text: "ödənişdən", match: true }], author: { id: "u1", display_name: "Alice" } }),
    hit({ item_number: 2, item_key: "QA-2", item_title: "odenis hesabati", title: [{ text: "odenis", match: true }, { text: " hesabati" }] }),
  ],
  next_cursor: null,
  counts: { items: 2, comments: 1 },
};

let urls: string[] = [];

beforeAll(() => {
  // jsdom has no modal dialogs: open/close is all the component needs.
  HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement) { this.open = true; };
  HTMLDialogElement.prototype.close = function (this: HTMLDialogElement) {
    this.open = false;
    this.dispatchEvent(new Event("close"));
  };
  Element.prototype.scrollIntoView = () => undefined;
});

beforeEach(() => {
  urls = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      urls.push(url);
      return Promise.resolve(new Response(JSON.stringify(page), { status: 200, headers: { "Content-Type": "application/json" } }));
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function Where() {
  const loc = useLocation();
  return <div data-testid="where">{loc.pathname + loc.search}</div>;
}

function setup(path = "/w/acme") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/w/:ws/*" element={<><GlobalSearch /><Where /></>} />
          <Route path="*" element={<><GlobalSearch /><Where /></>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("GlobalSearch", () => {
  it("is not shown outside a workspace", () => {
    setup("/");
    expect(screen.queryByRole("button", { name: "Axtar" })).toBeNull();
  });

  it("opens with Ctrl+K, groups results, navigates with the keyboard and opens the item", async () => {
    setup();
    fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    const input = await screen.findByRole("combobox", { name: "Axtarış sorğusu" });
    expect(document.activeElement).toBe(input);
    expect(screen.getByText(/Ən azı 2 hərf/)).toBeTruthy();

    fireEvent.change(input, { target: { value: "odenis" } });
    const list = await screen.findByRole("listbox");
    expect(urls[0]).toBe("/api/v1/workspaces/acme/search?q=odenis&limit=20");

    const groups = within(list).getAllByRole("group");
    expect(groups.map((g) => g.getAttribute("aria-labelledby") && document.getElementById(g.getAttribute("aria-labelledby") ?? "")?.textContent)).toEqual([
      "Tapşırıqlar2",
      "Şərhlər1",
    ]);
    const options = within(list).getAllByRole("option");
    expect(options.map((o) => o.textContent.slice(0, 4))).toEqual(["QA-1", "QA-2", "QA-3"]); // items first, then comments
    // Highlighting uses <mark> around text; server text is never parsed as HTML.
    expect(options[0]?.querySelector("mark")?.textContent).toBe("Ödəniş");
    expect(options[2]?.querySelector("img")).toBeNull();
    expect(options[2]?.textContent).toContain("<img src=x onerror=alert(1)>");
    expect(options[2]?.textContent).toContain("Alice şərhi");

    expect(options[0]?.getAttribute("aria-selected")).toBe("true");
    expect(input.getAttribute("aria-activedescendant")).toBe(options[0]?.id);
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(input.getAttribute("aria-activedescendant")).toBe(options[2]?.id);
    fireEvent.keyDown(input, { key: "ArrowDown" }); // wraps around
    fireEvent.keyDown(input, { key: "ArrowUp" });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => { expect(screen.getByTestId("where").textContent).toBe("/w/acme/p/QA?item=3"); });
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Axtar" }));
  });

  it("does not query until the text is searchable, and opens a hit on click", async () => {
    setup();
    fireEvent.click(screen.getByRole("button", { name: "Axtar" }));
    const input = await screen.findByRole("combobox");
    fireEvent.change(input, { target: { value: "a" } });
    await new Promise((r) => setTimeout(r, 400));
    expect(urls).toEqual([]);
    fireEvent.change(input, { target: { value: "QA-2" } });
    const options = await screen.findAllByRole("option");
    expect(screen.getByRole("status").textContent).toBe("Tapşırıqlar: 2, Şərhlər: 1");
    fireEvent.click(options[1] as HTMLElement);
    await waitFor(() => { expect(screen.getByTestId("where").textContent).toBe("/w/acme/p/QA?item=2"); });
  });
});
