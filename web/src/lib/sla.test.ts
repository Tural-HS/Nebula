import { describe, expect, it } from "vitest";

import type { ItemSla } from "../api/types";
import { slaDetails, slaLabel } from "./sla";

const base: ItemSla = {
  state: "on_track", started_at: "2026-05-01T08:00:00Z", response_due: "2026-05-02T20:00:00Z",
  resolution_due: "2026-05-06T20:00:00Z", responded_at: null, resolved_at: null,
};
const fmt = (iso: string) => iso.slice(0, 10);

describe("sla", () => {
  it("names the state in text, then the open dues", () => {
    expect(slaLabel(base, fmt)).toBe("SLA qaydasındadır · İlk cavab: 2026-05-02 · Həll: 2026-05-06");
    expect(slaLabel({ ...base, state: "breached" }, fmt)).toMatch(/^SLA pozulub/);
  });
  it("reached targets say so instead of a date", () => {
    expect(slaDetails({ ...base, responded_at: "2026-05-01T09:00:00Z" }, fmt)).toEqual(["İlk cavab: cavab verilib", "Həll: 2026-05-06"]);
    expect(slaDetails({ ...base, response_due: null, resolved_at: "2026-05-03T09:00:00Z", state: "met" }, fmt)).toEqual(["Həll: həll olunub"]);
  });
});
