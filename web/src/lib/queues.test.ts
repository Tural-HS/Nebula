import { describe, expect, it } from "vitest";

import { activeQueue, QUEUES, queueState, queueVisible, rowSignal } from "./queues";
import { emptyState, readState, writeState } from "./views";

describe("queues", () => {
  it("each preset is recognised as itself", () => {
    for (const q of QUEUES) expect(activeQueue(queueState(q))).toBe(q);
  });
  it("a mix of filters is no preset", () => {
    expect(activeQueue({ assignee: "me", due: "overdue", approval: "", blocked: false })).toBeNull();
    expect(activeQueue({ assignee: "none", due: "", approval: "", blocked: false })).toBeNull();
    expect(activeQueue({ assignee: "", due: "today", approval: "", blocked: false })).toBeNull();
    expect(activeQueue({ assignee: "", due: "", approval: "mine", blocked: false })).toBeNull();
  });
  it("presets keep search, statuses and fields, and survive the URL", () => {
    const s = { ...emptyState(), q: "akt", status: ["00000000-0000-4000-8000-000000000001"], ...queueState("blocked") };
    const back = readState(writeState(s));
    expect(back.blocked).toBe(true);
    expect(back.q).toBe("akt");
    expect(activeQueue(back)).toBe("blocked");
    expect(writeState({ ...s, ...queueState("all") }).has("blocked")).toBe(false);
  });
  it("only blocked=true turns the blocked filter on", () => {
    expect(readState(new URLSearchParams("blocked=1")).blocked).toBe(false);
    expect(readState(new URLSearchParams("blocked=true")).blocked).toBe(true);
  });
  it("the row stripe shows the most urgent signal", () => {
    expect(rowSignal({ overdue: true, approval: {}, blocked: true })).toBe("overdue");
    expect(rowSignal({ overdue: false, approval: {}, blocked: true })).toBe("approval");
    expect(rowSignal({ overdue: false, approval: null, blocked: true })).toBe("blocked");
    expect(rowSignal({ overdue: false })).toBeNull();
  });
});

describe("SLA queues", () => {
  it("are hidden until they can matter, and survive the URL", () => {
    expect(queueVisible("sla_breached", 0, false)).toBe(false);
    expect(queueVisible("sla_breached", 2, false)).toBe(true);
    expect(queueVisible("sla_at_risk", 0, true)).toBe(true);
    expect(queueVisible("overdue", 0, false)).toBe(true);
    const back = readState(writeState({ ...emptyState(), ...queueState("sla_breached") }));
    expect(back.sla).toBe("breached");
    expect(activeQueue(back)).toBe("sla_breached");
    expect(readState(new URLSearchParams("sla=late")).sla).toBe("");
  });
});

describe("category and month queues (ADR-0065)", () => {
  it("month means due by month end and still to do", () => {
    expect(queueState("month")).toMatchObject({ due: "month", category: "todo" });
    expect(activeQueue({ assignee: "", due: "month", approval: "", blocked: false, sla: "", category: "todo" })).toBe("month");
    expect(activeQueue({ assignee: "", due: "month", approval: "", blocked: false, sla: "", category: "" })).toBeNull();
    expect(activeQueue({ assignee: "", due: "", approval: "", blocked: false, sla: "", category: "done" })).toBe("done");
  });
});
