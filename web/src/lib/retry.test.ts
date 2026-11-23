import { describe, expect, it } from "vitest";

import { ApiError } from "../api/client";
import { retryDelay, shouldRetry, shouldRetryMutation } from "./retry";

const err = (status: number, retryAfter?: number) => new ApiError(status, "x", "x", undefined, retryAfter);

describe("shouldRetry", () => {
  it.each([403, 404, 409, 422])("does not retry %i", (s) => {
    expect(shouldRetry(0, err(s))).toBe(false);
  });
  it.each([429, 500, 503])("retries %i", (s) => {
    expect(shouldRetry(0, err(s))).toBe(true);
  });
  it("retries network errors", () => {
    expect(shouldRetry(1, new TypeError("Failed to fetch"))).toBe(true);
  });
  it("gives up after three attempts", () => {
    expect(shouldRetry(3, err(429))).toBe(false);
  });
});

describe("retryDelay", () => {
  it("honours Retry-After, capped at 30 s", () => {
    expect(retryDelay(0, err(429, 2))).toBe(2000);
    expect(retryDelay(0, err(429, 3600))).toBe(30_000);
  });
  it("backs off exponentially otherwise", () => {
    expect([0, 1, 2, 5].map((n) => retryDelay(n, err(503)))).toEqual([500, 1000, 2000, 8000]);
  });
});

describe("shouldRetryMutation", () => {
  it("retries only rate-limited writes", () => {
    expect(shouldRetryMutation(0, err(429))).toBe(true);
    expect(shouldRetryMutation(0, err(500))).toBe(false);
    expect(shouldRetryMutation(0, err(409))).toBe(false);
    expect(shouldRetryMutation(0, new TypeError("Failed to fetch"))).toBe(false);
    expect(shouldRetryMutation(3, err(429))).toBe(false);
  });
});
