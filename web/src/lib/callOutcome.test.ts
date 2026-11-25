import { describe, expect, it } from "vitest";

import { outcomeComment } from "./callOutcome";

describe("outcomeComment", () => {
  it("writes a short, searchable comment", () => {
    expect(outcomeComment("no_answer")).toBe("📞 Cavab vermədi");
    expect(outcomeComment("promised", "2026-12-05")).toMatch(/^📞 Ödəməyə söz verdi: 5 dek 2026$/);
    expect(outcomeComment("promised")).toBe("📞 Ödəməyə söz verdi");
  });
});
