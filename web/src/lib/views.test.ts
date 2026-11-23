import { describe, expect, it } from "vitest";

import type { FieldDefinition, Member } from "../api/types";
import {
  DEFAULT_COLUMNS,
  emptyState,
  filtersOf,
  formatDecimal,
  formatFieldValue,
  hasState,
  moveColumn,
  readState,
  sameState,
  stateFromView,
  sumFields,
  viewFromState,
  visibleColumns,
  writeState,
} from "./views";

const S1 = "0193a1b2-0000-7000-8000-000000000001";
const S2 = "0193a1b2-0000-7000-8000-000000000002";
const U1 = "0193a1b2-0000-7000-8000-0000000000aa";

const field = (key: string, type: FieldDefinition["type"], extra: Partial<FieldDefinition> = {}): FieldDefinition => ({
  id: key, key, name: key.toUpperCase(), type, options: {}, archived: false, ...extra,
});
const fields = new Map([
  ["monthly", field("monthly", "number")],
  ["remaining", field("remaining", "number")],
  ["plan", field("plan", "select", { options: { choices: [{ id: "c1", label: "Plan 2027" }] } })],
  ["old", field("old", "number", { archived: true })],
  ["owner", field("owner", "user")],
  ["start", field("start", "date")],
  ["tags", field("tags", "multi_select", { options: { choices: [{ id: "a", label: "UI" }, { id: "b", label: "API" }] } })],
  ["paid", field("paid", "boolean")],
]);

describe("page state in the URL", () => {
  it("round-trips every part and keeps unrelated params", () => {
    const s = {
      ...emptyState(),
      mode: "board" as const,
      q: "ödəniş",
      assignee: "me",
      due: "overdue" as const,
      status: [S2, S1],
      fields: { plan: "c1" },
      columns: ["key", "title", "field.monthly"],
      sort: "due" as const,
      group: "category" as const,
    };
    const p = writeState(s, new URLSearchParams("item=7&saved=x&view=table&field.gone=1"));
    expect(p.get("item")).toBe("7");
    expect(p.get("saved")).toBe("x");
    expect(p.get("field.gone")).toBeNull();
    expect(p.get("status")).toBe(`${S1},${S2}`);
    expect(readState(p)).toEqual({ ...s, status: [S1, S2] });
  });

  it("writes nothing for the default state", () => {
    expect(writeState(emptyState()).toString()).toBe("");
    expect(hasState(new URLSearchParams("saved=x&item=3"))).toBe(false);
    expect(hasState(new URLSearchParams("saved=x&field.plan=c1"))).toBe(true);
  });

  it("ignores malformed values", () => {
    const s = readState(new URLSearchParams("view=list&assignee=<script>&due=never&status=nope," + S1 + "&cols=title,salary,field.Bad,field.ok&sort=x&group=y&field.BAD=1"));
    expect(s).toEqual({ ...emptyState(), status: [S1], columns: ["title", "field.ok"] });
  });

  it("detects changes against a view", () => {
    const a = readState(new URLSearchParams("q=x&status=" + S1));
    expect(sameState(a, readState(new URLSearchParams("status=" + S1 + "&q=x")))).toBe(true);
    expect(sameState(a, { ...a, sort: "due" })).toBe(false);
  });
});

describe("saved views", () => {
  it("converts state to a view config and back", () => {
    const s = { ...emptyState(), q: " a ", assignee: U1, columns: ["title", "field.monthly"], sort: "due" as const };
    const v = viewFromState(s);
    expect(v).toEqual({
      kind: "table",
      config: { filters: { q: "a", assignee: U1 }, columns: ["title", "field.monthly"], sort: "due", group_by: "status" },
    });
    const back = stateFromView(v);
    expect(sameState(back, s)).toBe(true);
  });

  it("uses the default columns for a view without columns", () => {
    const s = stateFromView({ kind: "board", config: { filters: {}, columns: [], sort: "number", group_by: "category" } });
    expect(s.columns).toEqual(DEFAULT_COLUMNS);
    expect(s.mode).toBe("board");
    expect(s.group).toBe("category");
  });

  it("builds the API filters with the debounced query", () => {
    const s = { ...emptyState(), q: "typed", status: [S1], fields: { plan: "c1" }, sort: "due" as const };
    expect(filtersOf(s, "debounced")).toEqual({ q: "debounced", assignee: "", due: "", status: [S1], fields: { plan: "c1" }, sort: "due" });
  });
});

describe("columns", () => {
  it("drops unknown and archived fields and keeps the title", () => {
    expect(visibleColumns(["key", "field.old", "field.nope", "field.monthly"], fields)).toEqual(["key", "title", "field.monthly"]);
    expect(visibleColumns(["status"], fields)).toEqual(["title", "status"]);
  });

  it("sums only number columns, at most 10", () => {
    expect(sumFields(["title", "field.monthly", "field.plan", "field.remaining", "field.old"], fields)).toEqual(["monthly", "remaining"]);
  });

  it("moves columns within bounds", () => {
    expect(moveColumn(["a", "b", "c"], "b", -1)).toEqual(["b", "a", "c"]);
    expect(moveColumn(["a", "b", "c"], "c", 1)).toEqual(["a", "b", "c"]);
    expect(moveColumn(["a", "b"], "x", 1)).toEqual(["a", "b"]);
  });
});

describe("formatting", () => {
  it.each([
    ["1234567.5", "1 234 567,5"],
    ["-0.25", "-0,25"],
    ["2000000000000000.1", "2 000 000 000 000 000,1"],
    ["0", "0"],
    ["abc", "abc"],
    [1e15, "1 000 000 000 000 000"],
    [100.5, "100,5"],
  ])("formatDecimal(%s)", (v, want) => {
    expect(formatDecimal(v)).toBe(want);
  });

  it("formats field values by type", () => {
    const members = new Map<string, Member>([[U1, { user_id: U1, email: "a@x", display_name: "Aynur", role: "member" }]]);
    const f = (k: string, v: Parameters<typeof formatFieldValue>[1]) => formatFieldValue(fields.get(k), v, members);
    expect(f("monthly", 1500)).toBe("1 500");
    expect(f("start", "2026-11-20")).toBe("20 noy 2026");
    expect(f("plan", "c1")).toBe("Plan 2027");
    expect(f("plan", "gone")).toBe("?");
    expect(f("tags", ["b", "a"])).toBe("API, UI");
    expect(f("owner", U1)).toBe("Aynur");
    expect(f("paid", false)).toBe("Xeyr");
    expect(f("monthly", null)).toBe("");
    expect(formatFieldValue(undefined, "raw", members)).toBe("raw");
  });
});
