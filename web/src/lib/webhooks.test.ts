import { describe, expect, it } from "vitest";

import { defaultEvents, looksLikeChatWebhook } from "./webhooks";

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
    expect(defaultEvents("slack")).toBe("item.*, comment.created");
  });
});
