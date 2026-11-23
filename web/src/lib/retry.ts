import { ApiError } from "../api/client";

const MAX_RETRIES = 3;

/**
 * Whether a failed query should be retried. Client errors (403/404/422) will
 * not fix themselves, but 429 (rate limited) and 5xx / network failures are
 * transient: giving up on them leaves the UI stuck with missing data.
 */
export function shouldRetry(failureCount: number, err: unknown): boolean {
  if (failureCount >= MAX_RETRIES) return false;
  if (!(err instanceof ApiError)) return true; // network error
  return err.status === 429 || err.status >= 500;
}

/** Waits as long as the server asked (capped), otherwise backs off exponentially. */
export function retryDelay(failureCount: number, err: unknown): number {
  if (err instanceof ApiError && err.retryAfter !== undefined) return Math.min(err.retryAfter, 30) * 1000;
  return Math.min(500 * 2 ** failureCount, 8000);
}

/**
 * Whether a failed mutation should be retried: only 429. The rate limiter
 * rejects a request before any handler runs, so sending it again cannot apply
 * a change twice. Anything else (5xx, network) might have been applied
 * already, so a write is never retried blindly.
 */
export function shouldRetryMutation(failureCount: number, err: unknown): boolean {
  return failureCount < MAX_RETRIES && err instanceof ApiError && err.status === 429;
}
