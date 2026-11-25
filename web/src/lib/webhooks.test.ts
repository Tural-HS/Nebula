import { describe, expect, it } from "vitest";

import { approvalsByWildcard, defaultEvents, looksLikeChatWebhook, sendsApprovals, withApprovals } from "./webhooks";

describe("looksLikeChatWebhook", () => {
  it.each([
    ["https://hooks.slack.com/services/T000/B000/XXXX", true],
    ["https://chat.example.com/hooks/abcdefghijklmnopqrstuvwxyz", true],
    ["https://chat.example.com/hooks/abcdefghijklmnopqrstuvwxyz/", true],
    ["https://chat.example.com/hooks/short", false],
    ["https://receiver.example.com/nebula", false],
    ["https://evil.example.com/?u=hooks.slack.com", false],
    ["not a url", false],
    ["", false],
  ])("%s -> %s", (url, want) => {
    expect(looksLikeChatWebhook(url)).toBe(want);
  });
});

describe("defaultEvents", () => {
  it("keeps the Nebula default and adds comments for chat", () => {
    expect(defaultEvents("nebula")).toBe("item.*");
    expect(defaultEvents("slack")).toBe("item.*, comment.created, approval.*, workspace.overdue_digest");
  });
});

describe("approval events toggle", () => {
  it.each([
    [["item.*"], false],
    [["item.*", "approval.*"], true],
    [["approval.rejected"], true],
    [["*"], true],
    [["approvals.x"], false],
  ])("%j sends approvals: %s", (events, want) => {
    expect(sendsApprovals(events)).toBe(want);
  });

  it("adds approval.* once and removes every approval pattern", () => {
    expect(withApprovals(["item.*"], true)).toEqual(["item.*", "approval.*"]);
    expect(withApprovals(["item.*", "approval.requested"], true)).toEqual(["item.*", "approval.requested"]);
    expect(withApprovals(["item.*", "approval.*", "approval.rejected"], false)).toEqual(["item.*"]);
    expect(withApprovals(["*"], false)).toEqual(["*"]);
    expect(approvalsByWildcard(["*"])).toBe(true);
    expect(approvalsByWildcard(["approval.*"])).toBe(false);
  });
});
